package loader

import (
	"fmt"
	"maps"
	"reflect"
	"slices"

	cordis "github.com/LarsArtmann/cordis/go"
)

// update applies opts to the entry. With create the options are replaced
// wholesale; otherwise non-zero fields are merged over the current options.
// force skips the no-difference early return. It starts, restarts or
// disposes the entry's fiber as the new options require. Uncomparable
// isolate labels are rejected before any option is stored or fiber is
// touched, so a rejected update leaves the entry exactly as it was.
func (e *Entry) update(opts EntryOptions, create, force bool) error {
	t := e.parent.tree
	t.mu.Lock()

	legacy := e.opts

	next := opts
	if !create {
		next = mergeOptions(legacy, opts)
	}

	if err := validateIsolateLabels(next.Name, next.Isolate); err != nil {
		t.mu.Unlock()

		return err
	}

	if create {
		e.opts = cloneOptions(opts)
	} else {
		e.opts = next
	}

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

	ectx, err := e.buildContext()
	if err != nil {
		return fmt.Errorf("loader: entry %q: %w", e.opts.Name, err)
	}

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
func (e *Entry) buildContext() (*cordis.Context, error) {
	ectx := e.parent.host.Extend()
	for _, name := range slices.Sorted(maps.Keys(e.opts.Intercept)) {
		ectx = ectx.Intercept(name, e.opts.Intercept[name])
	}

	for _, name := range slices.Sorted(maps.Keys(e.opts.Isolate)) {
		switch label := e.opts.Isolate[name].(type) {
		case nil, bool:
			child, err := ectx.Isolate(name)
			if err != nil {
				return nil, err
			}

			ectx = child
		default:
			child, err := ectx.Isolate(name, label)
			if err != nil {
				return nil, err
			}

			ectx = child
		}
	}

	return ectx, nil
}
