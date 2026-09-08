package loader

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	cordis "github.com/LarsArtmann/cordis/go"
)

func TestRootGroupCreateEntriesDataAndRemove(t *testing.T) {
	rec := &recorder{}
	ctx := cordis.New()
	tree := NewTree(ctx, registerEcho(t, rec))
	defer tree.Close()

	root := tree.Root()
	if root.Tree() != tree {
		t.Fatal("Root().Tree() returned a different tree")
	}

	id, err := root.Create(EntryOptions{Name: "echo", Config: echoConf{Msg: "anon"}})
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("anonymous entry got an empty id")
	}
	tree.Await()

	entries := root.Entries()
	if len(entries) != 1 || entries[0].ID() != id {
		t.Fatalf("Entries() = %v, want the anonymous entry", entries)
	}
	data := root.Data()
	if len(data) != 1 || data[0].ID != id {
		t.Fatalf("Data() = %v, want the anonymous entry options", data)
	}

	root.Remove(id, false)
	if got := root.Entries(); len(got) != 0 {
		t.Fatalf("Entries() after Remove = %v, want empty", got)
	}
	if got := root.Data(); len(got) != 0 {
		t.Fatalf("Data() after Remove = %v, want empty", got)
	}
}

// TestGroupUpdateReconcilesScopeChanges exercises the reconcile paths that
// need a fresh fiber (name, inject and isolate changes) and the merge path
// that keeps intercept-only changes in place.
func TestGroupUpdateReconcilesScopeChanges(t *testing.T) {
	rec := &recorder{}
	resolver := NewResolver()
	RegisterType(resolver, "provider", func(ctx *cordis.Context, _ struct{}) error {
		_, err := ctx.Provide("svc", "v")
		return err
	})
	RegisterType(resolver, "echo", func(ctx *cordis.Context, conf echoConf) error {
		rec.add("start:" + conf.Msg)
		_, err := ctx.Cleanup("echo", func() { rec.add("stop:" + conf.Msg) })
		return err
	})
	tree := NewTree(cordis.New(), resolver)
	defer tree.Close()

	if _, err := tree.Create(EntryOptions{ID: "p", Name: "provider"}, "", -1); err != nil {
		t.Fatal(err)
	}
	g := EntryOptions{ID: "g", Name: "group", Config: []EntryOptions{
		{ID: "m", Name: "echo", Config: echoConf{Msg: "v1"}, Inject: map[string]any{"svc": nil}},
	}}
	if _, err := tree.Create(g, "", -1); err != nil {
		t.Fatal(err)
	}
	tree.Await()

	// Changing the inject set requires a fresh fiber.
	g.Config = []EntryOptions{
		{ID: "m", Name: "echo", Config: echoConf{Msg: "v2"}, Inject: map[string]any{"svc": nil}, Intercept: map[string]any{"svc": "over"}},
	}
	if err := tree.Update("g", g); err != nil {
		t.Fatal(err)
	}
	tree.Await()
	events := rec.snapshot()
	if !slices.Contains(events, "stop:v1") || !slices.Contains(events, "start:v2") {
		t.Fatalf("events = %v, want scope-change rebuild", events)
	}
}

