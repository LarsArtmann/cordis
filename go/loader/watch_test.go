package loader

import (
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	cordis "github.com/LarsArtmann/cordis/go"
)

func openWatched(t *testing.T, dir string, resolver *Resolver, entries []EntryOptions) *Loader {
	t.Helper()
	writeConfig(t, dir, entries)
	l, err := Open(cordis.New(), resolver, dir)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestReloadAppliesDiff(t *testing.T) {
	dir := t.TempDir()
	rec := &recorder{}
	l := openWatched(t, dir, registerEcho(t, rec), []EntryOptions{
		{ID: "a", Name: "echo", Config: echoConf{Msg: "a1"}},
		{ID: "b", Name: "echo", Config: echoConf{Msg: "b"}},
	})
	defer l.Close()

	writeConfig(t, dir, []EntryOptions{
		{ID: "a", Name: "echo", Config: echoConf{Msg: "a2"}},
		{ID: "c", Name: "echo", Config: echoConf{Msg: "c"}},
	})
	if err := l.Reload(); err != nil {
		t.Fatal(err)
	}

	events := rec.snapshot()
	for _, want := range []string{"stop:a1", "start:a2", "stop:b", "start:c"} {
		if !slices.Contains(events, want) {
			t.Fatalf("events = %v, missing %s", events, want)
		}
	}
	if _, ok := l.Tree().Lookup("b"); ok {
		t.Fatal("b survived the reload")
	}
}

func TestReloadBadFileKeepsOldConfig(t *testing.T) {
	dir := t.TempDir()
	rec := &recorder{}
	l := openWatched(t, dir, registerEcho(t, rec), []EntryOptions{
		{ID: "a", Name: "echo", Config: echoConf{Msg: "keep"}},
	})
	defer l.Close()

	if err := os.WriteFile(filepath.Join(dir, DefaultConfigName), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := l.Reload(); err == nil {
		t.Fatal("Reload accepted a broken file")
	}
	events := rec.snapshot()
	if slices.Contains(events, "stop:keep") {
		t.Fatalf("events = %v, old config was torn down", events)
	}
	if e, ok := l.Tree().Lookup("a"); !ok || fiberState(t, e) != cordis.StateActive {
		t.Fatal("old entry not active after failed reload")
	}
}

func TestReloadRollsBackOnlyBrokenSubtree(t *testing.T) {
	dir := t.TempDir()
	rec := &recorder{}
	l := openWatched(t, dir, registerEcho(t, rec), []EntryOptions{
		{ID: "solo", Name: "echo", Config: echoConf{Msg: "solo"}},
		{ID: "g", Name: "group", Config: []EntryOptions{
			{ID: "g1", Name: "echo", Config: echoConf{Msg: "g1"}},
		}},
	})
	defer l.Close()

	// The new config breaks one child inside the group; everything else
	// stays live and the healthy group child is untouched.
	writeConfig(t, dir, []EntryOptions{
		{ID: "solo", Name: "echo", Config: echoConf{Msg: "solo"}},
		{ID: "g", Name: "group", Config: []EntryOptions{
			{ID: "g1", Name: "echo", Config: echoConf{Msg: "g1"}},
			{ID: "g2", Name: "echo", Config: map[string]any{"msg": 42}},
		}},
	})
	// Veto semantics: the group plugin diffing in place never rejects the
	// reload wholesale; the broken child is recorded and the rest applies.
	if err := l.Reload(); err != nil {
		t.Fatal(err)
	}

	events := rec.snapshot()
	// A restart of g1 would record stop:g1; the lone start:g1 is its
	// initial start.
	if slices.Contains(events, "stop:g1") {
		t.Fatalf("events = %v, healthy group child restarted", events)
	}
	if slices.Contains(events, "stop:solo") {
		t.Fatalf("events = %v, sibling disposed by broken subtree", events)
	}
	if errs := l.Tree().Errors(); len(errs) != 1 {
		t.Fatalf("errors = %v, want the single broken child", errs)
	}
	if e, ok := l.Tree().Lookup("g2"); !ok || e.Err() == nil {
		t.Fatal("broken child not recorded with an error")
	}
}

func TestPollWatcherDetectsChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, DefaultConfigName)
	if err := os.WriteFile(path, []byte("[]"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := NewPollWatcher(path, 10*time.Millisecond)

	var changes sync.WaitGroup
	changes.Add(1)
	count := make(chan int, 8)
	go func() {
		n := 0
		w.Watch(func() {
			n++
			count <- n
			changes.Done()
		})
	}()

	time.Sleep(30 * time.Millisecond)
	if err := os.WriteFile(path, []byte("[{}]"), 0o644); err != nil {
		t.Fatal(err)
	}
	changes.Wait()

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if n := <-count; n != 1 {
		t.Fatalf("first change number = %d, want 1", n)
	}
}

func TestServeReloadsOnChange(t *testing.T) {
	dir := t.TempDir()
	rec := &recorder{}
	l := openWatched(t, dir, registerEcho(t, rec), []EntryOptions{
		{ID: "a", Name: "echo", Config: echoConf{Msg: "v1"}},
	})
	l.Serve(NewPollWatcher(filepath.Join(dir, DefaultConfigName), 10*time.Millisecond))
	defer l.Close()

	writeConfig(t, dir, []EntryOptions{
		{ID: "a", Name: "echo", Config: echoConf{Msg: "v2"}},
	})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if slices.Contains(rec.snapshot(), "start:v2") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("reload never applied: %v", rec.snapshot())
}

func TestConcurrentReloadAndUpdates(t *testing.T) {
	dir := t.TempDir()
	rec := &recorder{}
	l := openWatched(t, dir, registerEcho(t, rec), []EntryOptions{
		{ID: "a", Name: "echo", Config: echoConf{Msg: "base"}},
	})

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for range 4 {
				writeConfig(t, dir, []EntryOptions{
					{ID: "a", Name: "echo", Config: echoConf{Msg: "r"}},
					{ID: "c", Name: "echo", Config: echoConf{Msg: "c"}},
				})
				_ = l.Reload()
				if n%2 == 0 {
					_ = l.Tree().SetConfig("a", echoConf{Msg: "direct"})
				}
			}
		}(i)
	}
	wg.Wait()
	l.Tree().Await()
	l.Close()

	if errs := l.Tree().Errors(); len(errs) > 0 {
		t.Fatalf("unexpected entry errors: %v", errs)
	}
	if events := rec.snapshot(); len(events) == 0 {
		t.Fatal("no lifecycle events recorded")
	}
}

func TestCloseStopsWatcher(t *testing.T) {
	dir := t.TempDir()
	rec := &recorder{}
	l := openWatched(t, dir, registerEcho(t, rec), []EntryOptions{
		{ID: "a", Name: "echo", Config: echoConf{Msg: "x"}},
	})
	l.Serve(NewPollWatcher(filepath.Join(dir, DefaultConfigName), 10*time.Millisecond))

	// Before Close the entry is live and nothing has stopped it.
	if slices.Contains(rec.snapshot(), "stop:x") {
		t.Fatal("unexpected stop before Close")
	}
	l.Close()

	// After Close the entry is disposed and the watcher loop has returned.
	if e, ok := l.Tree().Lookup("a"); ok && e.Fiber() != nil {
		t.Fatal("entry still live after Close")
	}
}
