package cordis

import (
	"errors"
	"reflect"
	"testing"
)

func logMessages(ctx *Context) []Message {
	return ctx.core.logger.Buffer()
}

func logTexts(ctx *Context) []string {
	var out []string
	for _, m := range logMessages(ctx) {
		out = append(out, FormatMessage(m))
	}
	return out
}

func TestLoggerBuffer(t *testing.T) {
	ctx := New()
	logger := ctx.Logger("test")
	ctx.core.logger.SetBufferSize(2)

	logger.Info("one")
	logger.Info("two")
	logger.Info("three")
	if got := logTexts(ctx); !reflect.DeepEqual(got, []string{"two", "three"}) {
		t.Fatalf("expected [two three], got %v", got)
	}

	ctx.core.logger.SetBufferSize(1)
	logger.Info("four")
	if got := logTexts(ctx); !reflect.DeepEqual(got, []string{"four"}) {
		t.Fatalf("expected [four], got %v", got)
	}

	ctx.core.logger.SetBufferSize(0)
	logger.Info("five")
	if got := logTexts(ctx); len(got) != 0 {
		t.Fatalf("expected empty buffer, got %v", got)
	}
}

func TestLoggerExporters(t *testing.T) {
	ctx := New()
	var a, b []string
	removeA := ctx.core.logger.AddExporter(ExporterFunc(func(m Message) {
		a = append(a, FormatMessage(m))
	}))
	ctx.core.logger.AddExporter(ExporterFunc(func(m Message) {
		b = append(b, FormatMessage(m))
	}))

	ctx.Logger("test").Info("hello")
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("expected both exporters, got a=%v b=%v", a, b)
	}
	removeA()
	ctx.Logger("test").Info("world")
	if len(a) != 1 || len(b) != 2 {
		t.Fatalf("expected only exporter b, got a=%v b=%v", a, b)
	}
	removeA() // idempotent
}

func TestLoggerLevels(t *testing.T) {
	ctx := New()
	var got []string
	ctx.core.logger.AddExporter(ExporterFunc(func(m Message) {
		got = append(got, FormatMessage(m))
	}), map[string]Level{"quiet": LevelError})

	ctx.Logger("quiet").Info("suppressed")
	ctx.Logger("quiet").Error("boom")
	ctx.Logger("loud").Info("visible")
	if !reflect.DeepEqual(got, []string{"boom", "visible"}) {
		t.Fatalf("unexpected messages: %v", got)
	}
}

func TestLoggerNameResolution(t *testing.T) {
	ctx := New()

	if name := ctx.Logger().Name(); name != "root" {
		t.Fatalf("expected root, got %s", name)
	}
	if name := ctx.Logger("custom").Name(); name != "custom" {
		t.Fatalf("expected custom, got %s", name)
	}
	scoped := ctx.Intercept("logger", LoggerIntercept{Name: "scoped"})
	if name := scoped.Logger().Name(); name != "scoped" {
		t.Fatalf("expected scoped, got %s", name)
	}
	// Explicit names win over intercepts.
	if name := scoped.Logger("explicit").Name(); name != "explicit" {
		t.Fatalf("expected explicit, got %s", name)
	}

	plugin := NewPlugin("worker", func(ctx *Context, _ struct{}) error { return nil })
	fiber, err := Start(ctx, plugin, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if name := fiber.Context().Logger().Name(); name != "worker" {
		t.Fatalf("expected fiber name, got %s", name)
	}
}

func TestLoggerFormat(t *testing.T) {
	cases := []struct {
		args []any
		want string
	}{
		{[]any{"hello"}, "hello"},
		{[]any{"value: %d", 42.7}, "value: 42"},
		{[]any{"%s %s", "a", "b"}, "a b"},
		{[]any{"%o", map[string]int{"a": 1}}, `{"a":1}`},
		{[]any{"100%%"}, "100%"},
		{[]any{"tail", 1, "two"}, "tail 1 two"},
	}
	for _, tc := range cases {
		got := FormatMessage(Message{Args: tc.args})
		if got != tc.want {
			t.Errorf("FormatMessage(%v) = %q, want %q", tc.args, got, tc.want)
		}
	}
}

func TestLoggerErrorExpansion(t *testing.T) {
	ctx := New()
	var got []string
	ctx.core.logger.AddExporter(ExporterFunc(func(m Message) {
		got = append(got, FormatMessage(m))
	}))
	logger := ctx.Logger("test")
	err := errors.Join(errors.New("one"), errors.New("two"))
	logger.Error(err)
	if !reflect.DeepEqual(got, []string{"one", "two"}) {
		t.Fatalf("expected joined errors expanded, got %v", got)
	}
}
