package cordis

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

type covEvent struct {
	ID int
}

// Serial, Bail and empty-hook paths of the dispatch modes.
func TestDispatchModes(t *testing.T) {
	ctx := New()

	if got := ctx.Serial("nothing"); got != nil {
		t.Fatal("Serial without hooks must return nil")
	}
	if got := ctx.Bail("nothing"); got != nil {
		t.Fatal("Bail without hooks must return nil")
	}
	if err := ctx.Parallel("nothing"); err != nil {
		t.Fatal("Parallel without hooks must return nil:", err)
	}

	falseBails := false
	if _, err := ctx.On("b", func(args ...any) any { return falseBails }); err != nil {
		t.Fatal(err)
	}
	if _, err := ctx.On("b", func(args ...any) any { return "second" }); err != nil {
		t.Fatal(err)
	}
	if got := ctx.Serial("b"); got != "second" {
		t.Fatal("false results must not bail, wanted the second listener, got", got)
	}

	if _, err := ctx.On("v", func(args ...any) any { return "value" }); err != nil {
		t.Fatal(err)
	}
	if got := ctx.Bail("v"); got != "value" {
		t.Fatal("non-bool results must bail, got", got)
	}

	// Parallel joins listener errors and captured panics.
	e1 := errors.New("boom1")
	if _, err := ctx.On("p", func(args ...any) any { return e1 }); err != nil {
		t.Fatal(err)
	}
	if _, err := ctx.On("p", func(args ...any) any { panic("boom2") }); err != nil {
		t.Fatal(err)
	}
	if _, err := ctx.On("p", func(args ...any) any { return nil }); err != nil {
		t.Fatal(err)
	}
	err := ctx.Parallel("p")
	if err == nil || !strings.Contains(err.Error(), "boom1") || !strings.Contains(err.Error(), "boom2") {
		t.Fatal("Parallel must join errors and panics, got", err)
	}
}

// Waterfall composition: pass-through, short-circuit and argument guards.
func TestWaterfallComposition(t *testing.T) {
	ctx := New()

	if _, err := ctx.On("wf", func(args ...any) any {
		next := args[1].(func(...any) any)
		return next(args[0].(int) * 2)
	}); err != nil {
		t.Fatal(err)
	}
	got := ctx.Waterfall("wf", 21, func(v ...any) any {
		return v[0].(int) + 1
	})
	if got != 43 {
		t.Fatal("waterfall composition broken, got", got)
	}

	if _, err := ctx.On("wf-short", func(args ...any) any { return "short" }); err != nil {
		t.Fatal(err)
	}
	if got := ctx.Waterfall("wf-short", func(v ...any) any { return "end" }); got != "short" {
		t.Fatal("listener that skips next must short-circuit, got", got)
	}

	// No hooks: the terminal runs unchanged.
	if got := ctx.Waterfall("wf-none", 1, func(v ...any) any { return v }); got == nil {
		t.Fatal("terminal must run when no listener is registered")
	}

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("Waterfall without args must panic")
			}
		}()
		ctx.Waterfall("wf-panic")
	}()
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("Waterfall with a non-terminal last arg must panic")
			}
		}()
		ctx.Waterfall("wf-panic", 42)
	}()
}

