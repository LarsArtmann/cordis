package cordis_test

import (
	"reflect"
	"testing"

	cordis "github.com/LarsArtmann/cordis/go"
)

// Pins upstream #206: context intercepts merge from the outermost scope to
// the nearest one, with the inject configs of governing fibers underneath.

func TestInterceptChainMergesOuterToInner(t *testing.T) {
	ctx := cordis.New()

	inner := ctx.
		Intercept("foo", map[string]any{"a": 1, "b": 1}).
		Intercept("foo", map[string]any{"b": 2})

	got, ok := inner.Intercepted("foo")
	if !ok {
		t.Fatal("Intercepted must see the merged override")
	}

	want := map[string]any{"a": 1, "b": 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("outer fields must survive an inner override: got %v, want %v", got, want)
	}

	if _, ok := ctx.Intercepted("foo"); ok {
		t.Fatal("the parent scope must not see the override")
	}
}

func TestInjectConfigMergesUnderScopeIntercepts(t *testing.T) {
	ctx := cordis.New()
	if _, err := ctx.Provide("foo", "service"); err != nil {
		t.Fatal(err)
	}

	var got any

	scoped := ctx.Intercept("foo", map[string]any{"b": 2})

	_, err := cordis.Start(scoped, cordis.NewPlugin("consumer", func(c *cordis.Context, _ int) error {
		inner := c.Intercept("foo", map[string]any{"c": 3})

		var ok bool

		got, ok = inner.Intercepted("foo")
		if !ok {
			t.Fatal("Intercepted must see the merged override")
		}

		return nil
	}).Inject("foo").InjectConfig("foo", map[string]any{"a": 1, "b": 1}), 0)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]any{"a": 1, "b": 2, "c": 3}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("inject config must sit under scope intercepts: got %v, want %v", got, want)
	}
}

func TestStructInterceptsMergeFieldwise(t *testing.T) {
	ctx := cordis.New()

	inner := ctx.
		Intercept("logger", cordis.LoggerIntercept{Level: cordis.LevelError}).
		Intercept("logger", cordis.LoggerIntercept{Name: "worker"})

	got, ok := inner.Intercepted("logger")
	if !ok {
		t.Fatal("Intercepted must see the merged override")
	}

	li, ok := got.(cordis.LoggerIntercept)
	if !ok {
		t.Fatalf("merged intercept must keep the struct type, got %T", got)
	}

	if li.Name != "worker" || li.Level != cordis.LevelError {
		t.Fatalf("non-zero overlay fields must win, unset fields must inherit: got %+v", li)
	}
}
