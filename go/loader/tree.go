package loader

import (
	"fmt"
	"log/slog"
	"slices"
	"sync"

	cordis "github.com/LarsArtmann/cordis/go"
)

// GroupName is the plugin name of the group builtin.
const GroupName = "group"

// Tree owns the entries of one config scope. It mirrors upstream EntryTree:
// the root group holds the configured entries, entries created through it
// live in the store, and a write hook persists config changes.
//
// The zero value is not usable; create trees with NewTree. All methods are
// safe for concurrent use. The tree mutex guards its own bookkeeping and
// every entry's options; it is never held across framework calls, so
// reloads serialize through their callers.
type Tree struct {
	owner    *Entry
	ctx      *cordis.Context
	resolver *Resolver

	// EnableLogs promotes lifecycle logs from debug to info level.
	EnableLogs bool

	mu        sync.Mutex
	store     map[string]*Entry
	order     []string
	fibers    map[*cordis.Fiber]*Entry
	root      *EntryGroup
	writeHook func(data []EntryOptions)
}

// NewTree creates a tree rooted at ctx. A nil resolver creates an empty
// one. The tree observes plugin disposal globally: when an entry's fiber is
// disposed from outside the loader (plugin self-disposal), the entry is
// marked disabled so a reload will not resurrect it.
func NewTree(ctx *cordis.Context, resolver *Resolver) *Tree {
	if resolver == nil {
		resolver = NewResolver()
	}
	t := &Tree{
		ctx:      ctx,
		resolver: resolver,
		store:    make(map[string]*Entry),
		fibers:   make(map[*cordis.Fiber]*Entry),
	}
	t.root = &EntryGroup{tree: t, host: ctx}
	if _, err := ctx.On(cordis.EventPlugin, t.onPlugin, cordis.Global()); err != nil {
		slog.Warn("loader: cannot observe plugin disposal", "err", err)
	}
	// One global config save listener, mirroring the upstream loader's
	// global prepend internal/update listener: updates from tracked fibers
	// are written back into the entry options and persisted.
	if _, err := ctx.On(cordis.EventUpdate, t.onConfigUpdate, cordis.Global(), cordis.Prepend()); err != nil {
		slog.Warn("loader: cannot observe config updates", "err", err)
	}
	return t
}

// onConfigUpdate saves the new config of a tracked fiber into its entry
// options and forwards the update unchanged.
func (t *Tree) onConfigUpdate(args ...any) any {
	f, _ := args[0].(*cordis.Fiber)
	config := args[1]
	noSave, _ := args[2].(bool)
	next, ok := args[3].(func(...any) any)
	if !ok {
		return nil
	}
	t.mu.Lock()
	e := t.fibers[f]
	if e != nil && !noSave {
		e.opts.Config = config
	}
	t.mu.Unlock()
	if e != nil && !noSave {
		t.Write()
	}
	return next(f, config, noSave)
}

// Root returns the root entry group.
func (t *Tree) Root() *EntryGroup { return t.root }

// Context returns the host context of the root group.
func (t *Tree) Context() *cordis.Context { return t.ctx }

// Entries returns every entry in the store, in creation order.
func (t *Tree) Entries() []*Entry {
	t.mu.Lock()
	ids := slices.Clone(t.order)
	t.mu.Unlock()
	out := make([]*Entry, 0, len(ids))
	for _, id := range ids {
		if e := t.lookupEntry(id); e != nil {
			out = append(out, e)
		}
	}
	return out
}

// Lookup returns the entry with the given local id.
func (t *Tree) Lookup(id string) (*Entry, bool) {
	e := t.lookupEntry(id)
	return e, e != nil
}

func (t *Tree) lookupEntry(id string) *Entry {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.store[id]
}

// Resolve resolves a (possibly hierarchical) id to an entry. Hierarchies
// span subtrees, which the file loader introduces; within one tree ids are
// flat.
func (t *Tree) Resolve(id string) (*Entry, error) {
	e := t.lookupEntry(id)
	if e == nil {
		return nil, fmt.Errorf("loader: cannot resolve entry %s", id)
	}
	return e, nil
}

// ResolveGroup resolves the group hosted by the entry with the given id, or
// the root group for the empty id.
func (t *Tree) ResolveGroup(id string) (*EntryGroup, error) {
	if id == "" {
		return t.root, nil
	}
	e, err := t.Resolve(id)
	if err != nil {
		return nil, err
	}
	g := e.Subgroup()
	if g == nil {
		return nil, fmt.Errorf("loader: entry %s is not a group", id)
	}
	return g, nil
}

