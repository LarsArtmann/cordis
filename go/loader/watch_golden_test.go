package loader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cordis "github.com/LarsArtmann/cordis/go"
)

// TestWatchReloadGoldenTrace pins the complete watch/reload transcript of a
// config-driven loader as one byte-stable sequence: initial start, in-place
// config update, reload diff (restart, group diff, fresh entry), plugin
// self-disposal with its persistence side effect, and close. It is the
// Go-only counterpart of the golden/ scenarios — the loader has no Rust or
// Zig port, so a three-runner scenario cannot exist; this trace fixes the
// same kind of canonical transcript for the subsystem.
//
// Regenerate with GOLDEN_UPDATE=1 go test -run TestWatchReloadGoldenTrace ./loader/
func TestWatchReloadGoldenTrace(t *testing.T) {
	trace := make([]string, 0, 32)
	record := func(format string, args ...any) {
		trace = append(trace, fmt.Sprintf(format, args...))
	}

	resolver := NewResolver()
	RegisterType(resolver, "echo", func(ctx *cordis.Context, conf echoConf) error {
		record("start %s", conf.Msg)
		if conf.Msg == "bye" {
			ctx.Fiber().Dispose()
			return nil
		}
		_, err := ctx.Cleanup("echo", func() { record("stop %s", conf.Msg) })
		return err
	})

	dir := t.TempDir()
	writeConfig(t, dir, []EntryOptions{
		{ID: "a", Name: "echo", Config: echoConf{Msg: "one"}},
		{ID: "b", Name: "echo", Config: echoConf{Msg: "two"}},
		{ID: "g", Name: "group", Config: []EntryOptions{
			{ID: "g1", Name: "echo", Config: echoConf{Msg: "alpha"}},
		}},
	})

	ctx := cordis.New()
	if _, err := ctx.On(EventPartialDispose, func(args ...any) any {
		id, _ := args[0].(string)
		record("partial %s", id)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ctx.On(EventConfigUpdate, func(args ...any) any {
		entries, _ := args[0].([]EntryOptions)
		ids := make([]string, 0, len(entries))
		for _, e := range entries {
			ids = append(ids, e.ID)
		}
		record("config-update %s", strings.Join(ids, ","))
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	l, err := Open(ctx, resolver, dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := l.Tree().SetConfig("a", echoConf{Msg: "one-b"}); err != nil {
		t.Fatal(err)
	}
	l.Tree().Await()

	writeConfig(t, dir, []EntryOptions{
		{ID: "a", Name: "echo", Config: echoConf{Msg: "one-c"}},
		{ID: "b", Name: "echo", Config: echoConf{Msg: "two"}},
		{ID: "g", Name: "group", Config: []EntryOptions{
			{ID: "g1", Name: "echo", Config: echoConf{Msg: "beta"}},
			{ID: "g2", Name: "echo", Config: echoConf{Msg: "gamma"}},
		}},
	})
	if err := l.Reload(); err != nil {
		t.Fatal(err)
	}

	if err := l.Tree().SetConfig("b", echoConf{Msg: "bye"}); err != nil {
		t.Fatal(err)
	}
	l.Tree().Await()

	l.Close()
	record("closed")

	goldenPath := filepath.Join("testdata", "watch-golden.txt")
	if os.Getenv("GOLDEN_UPDATE") != "" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, []byte(strings.Join(trace, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	expected := strings.Split(strings.TrimRight(string(want), "\n"), "\n")
	if len(trace) != len(expected) {
		t.Fatalf("trace has %d lines, golden has %d:\n%s", len(trace), len(expected), strings.Join(trace, "\n"))
	}
	for i, line := range expected {
		if trace[i] != line {
			t.Fatalf("trace divergence at line %d:\n  want: %s\n  got:  %s\nfull trace:\n%s", i+1, line, trace[i], strings.Join(trace, "\n"))
		}
	}
}
