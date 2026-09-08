package hmr_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	cordis "github.com/LarsArtmann/cordis/go"
	"github.com/LarsArtmann/cordis/go/hmr"
	"github.com/LarsArtmann/cordis/go/loader"
)

type echoConf struct {
	Msg string `json:"msg"`
}

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

func (r *recorder) last() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.events) == 0 {
		return ""
	}
	return r.events[len(r.events)-1]
}

// echoImpl builds one implementation generation of an echo plugin: it
// records a start event with its generation tag plus the config it
// received, and a stop event on disposal.
func echoImpl(tag string, rec *recorder) func(ctx *cordis.Context, conf echoConf) error {
	return func(ctx *cordis.Context, conf echoConf) error {
		rec.add("start:" + tag + ":" + conf.Msg)
		_, err := ctx.Cleanup("echo", func() { rec.add("stop:" + tag) })
		return err
	}
}

// brokenImpl is an implementation generation whose apply always fails.
func brokenImpl(rec *recorder) func(ctx *cordis.Context, conf echoConf) error {
	return func(ctx *cordis.Context, conf echoConf) error {
		rec.add("start:broken")
		return errors.New("boom")
	}
}

func typeReg[C any](name string, apply func(*cordis.Context, C) error) loader.Registration {
	return loader.TypedRegistration(name, apply)
}

func setup(t *testing.T) (*hmr.Manager, *loader.Tree, *cordis.Context, *recorder) {
	t.Helper()
	ctx := cordis.New()
	resolver := loader.NewResolver()
	tree := loader.NewTree(ctx, resolver)
	rec := &recorder{}
	resolver.MustRegister("echo", typeReg("echo", echoImpl("v1", rec)))
	resolver.MustRegister("app", typeReg("app", echoImpl("app1", rec)))
	resolver.MustRegister("lib", typeReg("lib", echoImpl("lib1", rec)))
	resolver.MustRegister("solo", typeReg("solo", echoImpl("solo1", rec)))
	return hmr.New(resolver, tree), tree, ctx, rec
}

func createEntry(t *testing.T, tree *loader.Tree, opts loader.EntryOptions) {
	t.Helper()
	if _, err := tree.Create(opts, "", -1); err != nil {
		t.Fatal(err)
	}
}

func fiberState(t *testing.T, tree *loader.Tree, id string) cordis.FiberState {
	t.Helper()
	e, ok := tree.Lookup(id)
	if !ok {
		t.Fatalf("entry %s missing", id)
	}
	f := e.Fiber()
	if f == nil {
		t.Fatalf("entry %s has no fiber", id)
	}
	return f.State()
}

func TestSwapReloadsEntry(t *testing.T) {
	mgr, tree, _, rec := setup(t)
	createEntry(t, tree, loader.EntryOptions{ID: "a", Name: "echo", Config: echoConf{Msg: "cfg"}})

	if _, err := hmr.SwapType(mgr, "echo", echoImpl("v2", rec)); err != nil {
		t.Fatal(err)
	}

	events := rec.snapshot()
	for _, want := range []string{"start:v1:cfg", "stop:v1", "start:v2:cfg"} {
		if !slices.Contains(events, want) {
			t.Fatalf("events = %v, missing %s", events, want)
		}
	}
	if fiberState(t, tree, "a") != cordis.StateActive {
		t.Fatal("entry not active after swap")
	}
	if e, _ := tree.Lookup("a"); e.ID() != "a" {
		t.Fatal("entry identity changed across swap")
	}
}

func TestSwapEmitsEvent(t *testing.T) {
	mgr, tree, ctx, rec := setup(t)
	createEntry(t, tree, loader.EntryOptions{ID: "a", Name: "echo", Config: echoConf{Msg: "cfg"}})

	var got hmr.Report
	_, _ = ctx.On(hmr.EventReload, func(args ...any) any {
		got = args[0].(hmr.Report)
		return nil
	})

	if _, err := hmr.SwapType(mgr, "echo", echoImpl("v2", rec)); err != nil {
		t.Fatal(err)
	}
	if got.Module != "echo" || got.Generation != 1 || !slices.Equal(got.Reloaded, []string{"a"}) {
		t.Fatalf("report = %+v", got)
	}
}

