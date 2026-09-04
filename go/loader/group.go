package loader

import (
	"slices"

	cordis "github.com/LarsArtmann/cordis/go"
)

// EntryGroup owns a list of entry options and the entries created from
// them. The root group of a Tree has no fiber of its own; nested groups are
// run by the group plugin, which diffs its entries in place on config
// updates and stops them when the hosting entry is disposed.
type EntryGroup struct {
	tree      *Tree
	host      *cordis.Context
	hostEntry *Entry

	// data is guarded by tree.mu.
	data []EntryOptions
}

// Tree returns the tree this group belongs to.
func (g *EntryGroup) Tree() *Tree { return g.tree }

// Data returns a snapshot of the group's configured options.
func (g *EntryGroup) Data() []EntryOptions {
	t := g.tree
	t.mu.Lock()
	defer t.mu.Unlock()
	return cloneEntries(g.data)
}

// Entries returns the entries the group currently manages, in config
// order.
func (g *EntryGroup) Entries() []*Entry {
	t := g.tree
	t.mu.Lock()
	ids := make([]string, 0, len(g.data))
	for _, o := range g.data {
		if o.ID != "" {
			ids = append(ids, o.ID)
		}
	}
	t.mu.Unlock()

	out := make([]*Entry, 0, len(ids))
	for _, id := range ids {
		e := t.lookupEntry(id)
		if e != nil && e.parent == g {
			out = append(out, e)
		}
	}
	return out
}

// Create appends opts to the group config and starts the entry.
func (g *EntryGroup) Create(opts EntryOptions) (string, error) {
	t := g.tree
	t.mu.Lock()
	id := t.ensureIDLocked(&opts)
	g.data = append(g.data, cloneOptions(opts))
	t.mu.Unlock()
	return id, g.createEntry(opts, id)
}

// createEntry registers the entry in the store and applies its options.
func (g *EntryGroup) createEntry(opts EntryOptions, id string) error {
	t := g.tree
	t.mu.Lock()
	entry := t.store[id]
	if entry == nil {
		entry = &Entry{parent: g}
		t.store[id] = entry
		t.order = append(t.order, id)
	}
	entry.parent = g
	t.mu.Unlock()
	return entry.update(opts, true, true)
}

// Remove disposes the entry with the given id and deletes it from the
// store. Unless isDispose is set, the options are also removed from the
// group config.
func (g *EntryGroup) Remove(id string, isDispose bool) {
	t := g.tree
	t.mu.Lock()
	entry := t.store[id]
	if entry == nil || entry.parent != g {
		t.mu.Unlock()
		return
	}
	if !isDispose {
		g.data = removeEntryOption(g.data, id)
	}
	delete(t.store, id)
	t.removeOrderLocked(id)
	opts := cloneOptions(entry.opts)
	t.mu.Unlock()

	entry.dispose()
	t.ctx.Emit(EventPartialDispose, id, opts, false)
}

// Stop disposes every entry the group manages without rewriting the config,
// mirroring the group's disposal cleanup upstream.
func (g *EntryGroup) Stop() {
	t := g.tree
	t.mu.Lock()
	ids := make([]string, 0, len(g.data))
	for _, o := range g.data {
		if o.ID != "" {
			ids = append(ids, o.ID)
		}
	}
	t.mu.Unlock()
	for _, id := range ids {
		g.Remove(id, true)
	}
}

// Update replaces the group config and diffs it against the previous one:
// new entries are created, missing entries are removed and existing entries
// are reconciled in place, so unaffected entries keep running.
func (g *EntryGroup) Update(config []EntryOptions) error {
	t := g.tree
	t.mu.Lock()
	old := g.data
	g.data = cloneEntries(config)
	oldIDs := make(map[string]bool, len(old))
	for _, o := range old {
		if o.ID != "" {
			oldIDs[o.ID] = true
		}
	}
	t.mu.Unlock()

	keep := make(map[string]bool, len(config))
	var errs []error
	for _, opts := range config {
		id := opts.ID
		if id != "" && oldIDs[id] {
			if e := t.lookupEntry(id); e != nil && e.parent == g {
				if err := e.update(opts, true, true); err != nil {
					errs = append(errs, err)
				}
				keep[id] = true
				continue
			}
		}
		// The data slice already contains the new config; createEntry only
		// registers and starts the entry.
		t.mu.Lock()
		id = t.ensureIDLocked(&opts)
		t.mu.Unlock()
		if err := g.createEntry(opts, id); err != nil {
			errs = append(errs, err)
		}
		keep[id] = true
	}

	for _, id := range t.entryIDs() {
		e := t.lookupEntry(id)
		if e == nil || e.parent != g {
			continue
		}
		if !keep[id] {
			g.Remove(id, false)
		}
	}
	return joinedEntryErrors(errs)
}

// groupHandle builds the group plugin for one hosting entry. The factory
// runs per start, so the closure always addresses a live entry binding.
func (t *Tree) groupHandle(e *Entry) cordis.PluginHandle {
	return cordis.NewPlugin(GroupName, func(ctx *cordis.Context, config []EntryOptions) error {
		return t.runGroup(ctx, e, config)
	})
}

// runGroup adopts or creates the hosting entry's group, applies the initial
// config, registers the disposal cleanup and vetoes config-update restarts
// in favor of an in-place entry diff. Without the veto, a Go config update
// would unload and reload the group fiber; with it, unaffected entries keep
// running, matching upstream semantics.
func (t *Tree) runGroup(ctx *cordis.Context, host *Entry, config []EntryOptions) error {
	t.mu.Lock()
	g := host.subgroup
	if g == nil {
		g = &EntryGroup{tree: t, host: host.ctx, hostEntry: host}
		host.subgroup = g
	} else {
		g.host = host.ctx
	}
	t.mu.Unlock()

	if err := g.Update(config); err != nil {
		t.log("group " + host.ID() + ": " + err.Error())
	}
	if _, err := ctx.Cleanup(GroupName, g.Stop); err != nil {
		return err
	}
	_, err := ctx.On(cordis.EventUpdate, func(args ...any) any {
		f, _ := args[0].(*cordis.Fiber)
		raw := args[1]
		noSave, _ := args[2].(bool)
		next, ok := args[3].(func(...any) any)
		if !ok {
			return nil
		}
		if f == nil || f != ctx.Fiber() {
			// Not this group's own config update; pass it through.
			return next(args...)
		}
		cfg, derr := DecodeInto[[]EntryOptions](raw)
		if derr != nil {
			return next(args...)
		}
		if !noSave {
			t.mu.Lock()
			host.opts.Config = cfg
			t.mu.Unlock()
			t.Write()
		}
		if uerr := g.Update(cfg); uerr != nil {
			t.log("group " + host.ID() + ": " + uerr.Error())
		}
		return nil
	})
	return err
}

// removeEntryOption drops the option with the given id from a slice.
func removeEntryOption(config []EntryOptions, id string) []EntryOptions {
	idx := slices.IndexFunc(config, func(o EntryOptions) bool { return o.ID == id })
	if idx < 0 {
		return config
	}
	return slices.Delete(config, idx, idx+1)
}

func cloneEntries(config []EntryOptions) []EntryOptions {
	out := make([]EntryOptions, len(config))
	for i, o := range config {
		out[i] = cloneOptions(o)
	}
	return out
}