func TestMoveBetweenGroupsAndNoOp(t *testing.T) {
	rec := &recorder{}
	resolver := NewResolver()
	RegisterType(resolver, "echo", func(ctx *cordis.Context, conf echoConf) error {
		rec.add("start:" + conf.Msg)
		return nil
	})
	tree := NewTree(cordis.New(), resolver)
	defer tree.Close()

	if _, err := tree.Create(EntryOptions{ID: "a", Name: "echo", Config: echoConf{Msg: "a"}}, "", -1); err != nil {
		t.Fatal(err)
	}
	if _, err := tree.Create(EntryOptions{ID: "g", Name: "group", Config: []EntryOptions{
		{ID: "g1", Name: "echo", Config: echoConf{Msg: "g1"}},
	}}, "", -1); err != nil {
		t.Fatal(err)
	}
	tree.Await()

	// Root to root is a no-op.
	if err := tree.Move("a", "", 0); err != nil {
		t.Fatalf("root-to-root move: %v", err)
	}

	// Moving into the nested group relinks the config.
	if err := tree.Move("a", "g", 0); err != nil {
		t.Fatalf("move into group: %v", err)
	}
	data := tree.Root().Data()
	for _, opts := range data {
		if opts.ID == "a" {
			t.Fatalf("root config still holds the moved entry: %v", data)
		}
	}
	gEntry, _ := tree.Lookup("g")
	found := false
	for _, opts := range gEntry.Subgroup().Data() {
		if opts.ID == "a" {
			found = true
		}
	}
	if !found {
		t.Fatal("group config does not hold the moved entry")
	}
	// Position 0 in the target: the moved entry comes first.
	if got := gEntry.Subgroup().Data()[0].ID; got != "a" {
		t.Fatalf("moved entry at position %q, want first", got)
	}
	tree.Await()
	if fiberState(t, mustEntry(t, tree, "a")) != cordis.StateActive {
		t.Fatal("moved entry no longer active")
	}
}

func mustEntry(t *testing.T, tree *Tree, id string) *Entry {
	t.Helper()
	e, ok := tree.Lookup(id)
	if !ok {
		t.Fatalf("entry %s missing", id)
	}
	return e
}

func TestEntryErrorFormatting(t *testing.T) {
	cause := errors.New("boom")
	err := &EntryError{ID: "a", Name: "echo", Err: cause}
	if got, want := err.Error(), "loader: entry a (echo): boom"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	if !errors.Is(err, cause) {
		t.Fatal("EntryError does not unwrap to its cause")
	}
	entry, ok := errors.AsType[*EntryError](err)
	if !ok || entry.ID != "a" {
		t.Fatal("errors.AsType did not recover the EntryError")
	}
}

func TestResolverResolveAndDecode(t *testing.T) {
	resolver := NewResolver()
	if _, err := resolver.Resolve("nope"); err == nil {
		t.Fatal("Resolve of an unknown name must fail")
	}
	if _, err := resolver.Decode("nope", nil); err == nil {
		t.Fatal("Decode of an unknown name must fail")
	}

	// A registration without a decoder passes the raw value through.
	if err := resolver.Register("raw", Registration{
		New: func() cordis.PluginHandle { return cordis.NewPlugin("raw", func(*cordis.Context, any) error { return nil }) },
	}); err != nil {
		t.Fatal(err)
	}
	handle, err := resolver.Resolve("raw")
	if err != nil {
		t.Fatal(err)
	}
	if handle == nil {
		t.Fatal("Resolve returned a nil handle")
	}
	raw, err := resolver.Decode("raw", "untouched")
	if err != nil {
		t.Fatal(err)
	}
	if raw != "untouched" {
		t.Fatalf("Decode without a decoder = %v, want pass-through", raw)
	}

	typed, err := resolver.Decode("raw", map[string]any{"x": 1})
	if err != nil {
		t.Fatal(err)
	}
	_ = typed
}

func TestTreeLocateAndContext(t *testing.T) {
	tree := NewTree(cordis.New(), registerEcho(t, &recorder{}))
	defer tree.Close()
	if tree.Context() == nil {
		t.Fatal("Tree.Context() is nil")
	}

	if _, err := tree.Create(EntryOptions{ID: "a", Name: "echo", Config: echoConf{Msg: "a"}}, "", -1); err != nil {
		t.Fatal(err)
	}
	tree.Await()

	e := mustEntry(t, tree, "a")
	id, ok := tree.Locate(e.Fiber())
	if !ok || id != "a" {
		t.Fatalf("Locate = %q %v, want a true", id, ok)
	}

	foreign, err := cordis.Start(tree.Context(), cordis.NewPlugin("foreign", func(*cordis.Context, int) error { return nil }), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer foreign.Dispose()
	if _, ok := tree.Locate(foreign); ok {
		t.Fatal("Locate tracked a foreign fiber")
	}
}

func TestConfigErrorsAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "roundtrip.json")
	entries := []EntryOptions{{ID: "a", Name: "echo", Config: echoConf{Msg: "x"}}}
	if err := SaveFile(path, entries); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].ID != "a" {
		t.Fatalf("roundtrip = %v", loaded)
	}

	if _, err := LoadFile(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("LoadFile of a missing file must fail")
	}
	if err := SaveFile(filepath.Join(dir, "no", "such", "dir", "c.json"), entries); err == nil {
		t.Fatal("SaveFile into a missing directory must fail")
	}
	if _, err := DecodeConfig([]byte("{not json")); err == nil {
		t.Fatal("DecodeConfig of garbage must fail")
	}
	if _, err := EncodeConfig([]EntryOptions{{ID: "c", Name: "chan", Config: make(chan int)}}); err == nil {
		t.Fatal("EncodeConfig of an unmarshalable config must fail")
	}
	if _, ok := FindConfig(dir); ok {
		t.Fatal("FindConfig found a config in an empty directory")
	}
	if _, err := Open(cordis.New(), NewResolver(), dir); err == nil {
		t.Fatal("Open without a config file must fail")
	}
}

