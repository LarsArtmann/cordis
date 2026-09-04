package cordis

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// goldenRunner executes the shared cross-language scenario
// (golden/scenario.txt) and emits the canonical trace compared against
// golden/expected.txt. Rust and Zig ship structurally identical runners.
type goldenRunner struct {
	t       *testing.T
	ctx     *Context
	trace   []string
	fibers  map[string]*Fiber
	plugins map[string]*Plugin[int]
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
			return nil
		}
		r.attach(ctx, name)
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
			for _, dep := range strings.Split(strings.TrimPrefix(tok, "inject="), ",") {
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
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func TestGoldenScenario(t *testing.T) {
	scenario := readGoldenLines(t, "scenario.txt")
	r := &goldenRunner{
		t:       t,
		ctx:     New(),
		fibers:  map[string]*Fiber{},
		plugins: map[string]*Plugin[int]{},
	}
	for _, line := range scenario {
		r.run(line)
	}

	if os.Getenv("GOLDEN_UPDATE") != "" {
		body := strings.Join(r.trace, "\n") + "\n"
		if err := os.WriteFile(filepath.Join(goldenDir(t), "expected.txt"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	expected := readGoldenLines(t, "expected.txt")

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