func TestSwapOnlyAffectsDependents(t *testing.T) {
	mgr, tree, _, rec := setup(t)
	mgr.Declare("app", "lib")
	createEntry(t, tree, loader.EntryOptions{ID: "app", Name: "app", Config: echoConf{Msg: "x"}})
	createEntry(t, tree, loader.EntryOptions{ID: "lib", Name: "lib", Config: echoConf{Msg: "y"}})
	createEntry(t, tree, loader.EntryOptions{ID: "solo", Name: "solo", Config: echoConf{Msg: "z"}})

	if _, err := hmr.SwapType(mgr, "lib", echoImpl("lib2", rec)); err != nil {
		t.Fatal(err)
	}

	events := rec.snapshot()
	for _, want := range []string{"stop:lib1", "start:lib2:y", "stop:app1", "start:app1:x"} {
		if !slices.Contains(events, want) {
			t.Fatalf("events = %v, missing %s", events, want)
		}
	}
	if slices.Contains(events, "stop:solo1") {
		t.Fatalf("events = %v, unrelated solo reloaded", events)
	}
}

func TestSwapTransitiveDependents(t *testing.T) {
	mgr, tree, _, rec := setup(t)
	mgr.Declare("app", "lib")
	mgr.Declare("lib", "core")
	createEntry(t, tree, loader.EntryOptions{ID: "app", Name: "app", Config: echoConf{Msg: "x"}})
	createEntry(t, tree, loader.EntryOptions{ID: "solo", Name: "solo", Config: echoConf{Msg: "z"}})

	if _, err := mgr.Swap("core", typeReg("core", echoImpl("core2", rec))); err != nil {
		t.Fatal(err)
	}

	events := rec.snapshot()
	for _, want := range []string{"stop:app1", "start:app1:x"} {
		if !slices.Contains(events, want) {
			t.Fatalf("events = %v, missing %s", events, want)
		}
	}
	if slices.Contains(events, "stop:solo1") {
		t.Fatalf("events = %v, unrelated solo reloaded", events)
	}
}

func TestSwapRollsBackOnFailure(t *testing.T) {
	mgr, tree, _, rec := setup(t)
	createEntry(t, tree, loader.EntryOptions{ID: "a", Name: "echo", Config: echoConf{Msg: "cfg"}})

	rollbackErr := error(nil)
	if _, err := mgr.Swap("echo", typeReg("echo", brokenImpl(rec))); err == nil {
		t.Fatal("swap with a broken implementation returned no error")
	} else {
		rollbackErr = err
	}

	// The rollback error must carry the failed fiber's own error detail
	// for diagnosis, not just the entry id.
	if !strings.Contains(rollbackErr.Error(), "failed under the new implementation") {
		t.Fatalf("rollback error = %v, missing entry failure context", rollbackErr)
	}
	if !strings.Contains(rollbackErr.Error(), "boom") {
		t.Fatalf("rollback error = %v, missing Fiber.Err detail", rollbackErr)
	}

	events := rec.snapshot()
	if !slices.Contains(events, "start:broken") {
		t.Fatalf("events = %v, broken implementation never attempted", events)
	}
	if rec.last() != "start:v1:cfg" {
		t.Fatalf("events = %v, rollback did not restart the old implementation", events)
	}
	if mgr.Generation("echo") != 0 {
		t.Fatalf("generation = %d, rolled-back swap must not count", mgr.Generation("echo"))
	}
	if fiberState(t, tree, "a") != cordis.StateActive {
		t.Fatal("old implementation not restored to active state")
	}

	// Recovery: a good implementation applies after the failed one.
	if _, err := hmr.SwapType(mgr, "echo", echoImpl("v2", rec)); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(rec.snapshot(), "start:v2:cfg") {
		t.Fatal("recovery swap did not apply")
	}
}

func TestSwapKeepsUnaffectedModulesLive(t *testing.T) {
	mgr, tree, _, rec := setup(t)
	createEntry(t, tree, loader.EntryOptions{ID: "a", Name: "echo", Config: echoConf{Msg: "a"}})
	createEntry(t, tree, loader.EntryOptions{ID: "s", Name: "solo", Config: echoConf{Msg: "s"}})

	if _, err := hmr.SwapType(mgr, "echo", echoImpl("a2", rec)); err != nil {
		t.Fatal(err)
	}
	events := rec.snapshot()
	if slices.Contains(events, "stop:solo1") {
		t.Fatalf("events = %v, solo module was touched", events)
	}
	if fiberState(t, tree, "s") != cordis.StateActive {
		t.Fatal("unaffected entry no longer active")
	}
}