// Create adds opts to the group identified by parentID (root when empty) at
// the given position, or appended when pos is negative, and starts the
// entry.
func (t *Tree) Create(opts EntryOptions, parentID string, pos int) (string, error) {
	g, err := t.ResolveGroup(parentID)
	if err != nil {
		return "", err
	}
	t.mu.Lock()
	id := t.ensureIDLocked(&opts)
	if pos < 0 || pos >= len(g.data) {
		g.data = append(g.data, cloneOptions(opts))
	} else {
		g.data = slices.Insert(g.data, pos, cloneOptions(opts))
	}
	t.mu.Unlock()
	if err := g.createEntry(opts, id); err != nil {
		return id, err
	}
	t.Write()
	return id, nil
}

// Remove disposes the entry with the given id and drops it from its group
// config.
func (t *Tree) Remove(id string) error {
	e, err := t.Resolve(id)
	if err != nil {
		return err
	}
	e.parent.Remove(id, false)
	t.Write()
	return nil
}

// Update merges the non-zero fields of opts into the entry's options and
// reconciles the entry.
func (t *Tree) Update(id string, opts EntryOptions) error {
	e, err := t.Resolve(id)
	if err != nil {
		return err
	}
	if err := e.update(opts, false, false); err != nil {
		return err
	}
	t.Write()
	return nil
}

// Replace overwrites the entry's options wholesale and reconciles it.
func (t *Tree) Replace(id string, opts EntryOptions) error {
	e, err := t.Resolve(id)
	if err != nil {
		return err
	}
	opts.ID = id
	if err := e.update(opts, true, true); err != nil {
		return err
	}
	t.Write()
	return nil
}

// SetConfig restarts the entry with a new config in place. For group
// entries the update is vetoed by the group plugin and diffed across the
// managed entries instead.
func (t *Tree) SetConfig(id string, config any) error {
	e, err := t.Resolve(id)
	if err != nil {
		return err
	}
	return e.update(EntryOptions{Config: config}, false, true)
}

// Move relocates an entry into another group without restarting it.
func (t *Tree) Move(id, parentID string, pos int) error {
	e, err := t.Resolve(id)
	if err != nil {
		return err
	}
	target, err := t.ResolveGroup(parentID)
	if err != nil {
		return err
	}
	source := e.parent
	if source == target {
		return nil
	}
	t.mu.Lock()
	source.data = removeEntryOption(source.data, id)
	opts := cloneOptions(e.opts)
	if pos < 0 || pos >= len(target.data) {
		target.data = append(target.data, opts)
	} else {
		target.data = slices.Insert(target.data, pos, opts)
	}
	e.parent = target
	t.mu.Unlock()
	t.Write()
	return nil
}

// Await waits until every in-flight entry fiber has settled. Fibers parked
// in StatePending for missing dependencies are not in flight: they settle
// when their dependencies arrive, and callers can Await again afterwards.
// Fibers created while waiting (through group diffs) are awaited too.
func (t *Tree) Await() {
	for {
		seen := make(map[*cordis.Fiber]bool)
		for _, e := range t.Entries() {
			if f := e.Fiber(); f != nil && f.State() != cordis.StatePending {
				seen[f] = true
				_ = f.Await()
			}
		}
		grew := false
		for _, e := range t.Entries() {
			if f := e.Fiber(); f != nil && !seen[f] && f.State() != cordis.StatePending {
				grew = true
				break
			}
		}
		if !grew {
			return
		}
	}
}

// Errors returns a snapshot of the per-entry start failures.
func (t *Tree) Errors() map[string]error {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make(map[string]error)
	for id, e := range t.store {
		if e.err != nil {
			out[id] = e.err
		}
	}
	return out
}

// Validate checks every enabled entry without starting it: the plugin name
// must resolve and the config must decode into the plugin's typed config.
// Failures are reported per entry.
func (t *Tree) Validate() []error {
	t.mu.Lock()
	ids := slices.Clone(t.order)
	t.mu.Unlock()

	var errs []error
	for _, id := range ids {
		e := t.lookupEntry(id)
		if e == nil || e.Disabled() {
			continue
		}
		_, reg, err := t.resolveFor(e, e.Name())
		if err == nil && reg.Decode != nil {
			_, err = reg.Decode(e.Options().Config)
		}
		if err != nil {
			errs = append(errs, &EntryError{ID: id, Name: e.Name(), Err: err})
		}
	}
	return errs
}

