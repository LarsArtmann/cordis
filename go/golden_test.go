package cordis

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

// goldenRunner executes the shared cross-language scenario
// (golden/scenario.txt) and emits the canonical trace compared against
// golden/expected.txt. Rust and Zig ship structurally identical runners.
type goldenRunner struct {
	t        *testing.T
	ctx      *Context
	trace    []string
	fibers   map[string]*Fiber
	plugins  map[string]*Plugin[int]
	children map[string][]childSpec
}

type childSpec struct {
	name   string
	deps   []string
	config int
}

func (r *goldenRunner) logf(format string, args ...any) {
	r.trace = append(r.trace, fmt.Sprintf(format, args...))
}

func (r *goldenRunner) attach(ctx *Context, label string) {
	if _, err := ctx.registerCleanup(label, func() { r.logf("cleanup %s", label) }); err != nil {
		r.t.Fatal(err)
	}
}

func (r *goldenRunner) plugin(name string, deps []string, lifo bool) *Plugin[int] {
	if p, ok := r.plugins[name]; ok {
		return p
	}
	p := NewPlugin(name, func(ctx *Context, config int) error {
		r.logf("apply %s config=%d", name, config)
		if lifo {
			for i := 1; i <= 3; i++ {
				r.attach(ctx, fmt.Sprintf("%s#%d", name, i))
			}
		} else {
			r.attach(ctx, name)
		}
		for _, spec := range r.children[name] {
			fiber, err := Start(ctx, r.plugin(spec.name, spec.deps, false), spec.config)
			if err != nil {
				return err
			}
			r.fibers[spec.name] = fiber
		}
		return nil
	}).Inject(deps...)
	r.plugins[name] = p
	return p
}

// providerFor models a service provider as a plugin: withdrawing the
// service is disposing the provider fiber, which every port supports
// uniformly.
func (r *goldenRunner) providerFor(name string) *Plugin[int] {
	key := "provider:" + name
	if p, ok := r.plugins[key]; ok {
		return p
	}
	p := NewPlugin(key, func(ctx *Context, _ int) error {
		if _, err := ctx.Provide(name, 1); err != nil {
			return err
		}
		return nil
	})
	r.plugins[key] = p
	return p
}

func (r *goldenRunner) realmScope(name, label string) *Context {
	return r.ctx.Isolate(name, label)
}

func splitKV(tokens []string) (deps []string, realm string, config int, lifo bool) {
	config = 0
	for _, tok := range tokens {
		switch {
		case strings.HasPrefix(tok, "inject="):
			for dep := range strings.SplitSeq(strings.TrimPrefix(tok, "inject="), ",") {
				if dep != "" {
					deps = append(deps, dep)
				}
			}
		case strings.HasPrefix(tok, "realm="):
			realm = strings.TrimPrefix(tok, "realm=")
		case strings.HasPrefix(tok, "config="):
			if _, err := fmt.Sscanf(strings.TrimPrefix(tok, "config="), "%d", &config); err != nil {
				panic(err)
			}
		case tok == "lifo":
			lifo = true
		}
	}
	return deps, realm, config, lifo
}

func (r *goldenRunner) start(scope *Context, name string, deps []string, config int, lifo bool) {
	fiber, err := Start(scope, r.plugin(name, deps, lifo), config)
	if err != nil {
		r.t.Fatal(err)
	}
	r.fibers[name] = fiber
}

func (r *goldenRunner) provide(scope *Context, name, realm string) {
	if realm != "" {
		scope = r.realmScope(name, realm)
	}
	fiber, err := Start(scope, r.providerFor(name), 0)
	if err != nil {
		r.t.Fatal(err)
	}
	r.fibers["provider:"+name] = fiber
	r.logf("provided %s", name)
}

func (r *goldenRunner) withdraw(name, realm string) {
	key := "provider:" + name
	fiber := r.fibers[key]
	if fiber == nil {
		r.t.Fatalf("no provider fiber for %s", name)
	}
	fiber.Dispose()
	r.logf("withdrawn %s", name)
}