func TestRapidSuccessiveSwaps(t *testing.T) {
	mgr, tree, _, rec := setup(t)
	createEntry(t, tree, loader.EntryOptions{ID: "a", Name: "echo", Config: echoConf{Msg: "cfg"}})

	for i := range 50 {
		tag := fmt.Sprintf("g%d", i)
		if _, err := hmr.SwapType(mgr, "echo", echoImpl(tag, rec)); err != nil {
			t.Fatal(err)
		}
	}
	if mgr.Generation("echo") != 50 {
		t.Fatalf("generation = %d, want 50", mgr.Generation("echo"))
	}
	tree.Await()
	if fiberState(t, tree, "a") != cordis.StateActive {
		t.Fatal("entry not active after rapid swaps")
	}
	if rec.last() != "start:g49:cfg" {
		t.Fatalf("last event = %s, want the final generation live", rec.last())
	}
}

// TestSwapCreateRemoveStorm races parallel swaps against concurrent entry
// creation and removal. The swap mutex, the tree mutex and the resolver
// mutex must interleave without deadlock, torn state or lost rollbacks:
// every entry that survives the storm settles back to ACTIVE, and a final
// clean swap still applies.
func TestSwapCreateRemoveStorm(t *testing.T) {
	mgr, tree, _, rec := setup(t)

	const (
		swappers       = 4
		swapsPerWorker = 10
		creators       = 4
		entriesEach    = 10
	)

	var wg sync.WaitGroup
	errs := make(chan error, creators*entriesEach)
	for w := range swappers {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := range swapsPerWorker {
				tag := fmt.Sprintf("storm-w%d-g%d", w, i)
				if _, err := mgr.Swap("echo", typeReg("echo", echoImpl(tag, rec))); err != nil {
					// A racing Remove can legitimately fail a refresh
					// and roll the swap back; anything else must not
					// wedge the manager.
					if mgr.Generation("echo") < 0 {
						errs <- fmt.Errorf("generation went negative: %w", err)
					}
				}
			}
		}(w)
	}
	for c := range creators {
		wg.Add(1)
		go func(c int) {
			defer wg.Done()
			for i := range entriesEach {
				id := fmt.Sprintf("storm-%d-%d", c, i)
				if _, err := tree.Create(loader.EntryOptions{ID: id, Name: "echo", Config: echoConf{Msg: id}}, "", -1); err != nil {
					errs <- fmt.Errorf("create %s: %w", id, err)
					continue
				}
				if i%2 == 0 {
					if err := tree.Remove(id); err != nil {
						errs <- fmt.Errorf("remove %s: %w", id, err)
					}
				}
			}
			// One leftover swap-adjacent id per creator exercises Remove
			// racing Refresh from the other direction.
			id := fmt.Sprintf("late-%d", c)
			if _, err := tree.Create(loader.EntryOptions{ID: id, Name: "echo", Config: echoConf{Msg: id}}, "", -1); err != nil {
				errs <- fmt.Errorf("create %s: %w", id, err)
			}
		}(c)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	tree.Await()
	for _, e := range tree.Entries() {
		if f := e.Fiber(); f != nil && f.State() != cordis.StateActive {
			t.Fatalf("entry %s settled in %s, want active", e.ID(), f.State())
		}
	}

	if _, err := mgr.Swap("echo", typeReg("echo", echoImpl("final", rec))); err != nil {
		t.Fatalf("post-storm swap failed: %v", err)
	}
	tree.Await()
	if fiberState(t, tree, "late-0") != cordis.StateActive {
		t.Fatal("surviving entry not active after the final swap")
	}
}

func TestSwapNewModuleRegistersWithoutReload(t *testing.T) {
	mgr, tree, _, rec := setup(t)
	createEntry(t, tree, loader.EntryOptions{ID: "a", Name: "echo", Config: echoConf{Msg: "a"}})

	report, err := mgr.Swap("ghost", typeReg("ghost", echoImpl("ghost1", rec)))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Reloaded) != 0 || report.Generation != 1 {
		t.Fatalf("report = %+v", report)
	}
	if mgr.Generation("ghost") != 1 {
		t.Fatalf("generation = %d, want 1", mgr.Generation("ghost"))
	}
}