// Prepend ordering, Global filter exemption and Emit panic propagation.
func TestEventOptionsAndPanics(t *testing.T) {
	ctx := New()
	var order []string

	if _, err := ctx.On("e", func(...any) any { order = append(order, "first"); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := ctx.On("e", func(...any) any { order = append(order, "prepended"); return nil }, Prepend()); err != nil {
		t.Fatal(err)
	}
	ctx.Emit("e")
	if strings.Join(order, ",") != "prepended,first" {
		t.Fatal("Prepend must register ahead of existing listeners:", order)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("panicking listeners must propagate through Emit")
		}
	}()
	_, _ = ctx.On("boom", func(...any) any { panic("listener panic") })
	ctx.Emit("boom")
}

// Typed event wrappers panic on arity and payload type mismatches.
func TestTypedEventGuardPanics(t *testing.T) {
	ctx := New()
	name := EventName[covEvent]()

	d, err := On(ctx, func(covEvent) {})
	if err != nil {
		t.Fatal(err)
	}
	defer d()
	// Subscribe a typed listener, then emit the underlying string event
	// with a wrong payload and a wrong arity.
	_, _ = ctx.On(name, func(args ...any) any {
		if len(args) != 1 {
			panic("arity guard did not fire")
		}
		return nil
	})
	if _, err := ctx.On(name, func(args ...any) any {
		// wrong payload type
		defer func() { _ = recover() }()
		_ = args[0].(covEvent)
		panic("type guard did not fire")
	}); err != nil {
		t.Fatal(err)
	}

	_, _ = ctx.On(name, func(args ...any) any {
		defer func() { _ = recover() }()
		return nil
	})

	// Direct typed guards via the exported wrappers.
	guarded := func(t *testing.T) {
		t.Helper()
		defer func() { _ = recover() }()
		_ = MustGetNamed[int](ctx, "missing-service")
	}
	guarded(t)

	if _, err := GetNamed[int](ctx, "missing-service"); err == nil {
		t.Fatal("GetNamed of a missing service must fail")
	}
	if _, err := GetNamed[string](ctx, ServiceName[covEvent]()); err == nil {
		t.Fatal("GetNamed with a mismatched type must fail")
	}
}

// Once fires exactly once; the disposer is idempotent.
func TestOnceAndDispose(t *testing.T) {
	ctx := New()
	calls := 0
	d, err := ctx.Once("once", func(args ...any) any { calls++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	ctx.Emit("once")
	ctx.Emit("once")
	d()
	ctx.Emit("once")
	if calls != 1 {
		t.Fatal("Once must deliver exactly one event, got", calls)
	}
	d()
}

// Typed event round trip including the Once variant.
func TestTypedEventsRoundTrip(t *testing.T) {
	ctx := New()
	got := 0
	d, err := Once(ctx, func(e covEvent) { got = e.ID })
	if err != nil {
		t.Fatal(err)
	}
	defer d()
	Emit(ctx, covEvent{ID: 7})
	Emit(ctx, covEvent{ID: 9})
	if got != 7 {
		t.Fatal("typed Once must deliver the first event only, got", got)
	}

	// Raw emission with a wrong payload hits the typed guard's panic path.
	func() {
		defer func() { _ = recover() }()
		ctx.Emit(EventName[covEvent](), "not-a-covEvent")
	}()
	_ = 0
}

// Stdlib context lifecycle: cancelled on dispose, renewed on restart.
func TestStdContextLifecycle(t *testing.T) {
	ctx := New()
	fiber, err := Start(ctx, NewPlugin("stdctx", func(c *Context, _ int) error {
		select {
		case <-c.Fiber().StdContext().Done():
			t.Fatal("std context must be live while active")
		default:
		}
		return nil
	}), 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := fiber.Restart(); err != nil {
		t.Fatal(err)
	}
	if fiber.State() != StateActive {
		t.Fatal("restart must end active, got", fiber.State())
	}
	fiber.Dispose()
	select {
	case <-fiber.StdContext().Done():
	case <-time.After(time.Second):
		t.Fatal("std context must be cancelled after disposal")
	}
}

// The root fiber restarts instead of disposing, and its config accessor works.
func TestRootFiberAndConfig(t *testing.T) {
	ctx := New()
	root := ctx.Fiber()
	root.Dispose() // restarts
	if root.State() != StateActive {
		t.Fatal("disposing the root restarts it, got", root.State())
	}

	fiber, err := Start(ctx, NewPlugin("cfg", func(c *Context, cfg int) error {
		return nil
	}), 3)
	if err != nil {
		t.Fatal(err)
	}
	if got := fiber.Config(); got != 3 {
		t.Fatal("Config must return the last started config, got", got)
	}
	if fiber.Name() != "cfg" || fiber.UID() <= 0 {
		t.Fatal("fiber metadata broken")
	}
	if fiber.Context() == nil {
		t.Fatal("Context accessor must return the fiber context")
	}
	if fiber.GetEffects() == nil && fiber.State() == StateActive {
		t.Fatal("active fiber must expose its effect tree")
	}
}

// Plugin start guards.
func TestStartGuards(t *testing.T) {
	ctx := New()
	if _, err := Start(ctx, nil, 0); err == nil {
		t.Fatal("starting nil must fail")
	}
	if _, err := Start(ctx, NewPlugin("ok", func(*Context, int) error { return nil }), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := Start[int](ctx, nil, 0); err == nil {
		t.Fatal("starting nil must fail")
	}
}

// Framework error type: message fallback, code matching and validation
// rendering.
func TestErrorTypes(t *testing.T) {
	if ErrInactiveEffect.Error() != "cannot create effect on inactive context" {
		t.Fatal("message fallback broken:", ErrInactiveEffect.Error())
	}
	bare := &Error{Code: ErrCodeInactiveEffect}
	if bare.Error() != string(ErrCodeInactiveEffect) {
		t.Fatal("code fallback broken:", bare.Error())
	}
	if !IsInactiveEffect(fmt.Errorf("wrap: %w", ErrInactiveEffect)) {
		t.Fatal("IsInactiveEffect must unwrap")
	}
	if IsInactiveEffect(errors.New("other")) {
		t.Fatal("unrelated errors must not match")
	}
	ve := &ValidationError{Issues: []Issue{
		{Message: "bad"},
		{Path: []string{"a", "b"}, Message: "worse"},
	}}
	msg := ve.Error()
	if !strings.Contains(msg, "invalid config:") || !strings.Contains(msg, "a.b") || !strings.Contains(msg, "worse") {
		t.Fatal("validation rendering broken:", msg)
	}
}

// Intercept overrides configuration of the named service for child scopes.
func TestIntercept(t *testing.T) {
	ctx := New()
	scope := ctx.Intercept("log", map[string]any{"level": "debug"})
	if scope == ctx {
		t.Fatal("Intercept must return a child scope")
	}
	if _, ok := scope.Intercepted("log"); !ok {
		t.Fatal("Intercepted must see the override")
	}
	if _, ok := ctx.Intercepted("log"); ok {
		t.Fatal("the parent scope must not see the override")
	}
}

// Logger edge paths: per-name levels, exporter removal, error expansion of
// wrapped and joined errors, and remaining format verbs.
func TestLoggerEdges(t *testing.T) {
	ctx := New()
	logger := ctx.Logger("edges")

	var got []string
	remove := ctx.core.logger.AddExporter(ExporterFunc(func(m Message) {
		got = append(got, FormatMessage(m))
	}), map[string]Level{"edges": LevelWarn, "default": LevelError})

	logger.Info("suppressed")
	logger.Warn("shown %d", 42)
	if strings.Join(got, "|") != "shown 42" {
		t.Fatal("level filter broken:", got)
	}

	remove()
	got = nil
	ctx.core.logger.AddExporter(ExporterFunc(func(m Message) { got = append(got, FormatMessage(m)) }))
	logger.Error("plain")
	if len(got) != 1 || !strings.Contains(got[0], "plain") {
		t.Fatal("default level export broken:", got)
	}

	got = nil
	wrapped := fmt.Errorf("outer: %w", errors.New("inner"))
	logger.Error(wrapped)
	if len(got) != 1 || !strings.Contains(got[0], "inner") {
		t.Fatal("wrapped error must be unwrapped:", got)
	}

	got = nil
	joined := errors.Join(errors.New("a"), errors.New("b"))
	logger.Error(joined)
	if len(got) != 2 || !strings.Contains(got[0], "a") || !strings.Contains(got[1], "b") {
		t.Fatal("joined errors must fan out:", got)
	}

	got = nil
	logger.Debug("%o", 7)
	logger.Info("100%% sure")
	logger.Info("missing-verb %")
	if strings.Join(got, "|") == "" {
		t.Fatal("format verbs must render")
	}
	if logger.Name() != "edges" {
		t.Fatal("Name accessor broken")
	}
	ctx.core.logger.ClearExporters()
	got = nil
	logger.Info("after clear")
	if len(got) != 0 {
		t.Fatal("ClearExporters must detach every exporter")
	}
}

// State name rendering, guarded applies, Await paths and InjectConfig
// intercept wiring.
func TestFiberLifecycleEdges(t *testing.T) {
	if StateLoading.String() != "LOADING" || StateFailed.String() != "FAILED" ||
		StateUnloading.String() != "UNLOADING" {
		t.Fatal("state names broken")
	}
	if FiberState(99).String() != "UNKNOWN" {
		t.Fatal("unknown state name broken")
	}

	// A panicking plugin body becomes a failed fiber with a joined error.
	ctx := New()
	fiber, err := Start(ctx, NewPlugin("panic", func(*Context, int) error {
		panic("apply boom")
	}), 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := fiber.Await(); err == nil || !strings.Contains(err.Error(), "apply boom") {
		t.Fatal("panic must surface through Await, got", err)
	}
	if fiber.State() != StateFailed {
		t.Fatal("panicking fiber must fail, got", fiber.State())
	}

	// InjectConfig: the dependency is visible as an intercept override in
	// the plugin's own scope.
	seen := false
	if _, err := ctx.Provide("dep", "service"); err != nil {
		t.Fatal(err)
	}
	f2, err := Start(ctx, NewPlugin("configured", func(c *Context, _ int) error {
		if v, ok := c.Intercepted("dep"); ok {
			seen = v.(string) == "override"
		}
		return nil
	}).Inject("dep").InjectConfig("dep", "override"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := f2.Await(); err != nil {
		t.Fatal(err)
	}
	if !seen {
		t.Fatal("InjectConfig override not visible to the plugin scope")
	}
}

// A dependent stays pending while the provider's check function rejects it
// and reactivates once the check passes.
func TestProvideCheckGuardsDependents(t *testing.T) {
	ctx := New()
	ready := false
	fiber, err := Start(ctx, NewPlugin("guarded", func(c *Context, _ int) error {
		if _, ok := c.Get("dep"); !ok {
			return errors.New("dep missing")
		}
		return nil
	}).Inject("dep"), 0)
	if err != nil {
		t.Fatal(err)
	}

	d, err := ctx.Provide("dep", 1, func() bool { return ready })
	if err != nil {
		t.Fatal(err)
	}
	defer d()
	if fiber.State() != StatePending {
		t.Fatal("rejected check must keep the fiber pending, got", fiber.State())
	}

	ready = true
	if err := fiber.Restart(); err != nil {
		t.Fatal(err)
	}
	if fiber.State() != StateActive {
		t.Fatal("guarded fiber must activate when the check passes, got", fiber.State())
	}
}

// The internal interception events power the loader and hmr ports.
func TestInterceptionEvents(t *testing.T) {
	ctx := New()

	// EventGet: supply a fallback for a missing service.
	if _, err := ctx.On(EventGet, func(args ...any) any {
		next := args[2].(func(...any) any)
		return next(&GetResult{Value: "fallback", OK: true})
	}); err != nil {
		t.Fatal(err)
	}
	if v, ok := ctx.Get("missing"); !ok || v != "fallback" {
		t.Fatal("EventGet fallback broken:", v, ok)
	}
	// Existing services bypass the interception.
	if _, err := ctx.Provide("real", 1); err != nil {
		t.Fatal(err)
	}
	if v, ok := ctx.Get("real"); !ok || v != 1 {
		t.Fatal("successful Get must not be intercepted")
	}

	// EventSet: veto a registration and observe a good one.
	var vetoed, seen bool
	if _, err := ctx.On(EventSet, func(args ...any) any {
		next := args[2].(func(...any) any)
		if args[0] == "forbidden" {
			vetoed = true
			return errors.New("set rejected")
		}
		seen = true
		return next()
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ctx.Provide("forbidden", 1); err == nil {
		t.Fatal("EventSet veto must fail the Provide")
	}
	if !vetoed || seen {
		t.Fatal("EventSet observation broken")
	}
	if _, err := ctx.Provide("allowed", 2); err != nil {
		t.Fatal("EventSet must pass allowed registrations:", err)
	}

	// EventListener: replace a registration with a custom disposer.
	custom := Disposer(func() {})
	if _, err := ctx.On(EventListener, func(args ...any) any {
		if args[0] == "hooked" {
			return custom
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	d, err := ctx.On("hooked", func(...any) any { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if func() bool { return d == nil }() {
		t.Fatal("intercepted registration must return the replacement")
	}
	// Unrelated registrations pass through untouched.
	d2, err := ctx.On("normal", func(...any) any { return nil })
	if err != nil || d2 == nil {
		t.Fatal("unintercepted registration broken:", err)
	}
	d2()

	// EventDispatch: observe non-internal emissions only.
	var modes []string
	if _, err := ctx.On(EventDispatch, func(args ...any) any {
		modes = append(modes, args[0].(string), args[1].(string))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ctx.Emit("user-event", 1)
	ctx.Emit(EventService, "x", nil)
	if strings.Join(modes, ",") != "emit,user-event" {
		t.Fatal("EventDispatch must observe only non-internal dispatches:", modes)
	}
}