// SetWriteHook installs the persistence callback. It receives a snapshot of
// the root group config on every config change and runs outside the tree
// lock.
func (t *Tree) SetWriteHook(hook func(data []EntryOptions)) {
	t.mu.Lock()
	t.writeHook = hook
	t.mu.Unlock()
}

// Write invokes the write hook with a snapshot of the root config.
func (t *Tree) Write() {
	t.mu.Lock()
	hook := t.writeHook
	var data []EntryOptions
	if hook != nil {
		data = cloneEntries(t.root.data)
	}
	t.mu.Unlock()
	if hook != nil {
		hook(data)
	}
}

// Export returns a snapshot of the root group config.
func (t *Tree) Export() []EntryOptions {
	t.mu.Lock()
	defer t.mu.Unlock()
	return cloneEntries(t.root.data)
}

// Locate returns the id of the entry owning the fiber, if any.
func (t *Tree) Locate(f *cordis.Fiber) (string, bool) {
	t.mu.Lock()
	e := t.fibers[f]
	t.mu.Unlock()
	if e == nil {
		return "", false
	}
	return e.ID(), true
}

// Close disposes every entry of the tree.
func (t *Tree) Close() {
	t.root.Stop()
}

// onPlugin observes global fiber creation and disposal. A disposal of a
// tracked fiber that the loader did not initiate is a plugin self-disposal:
// the entry is marked disabled and persisted so reloads keep it off.
func (t *Tree) onPlugin(args ...any) any {
	f, _ := args[0].(*cordis.Fiber)
	if f == nil {
		return nil
	}
	t.mu.Lock()
	e := t.fibers[f]
	tracked := e != nil && e.fiber == f
	var id string
	var opts EntryOptions
	if tracked {
		id = e.opts.ID
		opts = cloneOptions(e.opts)
		delete(t.fibers, f)
		e.fiber = nil
	}
	t.mu.Unlock()
	if !tracked {
		return nil
	}
	t.markDisabled(e)
	t.log("unload plugin " + opts.Name)
	t.ctx.Emit(EventPartialDispose, id, opts, true)
	t.Write()
	return nil
}

// markDisabled flags an entry and its group config entry as disabled so
// reloads keep it off. It takes the tree lock.
func (t *Tree) markDisabled(e *Entry) {
	t.mu.Lock()
	e.opts.Disabled = true
	g := e.parent
	for i := range g.data {
		if g.data[i].ID == e.opts.ID {
			g.data[i].Disabled = true
			break
		}
	}
	t.mu.Unlock()
}

// resolveFor resolves the plugin for one entry. Built-in group names fall
// back to the tree's own group plugin unless a user registration shadows
// them.
func (t *Tree) resolveFor(e *Entry, name string) (cordis.PluginHandle, Registration, error) {
	if reg, err := t.resolver.lookup(name); err == nil {
		return reg.New(), reg, nil
	}
	if name == "group" || name == GroupBuiltin {
		reg := Registration{
			New: func() cordis.PluginHandle { return t.groupHandle(e) },
			Decode: func(raw any) (any, error) {
				return DecodeInto[[]EntryOptions](raw)
			},
		}
		return reg.New(), reg, nil
	}
	return nil, Registration{}, fmt.Errorf("loader: unknown plugin %q", name)
}

func (t *Tree) ensureIDLocked(opts *EntryOptions) string {
	if opts.ID != "" {
		return opts.ID
	}
	for {
		id := randomID()
		if _, taken := t.store[id]; !taken {
			opts.ID = id
			return id
		}
	}
}

func (t *Tree) removeOrderLocked(id string) {
	if idx := slices.Index(t.order, id); idx >= 0 {
		t.order = slices.Delete(t.order, idx, idx+1)
	}
}

func (t *Tree) entryIDs() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return slices.Clone(t.order)
}

func (t *Tree) log(msg string, args ...any) {
	t.mu.Lock()
	verbose := t.EnableLogs
	t.mu.Unlock()
	if verbose {
		slog.Info(msg, args...)
	} else {
		slog.Debug(msg, args...)
	}
}
