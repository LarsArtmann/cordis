// Package loader provides config-driven plugin management for cordis,
// porting packages/loader upstream. Entries describe plugins by name and
// config; a Tree starts, updates and disposes them, and a Resolver maps
// entry names to plugin factories.
//
// Native divergences from the TypeScript original (see PORTS.md):
//
//   - Go has no dynamic imports, so entries resolve plugins through a
//     Resolver registry instead of module imports. Factories must return a
//     fresh plugin value per start.
//   - Fiber.Update in Go genuinely unloads and reloads a fiber. Group
//     config updates therefore veto the restart through the internal/update
//     waterfall and diff their entries in place, exactly like the upstream
//     Group listener, so unaffected entries keep running.
//   - Entry isolate/intercept options map directly onto the core's native
//     Context.Isolate and Context.Intercept scopes.
package loader

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	cordis "github.com/LarsArtmann/cordis/go"
)

// Registration describes one resolvable entry kind: a factory producing a
// fresh plugin per start and an optional decoder turning raw config (the
// JSON-shaped maps and slices a config file yields) into the plugin's typed
// config. A nil Decode passes the raw value through.
type Registration struct {
	New    func() cordis.PluginHandle
	Decode func(raw any) (any, error)
}

// Resolver maps entry names to plugin registrations. The zero value is not
// usable; create one with NewResolver. All methods are safe for concurrent
// use.
type Resolver struct {
	mu      sync.Mutex
	entries map[string]Registration
}

// NewResolver creates an empty resolver.
func NewResolver() *Resolver {
	return &Resolver{entries: make(map[string]Registration)}
}

// Register adds a registration for name. It returns an error when the name
// is already registered.
func (r *Resolver) Register(name string, reg Registration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.entries[name]; ok {
		return fmt.Errorf("loader: plugin %q is already registered", name)
	}
	if reg.New == nil {
		return fmt.Errorf("loader: plugin %q registered without a factory", name)
	}
	r.entries[name] = reg
	return nil
}

// MustRegister is Register but panics on error. It is meant for programmatic
// setup where a duplicate is a programming mistake.
func (r *Resolver) MustRegister(name string, reg Registration) {
	if err := r.Register(name, reg); err != nil {
		panic(err)
	}
}

// RegisterType registers a plugin with a typed apply function and a decoder
// that normalizes raw config into C. It is the common case:
//
//	resolver.RegisterType[*Conf]("echo", func(ctx *cordis.Context, conf *Conf) error { ... })
func RegisterType[C any](r *Resolver, name string, apply func(ctx *cordis.Context, config C) error) {
	r.MustRegister(name, Registration{
		New: func() cordis.PluginHandle { return cordis.NewPlugin(name, apply) },
		Decode: func(raw any) (any, error) {
			return DecodeInto[C](raw)
		},
	})
}

// Resolve builds a fresh plugin handle for name.
func (r *Resolver) Resolve(name string) (cordis.PluginHandle, error) {
	reg, err := r.lookup(name)
	if err != nil {
		return nil, err
	}
	return reg.New(), nil
}

// Decode normalizes raw config for name. When the registration declares no
// decoder the raw value passes through unchanged.
func (r *Resolver) Decode(name string, raw any) (any, error) {
	reg, err := r.lookup(name)
	if err != nil {
		return nil, err
	}
	if reg.Decode == nil {
		return raw, nil
	}
	return reg.Decode(raw)
}

func (r *Resolver) lookup(name string) (Registration, error) {
	r.mu.Lock()
	reg, ok := r.entries[name]
	r.mu.Unlock()
	if !ok {
		return Registration{}, fmt.Errorf("loader: unknown plugin %q", name)
	}
	return reg, nil
}

// DecodeInto converts a raw, JSON-shaped value into C. Values already
// assignable to C pass through untouched; everything else goes through a
// JSON round-trip, so config files decoded into map[string]any reach typed
// plugins without manual conversion.
func DecodeInto[C any](raw any) (C, error) {
	var out C
	if raw != nil {
		if rt := reflect.TypeOf(raw); rt.AssignableTo(reflect.TypeOf(&out).Elem()) {
			return raw.(C), nil
		}
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return out, fmt.Errorf("loader: cannot encode config: %w", err)
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, fmt.Errorf("loader: cannot decode config: %w", err)
	}
	return out, nil
}
