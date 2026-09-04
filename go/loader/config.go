package loader

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	cordis "github.com/LarsArtmann/cordis/go"
)

// DefaultConfigName is the config file the file loader discovers.
const DefaultConfigName = "cordis.json"

// DecodeConfig parses a config file body into entry options. Both a bare
// entry array and an object with a "plugins" array are accepted. Decoders
// for other formats (YAML, TOML) can wrap their parser and hand the result
// to a Tree directly; the loader itself only needs stdlib JSON.
func DecodeConfig(data []byte) ([]EntryOptions, error) {
	var arr []EntryOptions
	if err := json.Unmarshal(data, &arr); err == nil {
		return arr, nil
	}
	var doc struct {
		Plugins []EntryOptions `json:"plugins"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("loader: config must be an entry array or {\"plugins\":[...]}: %w", err)
	}
	return doc.Plugins, nil
}

// EncodeConfig renders entry options as a config file body.
func EncodeConfig(entries []EntryOptions) ([]byte, error) {
	data, err := json.MarshalIndent(struct {
		Plugins []EntryOptions `json:"plugins"`
	}{Plugins: entries}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("loader: cannot encode config: %w", err)
	}
	return append(data, '\n'), nil
}

// LoadFile reads and decodes a config file.
func LoadFile(path string) ([]EntryOptions, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("loader: cannot read config %s: %w", path, err)
	}
	entries, err := DecodeConfig(data)
	if err != nil {
		return nil, fmt.Errorf("loader: %s: %w", path, err)
	}
	return entries, nil
}

// SaveFile writes entry options to a config file.
func SaveFile(path string, entries []EntryOptions) error {
	data, err := EncodeConfig(entries)
	if err != nil {
		return fmt.Errorf("loader: %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("loader: cannot write config %s: %w", path, err)
	}
	return nil
}

// FindConfig returns the config file of dir, if one exists.
func FindConfig(dir string) (string, bool) {
	path := filepath.Join(dir, DefaultConfigName)
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return path, true
	}
	return "", false
}

// Open discovers dir's config file, starts a loader from it and installs a
// write hook that persists config changes back to the file. It is the
// entry point for config-driven applications:
//
//	l, err := loader.Open(ctx, resolver, ".")
func Open(ctx *cordis.Context, resolver *Resolver, dir string) (*Loader, error) {
	path, ok := FindConfig(dir)
	if !ok {
		return nil, fmt.Errorf("loader: no %s in %s", DefaultConfigName, dir)
	}
	entries, err := LoadFile(path)
	if err != nil {
		return nil, err
	}
	l := New(ctx, resolver)
	l.Tree().SetWriteHook(func(data []EntryOptions) {
		if err := SaveFile(path, data); err != nil {
			slog.Error("loader: cannot persist config", "err", err)
		}
	})
	if err := l.Start(entries); err != nil {
		return nil, err
	}
	return l, nil
}
