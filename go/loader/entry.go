package loader

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"maps"
	"reflect"
	"slices"

	cordis "github.com/LarsArtmann/cordis/go"
)

// idSep separates the scope levels of hierarchical entry ids, mirroring
// upstream EntryTree.sep.
const idSep = ":"

// GroupBuiltin resolves to the group plugin, mirroring the cordis:group
// builtin upstream. The plain name "group" resolves to it as well unless a
// user registration takes precedence.
const GroupBuiltin = "cordis:group"

// EventPartialDispose fires when the loader removes or disables an entry.
// Args: (id string, options EntryOptions, active bool). It mirrors the
// loader/partial-dispose event upstream.
const EventPartialDispose = "loader/partial-dispose"

// EntryOptions is one configurable entry, mirroring upstream EntryOptions.
//
// Inject maps service names to either nil (a plain dependency) or a config
// override. Intercept maps service names to config overrides visible inside
// the entry scope. Isolate maps service names to either true (a fresh local
// realm) or a comparable label (a shared realm).
type EntryOptions struct {
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name"`
	Config    any            `json:"config,omitempty"`
	Group     bool           `json:"group,omitempty"`
	Disabled  bool           `json:"disabled,omitempty"`
	Inject    map[string]any `json:"inject,omitempty"`
	Intercept map[string]any `json:"intercept,omitempty"`
	Isolate   map[string]any `json:"isolate,omitempty"`
}

// Entry manages one configured plugin: its options, its fiber and the
// group nested inside it. Entries are created by an EntryGroup; their fibers
// are children of the group's host context.
type Entry struct {
	parent *EntryGroup

	opts     EntryOptions
	reg      Registration
	ctx      *cordis.Context
	fiber    *cordis.Fiber
	err      error
	subgroup *EntryGroup
}

// ID returns the entry's hierarchical id.
func (e *Entry) ID() string {
	t := e.parent.tree
	t.mu.Lock()
	id := e.opts.ID
	t.mu.Unlock()
	for node := t.owner; node != nil; node = node.parent.tree.owner {
		id = node.ID() + idSep + id
	}
	return id
}

// Name returns the entry's plugin name.
func (e *Entry) Name() string {
	t := e.parent.tree
	t.mu.Lock()
	defer t.mu.Unlock()
	return e.opts.Name
}

// Options returns a snapshot of the entry's options.
func (e *Entry) Options() EntryOptions {
	t := e.parent.tree
	t.mu.Lock()
	defer t.mu.Unlock()
	return cloneOptions(e.opts)
}

// Fiber returns the entry's live fiber, or nil.
func (e *Entry) Fiber() *cordis.Fiber {
	t := e.parent.tree
	t.mu.Lock()
	defer t.mu.Unlock()
	return e.fiber
}

// Err returns the error that prevented the entry from starting, if any.
func (e *Entry) Err() error {
	t := e.parent.tree
	t.mu.Lock()
	defer t.mu.Unlock()
	return e.err
}

// Subgroup returns the group nested inside this entry, if the entry hosts
// one.
func (e *Entry) Subgroup() *EntryGroup {
	t := e.parent.tree
	t.mu.Lock()
	defer t.mu.Unlock()
	return e.subgroup
}

// Disabled reports whether the entry, or any of its ancestor entries, is
// disabled. Entries with the group flag are always enabled.
func (e *Entry) Disabled() bool {
	for node := e; node != nil; {
		t := node.parent.tree
		t.mu.Lock()
		group, disabled := node.opts.Group, node.opts.Disabled
		t.mu.Unlock()
		if group {
			return false
		}
		if disabled {
			return true
		}
		node = node.parent.hostEntry
	}
	return false
}

// update applies opts to the entry. With create the options are replaced
// wholesale; otherwise non-zero fields are merged over the current options.
// force skips the no-difference early return. It starts, restarts or
// disposes the entry's fiber as the new options require.
func (e *Entry) update(opts EntryOptions, create, force bool) error {
	t := e.parent.tree
	t.mu.Lock()
	legacy := e.opts
	if create {
		e.opts = cloneOptions(opts)
	} else {
		e.opts = mergeOptions(e.opts, opts)
	}
	next := e.opts
	live := e.fiber != nil
	t.mu.Unlock()

	if e.Disabled() {
		e.dispose()
		return nil
	}
	if live {
		if !force && optionsEqual(legacy, next) {
			return nil
		}
		return e.reconcile(legacy, next)
	}
	return e.init()
}

// reconcile brings a live entry in line with new options. Scope defining
// fields (name, inject, intercept, isolate) require a fresh fiber; a config
// change decodes the raw config through the registration and restarts the
// existing fiber in place.
func (e *Entry) reconcile(legacy, next EntryOptions) error {
	if legacy.Name != next.Name ||
		!mapsEqual(legacy.Inject, next.Inject) ||
		!mapsEqual(legacy.Intercept, next.Intercept) ||
		!mapsEqual(legacy.Isolate, next.Isolate) {
		e.dispose()
		return e.init()
	}
	if !reflect.DeepEqual(legacy.Config, next.Config) {
		if f := e.Fiber(); f != nil {
			cfg := next.Config
			if e.reg.Decode != nil {
				decoded, err := e.reg.Decode(next.Config)
				if err != nil {
					t := e.parent.tree
					t.mu.Lock()
					e.err = err
					t.mu.Unlock()
					return err
				}
				cfg = decoded
			}
			return f.Update(cfg)
		}
	}
	return nil
}