func TestReloadWithoutConfigPath(t *testing.T) {
	l := New(cordis.New(), NewResolver())
	defer l.Close()
	if err := l.Reload(); err == nil {
		t.Fatal("Reload without a config path must fail")
	}
}

func TestServeTwiceKeepsFirstWatcher(t *testing.T) {
	dir := t.TempDir()
	l := openWatched(t, dir, registerEcho(t, &recorder{}), []EntryOptions{
		{ID: "a", Name: "echo", Config: echoConf{Msg: "v1"}},
	})
	defer l.Close()

	w := NewPollWatcher(filepath.Join(dir, DefaultConfigName), 10*time.Millisecond)
	l.Serve(w)
	l.Serve(NewPollWatcher(filepath.Join(dir, DefaultConfigName), 10*time.Millisecond))

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	l.Close()
	if err := os.Remove(filepath.Join(dir, DefaultConfigName)); err != nil {
		t.Fatal(err)
	}
}

// TestTreeRefreshRelinksInPlace pins Refresh's hot-swap contract: the
// entry keeps its id and config while the fiber is rebuilt from the
// resolver's current registration.
func TestTreeRefreshRelinksInPlace(t *testing.T) {
	rec := &recorder{}
	resolver := NewResolver()
	RegisterType(resolver, "echo", func(ctx *cordis.Context, conf echoConf) error {
		rec.add("start:" + conf.Msg)
		_, err := ctx.Cleanup("echo", func() { rec.add("stop:" + conf.Msg) })
		return err
	})
	tree := NewTree(cordis.New(), resolver)
	defer tree.Close()
	if _, err := tree.Create(EntryOptions{ID: "a", Name: "echo", Config: echoConf{Msg: "cfg"}}, "", -1); err != nil {
		t.Fatal(err)
	}
	tree.Await()
	before := mustEntry(t, tree, "a").Fiber()

	if err := tree.Refresh("a"); err != nil {
		t.Fatal(err)
	}
	tree.Await()

	after := mustEntry(t, tree, "a").Fiber()
	if before == nil || after == nil || before == after {
		t.Fatal("Refresh did not rebuild the fiber")
	}
	events := rec.snapshot()
	if !slices.Contains(events, "stop:cfg") || !slices.Contains(events, "start:cfg") {
		t.Fatalf("events = %v, want stop+start with the config preserved", events)
	}

	if _, ok := tree.Lookup("a"); !ok {
		t.Fatal("entry id lost across Refresh")
	}
	if errs := tree.Errors(); len(errs) != 0 {
		t.Fatalf("errors = %v, want none", errs)
	}
}

func TestLoaderLocate(t *testing.T) {
	dir := t.TempDir()
	l := openWatched(t, dir, registerEcho(t, &recorder{}), []EntryOptions{
		{ID: "a", Name: "echo", Config: echoConf{Msg: "a"}},
	})
	defer l.Close()
	l.Tree().Await()

	e := mustEntry(t, l.Tree(), "a")
	if id, ok := l.Locate(e.Fiber()); !ok || id != "a" {
		t.Fatalf("Locate = %q %v, want a true", id, ok)
	}
}