func (r *goldenRunner) run(line string) {
	tokens := strings.Fields(line)
	op := tokens[0]
	args := tokens[1:]

	switch op {
	case "provide":
		r.provide(r.ctx, args[0], "")
	case "provide-in-realm":
		deps, realm, _, _ := splitKV(args)
		_ = deps
		r.provide(r.ctx, args[0], realm)
	case "withdraw":
		r.withdraw(args[0], "")
	case "withdraw-in-realm":
		_, realm, _, _ := splitKV(args)
		r.withdraw(args[0], realm)
	case "start":
		deps, _, config, lifo := splitKV(args)
		r.start(r.ctx, args[0], deps, config, lifo)
	case "start-isolated":
		deps, realm, config, _ := splitKV(args)
		scope := r.ctx
		for _, dep := range deps {
			scope = scope.Isolate(dep, realm)
		}
		r.start(scope, args[0], deps, config, false)
	case "update":
		fiber := r.fibers[args[0]]
		if fiber == nil {
			r.t.Fatalf("no fiber for %s", args[0])
		}
		var config int
		if _, err := fmt.Sscanf(args[1], "%d", &config); err != nil {
			r.t.Fatal(err)
		}
		if err := fiber.Update(config); err != nil {
			r.t.Fatal(err)
		}
	case "restart":
		if err := r.fibers[args[0]].Restart(); err != nil {
			r.t.Fatal(err)
		}
	case "dispose":
		r.fibers[args[0]].Dispose()
		r.logf("disposed %s", args[0])
	case "restart-root":
		if err := r.ctx.Fiber().Restart(); err != nil {
			r.t.Fatal(err)
		}
		r.logf("root-restarted")
	case "spawn":
		spec := childSpec{name: args[0]}
		deps, _, config, _ := splitKV(args)
		spec.deps, spec.config = deps, config
		for _, tok := range args {
			if after, ok := strings.CutPrefix(tok, "parent="); ok {
				r.children[after] = append(r.children[after], spec)
			}
		}
	case "delete":
		r.ctx.Registry().Delete(r.plugin(args[0], nil, false))
		r.logf("deleted %s", args[0])
	case "expect-registry-size":
		var want int
		if _, err := fmt.Sscanf(args[0], "%d", &want); err != nil {
			r.t.Fatal(err)
		}
		if got := r.ctx.Registry().Size(); got != want {
			r.t.Fatalf("expected registry size %d, got %d", want, got)
		}
		r.logf("registry-size %d", want)
	case "expect-state":
		fiber := r.fibers[args[0]]
		if fiber == nil {
			r.t.Fatalf("no fiber for %s", args[0])
		}
		if state := fiber.State(); state.String() != args[1] {
			r.t.Fatalf("expected %s %s, got %s\ntrace so far:\n%s",
				args[0], args[1], state, strings.Join(r.trace, "\n"))
		}
		r.logf("state %s %s", args[0], args[1])
	default:
		r.t.Fatalf("unknown op %q", op)
	}
}

func goldenDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "golden")
}

func readGoldenLines(t *testing.T, name string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(goldenDir(t), name))
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

// eventRunner executes golden/scenario-events.txt: listener registration in
// several realms plus filtered and unfiltered emissions.
type eventRunner struct {
	t     *testing.T
	ctx   *Context
	trace *[]string
}

func (r *eventRunner) listener(event, who string) Listener {
	return func(args ...any) any {
		payload, _ := args[0].(int)
		*r.trace = append(*r.trace, fmt.Sprintf("fired %s %s payload=%d", event, who, payload))
		return nil
	}
}

func (r *eventRunner) run(line string) {
	tokens := strings.Fields(line)
	op := tokens[0]
	args := tokens[1:]

	var payload int
	var realm string
	for _, tok := range args[1:] {
		switch {
		case strings.HasPrefix(tok, "payload="):
			if _, err := fmt.Sscanf(strings.TrimPrefix(tok, "payload="), "%d", &payload); err != nil {
				r.t.Fatal(err)
			}
		case strings.HasPrefix(tok, "realm="):
			realm = strings.TrimPrefix(tok, "realm=")
		}
	}
	event := args[0]

	switch op {
	case "on", "on-isolated":
		scope := r.ctx
		who := "root"
		if realm != "" {
			scope = r.ctx.Isolate(event, realm)
			who = "realm=" + realm
		}
		if _, err := scope.On(event, r.listener(event, who)); err != nil {
			r.t.Fatal(err)
		}
	case "on-global":
		if _, err := r.ctx.On(event, r.listener(event, "global"), Global()); err != nil {
			r.t.Fatal(err)
		}
	case "emit":
		r.ctx.Emit(event, payload)
	case "emit-filtered":
		scope := r.ctx.Isolate(event, realm)
		emitter := scope.WithFilter(scope.RealmFilter(event))
		emitter.Emit(event, payload)
	default:
		r.t.Fatalf("unknown op %q", op)
	}
}

