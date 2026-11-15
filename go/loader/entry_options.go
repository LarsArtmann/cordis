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
)

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

// validateIsolateLabels rejects uncomparable isolate labels before any
// option is stored or fiber is started. Context.Isolate reports the same
// condition at start time, after the entry context has already been built;
// validating the merged options up front names the failing entry and its
// isolate key precisely and leaves the entry untouched. nil and bool labels
// select a fresh local realm and carry no identity, mirroring the
// buildContext switch.
func validateIsolateLabels(entry string, isolate map[string]any) error {
	for _, name := range slices.Sorted(maps.Keys(isolate)) {
		switch label := isolate[name].(type) {
		case nil, bool:
			continue
		default:
			if !reflect.TypeOf(label).Comparable() {
				return fmt.Errorf(
					"loader: entry %q: isolate[%q]: label of type %T is not comparable",
					entry,
					name,
					label,
				)
			}
		}
	}

	return nil
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
	maps.Copy(out, m)

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
// anonymous ids upstream. It fails only when the system entropy source is
// unreadable.
func randomID() (string, error) {
	b := make([]byte, 4)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", fmt.Errorf("loader: cannot generate id: %w", err)
	}

	return hex.EncodeToString(b), nil
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
