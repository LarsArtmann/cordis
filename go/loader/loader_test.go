package loader

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	cordis "github.com/LarsArtmann/cordis/go"
)

type recorder struct {
	mu     sync.Mutex
	events []string
}

func (r *recorder) add(event string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *recorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.events)
}

type echoConf struct {
	Msg string `json:"msg"`
}

func registerEcho(t *testing.T, rec *recorder) *Resolver {
	t.Helper()
	resolver := NewResolver()
	RegisterType(resolver, "echo", func(ctx *cordis.Context, conf echoConf) error {
		rec.add("start:" + conf.Msg)
		_, err := ctx.Cleanup("echo", func() { rec.add("stop:" + conf.Msg) })
		return err
	})
	return resolver
}

func writeConfig(t *testing.T, dir string, entries []EntryOptions) {
	t.Helper()
	data, err := EncodeConfig(entries)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, DefaultConfigName), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func fiberState(t *testing.T, e *Entry) cordis.FiberState {
	t.Helper()
	f := e.Fiber()
	if f == nil {
		t.Fatalf("entry %s has no fiber", e.ID())
	}
	return f.State()
}

func TestOpenStartsConfigFile(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, []EntryOptions{
		{ID: "a", Name: "echo", Config: echoConf{Msg: "one"}},
		{ID: "b", Name: "echo", Config: echoConf{Msg: "two"}},
	})
	rec := &recorder{}
	ctx := cordis.New()
	l, err := Open(ctx, registerEcho(t, rec), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	for _, id := range []string{"a", "b"} {
		e, ok := l.Tree().Lookup(id)
		if !ok {
			t.Fatalf("entry %s missing", id)
		}
		if s := fiberState(t, e); s != cordis.StateActive {
			t.Fatalf("entry %s state = %s, want active", id, s)
		}
	}
	events := rec.snapshot()
	if !slices.Contains(events, "start:one") || !slices.Contains(events, "start:two") {
		t.Fatalf("events = %v, want both starts", events)
	}
}

func TestValidationErrorsPerEntry(t *testing.T) {
	rec := &recorder{}
	resolver := registerEcho(t, rec)
	ctx := cordis.New()
	tree := NewTree(ctx, resolver)

	entries := []EntryOptions{
		{ID: "good", Name: "echo", Config: echoConf{Msg: "fine"}},
		{ID: "unknown", Name: "nope"},
		{ID: "badconf", Name: "echo", Config: map[string]any{"msg": 42}},
	}
	if err := tree.Root().Update(entries); err == nil {
		t.Fatal("Update succeeded, want per-entry errors")
	}
	tree.Await()

	// Validate is a dry run over the stored entries; the two broken ones
	// are reported per entry without touching the good one.
	for _, err := range tree.Validate() {
		var ee *EntryError
		if !errors.As(err, &ee) {
			t.Fatalf("validate returned %T, want *EntryError", err)
		}
		if ee.ID != "unknown" && ee.ID != "badconf" {
			t.Fatalf("unexpected validation error for %s", ee.ID)
		}
	}
	if n := len(tree.Validate()); n != 2 {
		t.Fatalf("Validate returned %d errors, want 2", n)
	}

	errs := tree.Errors()
	if len(errs) != 2 {
		t.Fatalf("Errors() = %v, want 2 failures", errs)
	}
	if _, ok := errs["unknown"]; !ok {
		t.Fatalf("Errors() missing unknown: %v", errs)
	}
	if e, ok := tree.Lookup("good"); !ok || fiberState(t, e) != cordis.StateActive {
		t.Fatal("good entry did not start despite sibling failures")
	}
}

func TestInjectWiring(t *testing.T) {
	rec := &recorder{}
	resolver := NewResolver()
	RegisterType(resolver, "needy", func(ctx *cordis.Context, _ struct{}) error {
		dep, err := cordis.GetNamed[int](ctx, "dep")
		if err != nil {
			rec.add("err:" + err.Error())
			return err
		}
		if cfg, ok := ctx.Intercepted("dep"); ok {
			rec.add("intercepted")
			_ = cfg
		}
		rec.add("needy")
		_ = dep
		return nil
	})
	ctx := cordis.New()
	tree := NewTree(ctx, resolver)

	if err := tree.Root().Update([]EntryOptions{
		{ID: "n", Name: "needy", Inject: map[string]any{"dep": map[string]any{"k": "v"}}},
	}); err != nil {
		t.Fatal(err)
	}

	e, _ := tree.Lookup("n")
	if f := e.Fiber(); f == nil || f.State() != cordis.StatePending {
		t.Fatalf("needy should be parked pending dependency, got %v", f)
	}

	if _, err := ctx.Provide("dep", 42); err != nil {
		t.Fatal(err)
	}
	tree.Await()

	if s := fiberState(t, e); s != cordis.StateActive {
		t.Fatalf("needy state = %s after provide, want active", s)
	}
	events := rec.snapshot()
	if !slices.Contains(events, "needy") || !slices.Contains(events, "intercepted") {
		t.Fatalf("events = %v, want needy + intercepted", events)
	}
}