// refresh re-resolves the plugin from the resolver and restarts the entry
// from scratch, preserving its options. A swap of the entry's registration
// becomes visible here: init resolves the new implementation.
func (e *Entry) refresh() error {
	e.dispose()
	return e.init()
}

// init resolves the plugin, builds the entry context and starts the fiber.
// Failures are recorded on the entry and returned; the entry stays in the
// store either way, mirroring the upstream init error path.
func (e *Entry) init() error {
	t := e.parent.tree
	if e.Disabled() {
		return nil
	}
	handle, raw, err := t.resolveFor(e, e.opts.Name)
	if err == nil {
		t.mu.Lock()
		e.reg = raw
		t.mu.Unlock()
		cfg := e.opts.Config
		if raw.Decode != nil {
			cfg, err = raw.Decode(e.opts.Config)
		}
		if err == nil {
			err = e.start(handle, cfg)
		}
	}
	if err != nil {
		t.mu.Lock()
		e.err = err
		t.mu.Unlock()
		return err
	}
	return nil
}

func (e *Entry) start(handle cordis.PluginHandle, cfg any) error {
	t := e.parent.tree
	ectx := e.buildContext()
	if len(e.opts.Inject) > 0 {
		cordis.InjectSpec(handle, e.opts.Inject)
	}
	t.mu.Lock()
	e.ctx = ectx
	t.mu.Unlock()
	t.log("apply plugin " + e.opts.Name)
	f, err := cordis.StartAny(ectx, handle, cfg)
	if err != nil {
		return err
	}
	if f.State() == cordis.StateDisposed {
		// The plugin disposed itself during apply. The disposal happened
		// before the entry tracked the fiber, so mark the entry disabled
		// here, mirroring the self-dispose detection.
		t.markDisabled(e)
		t.Write()
		return nil
	}
	t.mu.Lock()
	e.fiber, e.err = f, nil
	t.mu.Unlock()
	t.track(f, e)
	return nil
}

// buildContext derives the entry scope: a child of the group's host context
// with the entry's intercept and isolate options applied.
func (e *Entry) buildContext() *cordis.Context {
	ectx := e.parent.host.Extend()
	for _, name := range slices.Sorted(maps.Keys(e.opts.Intercept)) {
		ectx = ectx.Intercept(name, e.opts.Intercept[name])
	}
	for _, name := range slices.Sorted(maps.Keys(e.opts.Isolate)) {
		switch label := e.opts.Isolate[name].(type) {
		case nil, bool:
			ectx = ectx.Isolate(name)
		default:
			ectx = ectx.Isolate(name, label)
		}
	}
	return ectx
}

// dispose tears the entry down: its nested group and its fiber. The fiber
// reference is cleared before disposal so that a subsequent init always
// starts a fresh fiber, regardless of drain ordering.
func (e *Entry) dispose() {
	t := e.parent.tree
	t.mu.Lock()
	f := e.fiber
	e.fiber = nil
	if f != nil {
		delete(t.fibers, f)
	}
	sub := e.subgroup
	t.mu.Unlock()
	if sub != nil {
		sub.Stop()
	}
	if f != nil {
		f.Dispose()
	}
}

// track registers the fiber to entry association used by Locate and by the
// self-dispose detection.
func (t *Tree) track(f *cordis.Fiber, e *Entry) {
	t.mu.Lock()
	t.fibers[f] = e
	t.mu.Unlock()
}

// mergeOptions overlays the non-zero fields of next over legacy.
func mergeOptions(legacy, next EntryOptions) EntryOptions {
	merged := legacy
	if next.Name != "" {
		merged.Name = next.Name
	}
	if next.Config != nil {
		merged.Config = next.Config
	}
	if next.Group {
		merged.Group = true
	}
	if next.Disabled {
		merged.Disabled = true
	}
	if len(next.Inject) > 0 {
		merged.Inject = next.Inject
	}
	if len(next.Intercept) > 0 {
		merged.Intercept = next.Intercept
	}
	if len(next.Isolate) > 0 {
		merged.Isolate = next.Isolate
	}
	return merged
}

func cloneOptions(opts EntryOptions) EntryOptions {
	out := opts
	if opts.Inject != nil {
		out.Inject = cloneMap(opts.Inject)
	}
	if opts.Intercept != nil {
		out.Intercept = cloneMap(opts.Intercept)
	}
	if opts.Isolate != nil {
		out.Isolate = cloneMap(opts.Isolate)
	}
	return out
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func mapsEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || !reflect.DeepEqual(v, bv) {
			return false
		}
	}
	return true
}

func optionsEqual(a, b EntryOptions) bool {
	return a.ID == b.ID && a.Name == b.Name && a.Group == b.Group && a.Disabled == b.Disabled &&
		reflect.DeepEqual(a.Config, b.Config) &&
		mapsEqual(a.Inject, b.Inject) &&
		mapsEqual(a.Intercept, b.Intercept) &&
		mapsEqual(a.Isolate, b.Isolate)
}

// randomID returns a collision-resistant short hex id, mirroring the random
// anonymous ids upstream.
func randomID() string {
	b := make([]byte, 4)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		panic(fmt.Sprintf("loader: cannot generate id: %v", err))
	}
	return hex.EncodeToString(b)
}

// EntryError reports one entry's validation or start failure.
type EntryError struct {
	ID   string
	Name string
	Err  error
}

func (e *EntryError) Error() string {
	return fmt.Sprintf("loader: entry %s (%s): %v", e.ID, e.Name, e.Err)
}

func (e *EntryError) Unwrap() error { return e.Err }

// joinedEntryErrors formats per-entry failures without wrapping them away.
func joinedEntryErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}
