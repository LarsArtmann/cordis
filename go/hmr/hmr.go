// Package hmr hot-swaps plugin implementations in a running loader tree,
// porting packages/hmr upstream.
//
// A module is a named plugin implementation in the loader's Resolver.
// Swap replaces the implementation and relinks every live entry whose
// plugin is the swapped module or transitively imports it: the old fiber
// is disposed (running its cleanups) and a fresh fiber starts from the new
// implementation with the entry's config and identity preserved. The
// module's generation counter advances per applied swap, giving every
// implementation an observable identity. A swap that fails to start rolls
// the registration back, so the previous implementation keeps working.
//
// Native divergences from the TypeScript original (see PORTS.md):
//
//   - Go is AOT compiled, so a "changed file" becomes a changed factory
//     value. Observing the outside world (files, control planes, test
//     rigs) and turning it into Swap calls is the embedder's job; the
//     package owns classification, relink and rollback.
//   - Upstream derives the dependency graph from ESM imports and
//     classifies files into accepted/declined sets. Go declares the graph
//     with Declare; every module that reaches the swapped module through
//     it accepts the change and reloads, everything else declines by
//     omission and keeps running.
package hmr

import (
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sync"

	cordis "github.com/LarsArtmann/cordis/go"
	"github.com/LarsArtmann/cordis/go/loader"
)

// EventReload fires after a swap applied cleanly. Args: (Report). It
// mirrors the hmr/reload event upstream.
const EventReload = "hmr/reload"

// Report describes one applied swap.
type Report struct {
	// Module is the swapped module's name.
	Module string
	// Generation is the module's implementation generation after the
	// swap; it starts at 1 for the first applied swap.
	Generation uint64
	// Reloaded lists the entry ids that were relinked onto the new
	// implementation, in reload order.
	Reloaded []string
}

// Manager swaps plugin implementations in a Resolver and relinks the live
// entries of a Tree. The zero value is not usable; create one with New.
// Swaps serialize through an internal mutex, so concurrent calls apply one
// at a time.
type Manager struct {
	resolver *loader.Resolver
	tree     *loader.Tree

	mu       sync.Mutex
	swapping sync.Mutex
	imports  map[string][]string
	gens     map[string]uint64
}

// New creates a Manager operating on the given resolver and tree.
func New(resolver *loader.Resolver, tree *loader.Tree) *Manager {
	return &Manager{
		resolver: resolver,
		tree:     tree,
		imports:  make(map[string][]string),
		gens:     make(map[string]uint64),
	}
}

// Declare records the modules that name depends on. The graph decides who
// reloads: a swap of m relinks every entry whose plugin reaches m through
// these edges.
func (m *Manager) Declare(name string, imports ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.imports[name] = slices.Clone(imports)
}

// Generation returns the module's implementation generation: the number of
// applied swaps so far. A rolled-back swap does not count.
func (m *Manager) Generation(name string) uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.gens[name]
}

// Swap replaces the implementation of name and relinks every live entry
// whose plugin accepts the change. On failure the previous registration is
// restored, every touched entry is relinked back onto it, and the returned
// error joins the start failures.
func (m *Manager) Swap(name string, reg loader.Registration) (Report, error) {
	m.swapping.Lock()
	defer m.swapping.Unlock()

	m.mu.Lock()
	previous, found, err := m.resolver.Replace(name, reg)
	if err != nil {
		m.mu.Unlock()
		return Report{}, err
	}
	m.gens[name]++
	report := Report{Module: name, Generation: m.gens[name]}
	affected := m.reach(name)
	m.mu.Unlock()

	var touched []string
	for _, e := range m.tree.Entries() {
		if !slices.Contains(affected, e.Name()) {
			continue
		}
		if e.Fiber() == nil || e.Disabled() {
			continue
		}
		id := e.ID()
		touched = append(touched, id)
		if err := m.tree.Refresh(id); err != nil {
			return Report{Module: name}, m.rollback(name, previous, found, touched, err)
		}
		// Apply errors do not cross the framework boundary: a plugin
		// whose apply failed leaves its fiber in StateFailed.
		if f := e.Fiber(); f != nil && f.State() == cordis.StateFailed {
			return Report{Module: name}, m.rollback(name, previous, found, touched,
				fmt.Errorf("hmr: entry %s failed under the new implementation", id))
		}
	}
	report.Reloaded = touched

	m.tree.Context().Emit(EventReload, report)
	return report, nil
}

// SwapType is the typed common case of Swap, mirroring
// loader.RegisterType: it builds the registration from a typed apply
// function and swaps it in.
func SwapType[C any](m *Manager, name string, apply func(ctx *cordis.Context, config C) error) (Report, error) {
	return m.Swap(name, loader.Registration{
		New: func() cordis.PluginHandle { return cordis.NewPlugin(name, apply) },
		Decode: func(raw any) (any, error) {
			return loader.DecodeInto[C](raw)
		},
	})
}

// rollback restores the previous registration and relinks every entry the
// swap already touched back onto it, mirroring the upstream rollback of
// failed re-imports. The generation does not count the failed swap.
func (m *Manager) rollback(name string, previous loader.Registration, found bool, reloaded []string, cause error) error {
	m.mu.Lock()
	if found {
		_, _, _ = m.resolver.Replace(name, previous)
	}
	m.gens[name]--
	m.mu.Unlock()

	errs := []error{fmt.Errorf("hmr: swap of %s failed: %w", name, cause)}
	for _, id := range reloaded {
		if err := m.tree.Refresh(id); err != nil {
			errs = append(errs, fmt.Errorf("hmr: rollback of entry %s: %w", id, err))
			slog.Warn("hmr: rollback could not restore entry", "id", id, "err", err)
		}
	}
	return errors.Join(errs...)
}

// reach returns the modules that depend on name directly or transitively,
// including name itself, sorted. It is the accept set of a swap of name.
func (m *Manager) reach(name string) []string {
	reached := map[string]bool{name: true}
	for changed := true; changed; {
		changed = false
		for mod, imports := range m.imports {
			if reached[mod] {
				continue
			}
			if slices.ContainsFunc(imports, func(dep string) bool { return reached[dep] }) {
				reached[mod] = true
				changed = true
			}
		}
	}
	return slices.Sorted(maps.Keys(reached))
}
