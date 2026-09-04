// Package group ports the upstream group plugin: it manages a set of named
// child fibers with diffed updates. Create, remove and update child
// entries by id; the group disposes every remaining child when its own
// scope rolls back.
//
//	g := group.Start(ctx)
//	g.Create("worker-1", func(c *cordis.Context) error { ... })
//	g.Update(map[string]group.Factory{"worker-2": factory}) // removes worker-1
package group

import (
	"fmt"
	"sort"
	"sync"

	cordis "github.com/LarsArtmann/cordis/go"
)

// ServiceName is the canonical service name of the group service.
const ServiceName = "group"

// Factory starts the plugins of one child entry on the child scope given
// to it. Returning an error fails the child fiber.
type Factory func(ctx *cordis.Context) error

// Group is the child-entry manager published under ServiceName.
type Group struct {
	mu     sync.Mutex
	ctx    *cordis.Context
	fibers map[string]*entry
}

type entry struct {
	fiber *cordis.Fiber
}

// Start installs a group on the context tree. The group's children are
// disposed when the installing fiber unloads or is disposed.
func Start(ctx *cordis.Context) (*Group, error) {
	g := &Group{ctx: ctx, fibers: make(map[string]*entry)}
	if _, err := ctx.Provide(ServiceName, g); err != nil {
		return nil, err
	}
	return g, nil
}

// Create starts a child entry with the given id. It fails when the id is
// already taken or the factory errors; a failed factory leaves the group
// unchanged.
func (g *Group) Create(id string, factory Factory) error {
	g.mu.Lock()
	if _, ok := g.fibers[id]; ok {
		g.mu.Unlock()
		return fmt.Errorf("group: entry %q already exists", id)
	}
	g.mu.Unlock()

	scope := g.ctx.Extend()
	fiber, err := cordis.Start(scope, cordis.NewPlugin("group:"+id, func(c *cordis.Context, _ struct{}) error {
		return factory(c)
	}), struct{}{})
	if err != nil {
		return err
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.fibers[id]; ok {
		// Raced with another Create; drop the newcomer.
		fiber.Dispose()
		return fmt.Errorf("group: entry %q already exists", id)
	}
	g.fibers[id] = &entry{fiber: fiber}
	return nil
}

// Remove disposes the child entry with the given id, if present.
func (g *Group) Remove(id string) {
	g.mu.Lock()
	e, ok := g.fibers[id]
	delete(g.fibers, id)
	g.mu.Unlock()
	if ok {
		e.fiber.Dispose()
	}
}

// Get reports whether the child entry with the id has a live fiber. A
// child whose factory failed counts as not live.
func (g *Group) Get(id string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	e := g.fibers[id]
	return e != nil && e.fiber.State() == cordis.StateActive
}

// State returns the lifecycle state of the child entry's fiber.
func (g *Group) State(id string) cordis.FiberState {
	g.mu.Lock()
	defer g.mu.Unlock()
	if e := g.fibers[id]; e != nil {
		return e.fiber.State()
	}
	return cordis.StateDisposed
}

// IDs returns the sorted ids of all live child entries.
func (g *Group) IDs() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	ids := make([]string, 0, len(g.fibers))
	for id := range g.fibers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Update diffs the group against the wanted set: entries missing from the
// map are removed, entries present are created (or kept). It returns after
// every change has settled.
func (g *Group) Update(wanted map[string]Factory) error {
	g.mu.Lock()
	var stale []string
	for id := range g.fibers {
		if _, ok := wanted[id]; !ok {
			stale = append(stale, id)
		}
	}
	var fresh []string
	for id := range wanted {
		if _, ok := g.fibers[id]; !ok {
			fresh = append(fresh, id)
		}
	}
	g.mu.Unlock()

	for _, id := range stale {
		g.Remove(id)
	}
	for _, id := range fresh {
		if err := g.Create(id, wanted[id]); err != nil {
			return err
		}
	}
	return nil
}