// compareGoldenTrace pins a runner trace against the golden file, with a
// GOLDEN_UPDATE escape hatch to regenerate it.
func compareGoldenTrace(t *testing.T, trace []string, expectedFile string) {
	t.Helper()
	if os.Getenv("GOLDEN_UPDATE") != "" {
		body := strings.Join(trace, "\n") + "\n"
		if err := os.WriteFile(filepath.Join(goldenDir(t), expectedFile), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	expected := readGoldenLines(t, expectedFile)
	for i, want := range expected {
		if i >= len(trace) {
			t.Fatalf("trace ended early: expected %q at line %d", want, i+1)
		}
		if trace[i] != want {
			t.Fatalf("trace divergence at line %d:\n  want: %s\n  got:  %s", i+1, want, trace[i])
		}
	}
	if len(trace) != len(expected) {
		t.Fatalf("trace length %d, expected %d", len(trace), len(expected))
	}
}

func TestGoldenEvents(t *testing.T) {
	var trace []string
	r := &eventRunner{t: t, ctx: New(), trace: &trace}
	for _, line := range readGoldenLines(t, "scenario-events.txt") {
		r.run(line)
	}
	compareGoldenTrace(t, trace, "expected-events.txt")
}

func TestGoldenScenario(t *testing.T) {
	runScenario(t, "scenario.txt", "expected.txt", func(r *goldenRunner, line string) { r.run(line) })
}

func runScenario(t *testing.T, scenarioFile, expectedFile string, exec func(r *goldenRunner, line string)) {
	scenario := readGoldenLines(t, scenarioFile)
	r := &goldenRunner{
		t:        t,
		ctx:      New(),
		fibers:   map[string]*Fiber{},
		plugins:  map[string]*Plugin[int]{},
		children: map[string][]childSpec{},
	}
	for _, line := range scenario {
		exec(r, line)
	}

	if os.Getenv("GOLDEN_UPDATE") != "" {
		body := strings.Join(r.trace, "\n") + "\n"
		if err := os.WriteFile(filepath.Join(goldenDir(t), expectedFile), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	expected := readGoldenLines(t, expectedFile)

	for i, want := range expected {
		if i >= len(r.trace) {
			t.Fatalf("trace ended early: expected %q at line %d", want, i+1)
		}
		if r.trace[i] != want {
			t.Fatalf("trace divergence at line %d:\n  want: %s\n  got:  %s", i+1, want, r.trace[i])
		}
	}
	if len(r.trace) != len(expected) {
		t.Fatalf("trace length %d, expected %d", len(r.trace), len(expected))
	}
}

func TestGoldenCascade(t *testing.T) {
	runScenario(t, "scenario-cascade.txt", "expected-cascade.txt", func(r *goldenRunner, line string) { r.run(line) })
}

func TestGoldenSplitKV(t *testing.T) {
	deps, realm, config, lifo := splitKV([]string{
		"inject=a,,b", "realm=tenant", "config=7", "lifo", "ignored",
	})
	if strings.Join(deps, ",") != "a,b" {
		t.Fatal("deps parsing broken:", deps)
	}
	if realm != "tenant" || config != 7 || !lifo {
		t.Fatal("kv parsing broken:", realm, config, lifo)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("malformed config must panic")
		}
	}()
	_, _, _, _ = splitKV([]string{"config=not-a-number"})
}

// dispatchRunner executes golden/scenario-dispatch.txt: the bail, serial,
// waterfall and parallel dispatch modes over logging listeners. Listener
// payloads are ints; parallel listeners count instead of logging so the
// trace stays independent of goroutine interleaving.
type dispatchRunner struct {
	t        *testing.T
	ctx      *Context
	trace    *[]string
	counters map[string]*atomic.Int64
}

func (r *dispatchRunner) logf(format string, args ...any) {
	*r.trace = append(*r.trace, fmt.Sprintf(format, args...))
}

func (r *dispatchRunner) mustOn(event string, listener Listener) {
	if _, err := r.ctx.On(event, listener); err != nil {
		r.t.Fatal(err)
	}
}

// parseDispatchKV parses key=value tokens into a map.
func parseDispatchKV(tokens []string) map[string]string {
	kv := make(map[string]string)
	for _, tok := range tokens {
		if i := strings.Index(tok, "="); i > 0 {
			kv[tok[:i]] = tok[i+1:]
		}
	}
	return kv
}

func (r *dispatchRunner) run(line string) {
	tokens := strings.Fields(line)
	op := tokens[0]
	args := tokens[1:]
	kv := parseDispatchKV(args[1:])
	event := args[0]

	switch op {
	case "listener":
		name := args[1]
		returns, bails := kv["returns"], false
		if kv["returns"] != "" {
			bails = true
		}
		var ret int
		if bails {
			if _, err := fmt.Sscanf(returns, "%d", &ret); err != nil {
				r.t.Fatal(err)
			}
		}
		r.mustOn(event, func(v ...any) any {
			payload := v[0].(int)
			r.logf("fire %s %s payload=%d", event, name, payload)
			if bails {
				return payload + ret
			}
			return nil
		})
	case "wf-listener":
		name := args[1]
		var add int
		if _, err := fmt.Sscanf(kv["add"], "%d", &add); err != nil {
			r.t.Fatal(err)
		}
		r.mustOn(event, func(v ...any) any {
			payload := v[0].(int)
			next := v[1].(func(...any) any)
			r.logf("wf %s %s payload=%d", event, name, payload)
			return next(payload + add)
		})
	case "wf-cut":
		name := args[1]
		var ret int
		if _, err := fmt.Sscanf(kv["returns"], "%d", &ret); err != nil {
			r.t.Fatal(err)
		}
		r.mustOn(event, func(v ...any) any {
			payload := v[0].(int)
			r.logf("wf %s %s payload=%d", event, name, payload)
			return payload + ret
		})
	case "quiet":
		c := r.counters[event]
		if c == nil {
			c = &atomic.Int64{}
			r.counters[event] = c
		}
		r.mustOn(event, func(v ...any) any {
			c.Add(1)
			return nil
		})
	case "bail", "serial":
		var payload int
		if _, err := fmt.Sscanf(kv["payload"], "%d", &payload); err != nil {
			r.t.Fatal(err)
		}
		var result any
		if op == "bail" {
			result = r.ctx.Bail(event, payload)
		} else {
			result = r.ctx.Serial(event, payload)
		}
		if result == nil {
			r.logf("%s %s result=none", op, event)
		} else {
			r.logf("%s %s result=%d", op, event, result.(int))
		}
	case "waterfall":
		var payload int
		if _, err := fmt.Sscanf(kv["payload"], "%d", &payload); err != nil {
			r.t.Fatal(err)
		}
		terminal := func(v ...any) any {
			p := v[0].(int)
			r.logf("wf-terminal %s payload=%d", event, p)
			return p + 1000
		}
		result := r.ctx.Waterfall(event, payload, terminal)
		if result == nil {
			r.logf("waterfall %s result=none", event)
		} else {
			r.logf("waterfall %s result=%d", event, result.(int))
		}
	case "parallel":
		var payload int
		if _, err := fmt.Sscanf(kv["payload"], "%d", &payload); err != nil {
			r.t.Fatal(err)
		}
		c := r.counters[event]
		fired := int64(0)
		if c != nil {
			c.Store(0)
		}
		if err := r.ctx.Parallel(event, payload); err != nil {
			r.t.Fatalf("parallel %s: %v", event, err)
		}
		if c != nil {
			fired = c.Load()
		}
		r.logf("parallel %s fired=%d", event, fired)
	default:
		r.t.Fatalf("unknown op %q", op)
	}
}

func TestGoldenDispatch(t *testing.T) {
	scenario := readGoldenLines(t, "scenario-dispatch.txt")
	trace := make([]string, 0, 16)
	r := &dispatchRunner{
		t:        t,
		ctx:      New(),
		trace:    &trace,
		counters: map[string]*atomic.Int64{},
	}
	for _, line := range scenario {
		r.run(line)
	}
	compareGoldenTrace(t, trace, "expected-dispatch.txt")
}

func TestParseDispatchKV(t *testing.T) {
	kv := parseDispatchKV([]string{"b", "payload=7", "flag", "returns=-3"})
	if kv["payload"] != "7" || kv["returns"] != "-3" || kv["flag"] != "" || kv["b"] != "" {
		t.Fatalf("kv = %v", kv)
	}
}
