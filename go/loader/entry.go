package loader

import (
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

// Err returns the entry's most recent failure: the error that prevented
// the entry from starting, or the error of a fiber that failed after a
// successful start (a failed restart or apply). A later successful start
// clears it.
func (e *Entry) Err() error {
	t := e.parent.tree
	t.mu.Lock()
	defer t.mu.Unlock()

	return e.err
}

// recordError routes a fiber failure observed outside the start path (for
// example by Tree.Await) into the entry's error sink. The sink is
// last-writer-wins: every recorded failure replaces the previous one, so
// when two reloads fail concurrently the surviving error is whichever
// observation landed last, not the first. A later successful start clears
// the sink (init and the start path write nil on success).
func (e *Entry) recordError(err error) {
	t := e.parent.tree
	t.mu.Lock()
	e.err = err
	t.mu.Unlock()
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