func TestNestedGroupDiff(t *testing.T) {
	rec := &recorder{}
	ctx := cordis.New()
	tree := NewTree(ctx, registerEcho(t, rec))

	err := tree.Root().Update([]EntryOptions{
		{ID: "g", Name: "group", Config: []EntryOptions{
			{ID: "c1", Name: "echo", Config: echoConf{Msg: "c1"}},
			{ID: "c2", Name: "echo", Config: echoConf{Msg: "c2"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	tree.Await()

	g, _ := tree.Lookup("g")
	if g.Subgroup() == nil {
		t.Fatal("group entry has no subgroup")
	}
	c1, _ := tree.Lookup("c1")
	c2, _ := tree.Lookup("c2")
	if s := fiberState(t, c1); s != cordis.StateActive {
		t.Fatalf("c1 state = %s", s)
	}
	f2 := c2.Fiber()

	if err := tree.SetConfig("c1", echoConf{Msg: "c1b"}); err != nil {
		t.Fatal(err)
	}
	events := rec.snapshot()
	if !slices.Contains(events, "stop:c1") || !slices.Contains(events, "start:c1b") {
		t.Fatalf("events = %v, want c1 restart", events)
	}
	if c2.Fiber() != f2 {
		t.Fatal("c2 fiber changed on sibling update")
	}

	if err := tree.SetConfig("g", []EntryOptions{
		{ID: "c1", Name: "echo", Config: echoConf{Msg: "c1c"}},
		{ID: "c2", Name: "echo", Config: echoConf{Msg: "c2"}},
	}); err != nil {
		t.Fatal(err)
	}
	events = rec.snapshot()
	if !slices.Contains(events, "start:c1c") {
		t.Fatalf("events = %v, want c1c via group diff", events)
	}
	if c2.Fiber() != f2 {
		t.Fatal("c2 fiber changed on group config update")
	}

	if err := tree.Remove("g"); err != nil {
		t.Fatal(err)
	}
	events = rec.snapshot()
	for _, want := range []string{"stop:c1c", "stop:c2"} {
		if !slices.Contains(events, want) {
			t.Fatalf("events = %v, missing %s", events, want)
		}
	}
	if _, ok := tree.Lookup("c1"); ok {
		t.Fatal("c1 still in store after group removal")
	}
	if _, ok := tree.Lookup("c2"); ok {
		t.Fatal("c2 still in store after group removal")
	}
}

func TestIsolateAndInterceptOptions(t *testing.T) {
	rec := &recorder{}
	resolver := NewResolver()
	RegisterType(resolver, "provider", func(ctx *cordis.Context, _ struct{}) error {
		_, err := ctx.Provide("svc", "from-provider")
		return err
	})
	RegisterType(resolver, "reader", func(ctx *cordis.Context, _ struct{}) error {
		if cfg, ok := ctx.Intercepted("svc"); ok {
			rec.add("intercept:" + cfg.(map[string]any)["k"].(string))
		} else {
			rec.add("intercept:none")
		}
		return nil
	})
	ctx := cordis.New()
	tree := NewTree(ctx, resolver)

	err := tree.Root().Update([]EntryOptions{
		{ID: "p", Name: "provider", Isolate: map[string]any{"svc": true}},
		{ID: "r", Name: "reader", Intercept: map[string]any{"svc": map[string]any{"k": "v"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	tree.Await()

	if _, err := cordis.GetNamed[string](ctx, "svc"); err == nil {
		t.Fatal("isolated service leaked to the root scope")
	}
	events := rec.snapshot()
	if !slices.Contains(events, "intercept:v") {
		t.Fatalf("events = %v, want intercept:v", events)
	}
}

func TestSelfDisposeMarksDisabled(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, []EntryOptions{
		{ID: "s", Name: "boom"},
	})
	rec := &recorder{}
	resolver := NewResolver()
	RegisterType(resolver, "boom", func(ctx *cordis.Context, _ struct{}) error {
		rec.add("boom")
		ctx.Fiber().Dispose()
		return nil
	})
	ctx := cordis.New()
	l, err := Open(ctx, resolver, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	e, _ := l.Tree().Lookup("s")
	if e.Fiber() != nil {
		t.Fatal("self-disposed entry still has a fiber")
	}
	if !e.Disabled() {
		t.Fatal("self-disposed entry not marked disabled")
	}
	data, err := os.ReadFile(filepath.Join(dir, DefaultConfigName))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := DecodeConfig(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !entries[0].Disabled {
		t.Fatalf("persisted config = %v, want disabled entry", entries)
	}
}

func TestInPlaceConfigUpdate(t *testing.T) {
	rec := &recorder{}
	ctx := cordis.New()
	tree := NewTree(ctx, registerEcho(t, rec))
	if err := tree.Root().Update([]EntryOptions{
		{ID: "a", Name: "echo", Config: echoConf{Msg: "one"}},
		{ID: "b", Name: "echo", Config: echoConf{Msg: "two"}},
	}); err != nil {
		t.Fatal(err)
	}
	tree.Await()

	ea, _ := tree.Lookup("a")
	eb, _ := tree.Lookup("b")
	fa, fb := ea.Fiber(), eb.Fiber()

	if err := tree.SetConfig("a", echoConf{Msg: "one-b"}); err != nil {
		t.Fatal(err)
	}
	if ea.Fiber() != fa {
		t.Fatal("fiber identity changed on config update")
	}
	if eb.Fiber() != fb {
		t.Fatal("sibling fiber changed on unrelated update")
	}
	events := rec.snapshot()
	if !slices.Contains(events, "stop:one") || !slices.Contains(events, "start:one-b") {
		t.Fatalf("events = %v, want in-place restart", events)
	}
	if slices.Contains(events, "stop:two") {
		t.Fatalf("events = %v, sibling must not restart", events)
	}
}

func TestRootDiffStart(t *testing.T) {
	rec := &recorder{}
	ctx := cordis.New()
	l := New(ctx, registerEcho(t, rec))

	if err := l.Start([]EntryOptions{
		{ID: "a", Name: "echo", Config: echoConf{Msg: "a1"}},
		{ID: "b", Name: "echo", Config: echoConf{Msg: "b"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := l.Start([]EntryOptions{
		{ID: "a", Name: "echo", Config: echoConf{Msg: "a2"}},
		{ID: "c", Name: "echo", Config: echoConf{Msg: "c"}},
	}); err != nil {
		t.Fatal(err)
	}

	events := rec.snapshot()
	for _, want := range []string{"start:a1", "stop:a1", "start:a2", "stop:b", "start:c"} {
		if !slices.Contains(events, want) {
			t.Fatalf("events = %v, missing %s", events, want)
		}
	}
	exported := l.Tree().Export()
	if len(exported) != 2 || exported[0].ID != "a" || exported[1].ID != "c" {
		t.Fatalf("export = %v, want [a c]", exported)
	}
}

func TestUpdateReplaceAndDisabled(t *testing.T) {
	rec := &recorder{}
	ctx := cordis.New()
	tree := NewTree(ctx, registerEcho(t, rec))
	if err := tree.Root().Update([]EntryOptions{
		{ID: "a", Name: "echo", Config: echoConf{Msg: "a"}},
	}); err != nil {
		t.Fatal(err)
	}
	tree.Await()

	if err := tree.Update("a", EntryOptions{Disabled: true}); err != nil {
		t.Fatal(err)
	}
	e, _ := tree.Lookup("a")
	if e.Fiber() != nil || !e.Disabled() {
		t.Fatal("disabling did not dispose the entry")
	}
	if err := tree.Update("a", EntryOptions{Disabled: false}); err != nil {
		t.Fatal(err)
	}
	// Update merges non-zero fields; Disabled=false is the zero value, so
	// the entry stays disabled. Replace clears it.
	if e.Disabled() != true {
		t.Fatal("merge semantics changed unexpectedly")
	}
	if err := tree.Replace("a", EntryOptions{Name: "echo", Config: echoConf{Msg: "a"}}); err != nil {
		t.Fatal(err)
	}
	tree.Await()
	if s := fiberState(t, e); s != cordis.StateActive {
		t.Fatalf("state after replace = %s, want active", s)
	}
}

func TestDecodeIntoPassesTypedThrough(t *testing.T) {
	in := echoConf{Msg: "hi"}
	out, err := DecodeInto[echoConf](in)
	if err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Fatalf("typed passthrough changed value: %+v", out)
	}
	raw, err := DecodeInto[echoConf](map[string]any{"msg": "raw"})
	if err != nil {
		t.Fatal(err)
	}
	if raw.Msg != "raw" {
		t.Fatalf("raw decode = %+v", raw)
	}
}

func TestConfigFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if _, ok := FindConfig(dir); ok {
		t.Fatal("FindConfig found a file that does not exist")
	}
	entries := []EntryOptions{
		{ID: "a", Name: "echo", Config: echoConf{Msg: "x"}, Inject: map[string]any{"svc": nil}},
	}
	writeConfig(t, dir, entries)
	path, ok := FindConfig(dir)
	if !ok {
		t.Fatal("FindConfig missed cordis.json")
	}
	got, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "a" || got[0].Name != "echo" {
		t.Fatalf("round trip = %+v", got)
	}

	arrayBody := []byte(`[{"id":"z","name":"echo"}]`)
	arr, err := DecodeConfig(arrayBody)
	if err != nil || len(arr) != 1 || arr[0].ID != "z" {
		t.Fatalf("array decode = %v, %v", arr, err)
	}
}
