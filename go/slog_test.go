package cordis

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestSlogHandlerRoutesIntoLoggerService(t *testing.T) {
	ctx := New()
	logger := slog.New(NewSlogHandler(ctx, "worker"))

	logger.Warn("disk almost full", "percent", 91)

	buffer := ctx.core.logger.Buffer()
	if len(buffer) != 1 {
		t.Fatalf("expected one buffered message, got %d", len(buffer))
	}
	m := buffer[0]
	if m.Name != "worker" {
		t.Fatalf("expected name %q, got %q", "worker", m.Name)
	}
	if m.Level != LevelWarn || m.Type != "warn" {
		t.Fatalf("expected warn level, got %s/%s", m.Level, m.Type)
	}
	if got := FormatMessage(m); got != "disk almost full percent=91" {
		t.Fatalf("unexpected rendering: %q", got)
	}
}

func TestSlogHandlerLevelFiltering(t *testing.T) {
	ctx := New()
	logger := slog.New(NewSlogHandler(ctx))
	logger.Debug("dropped")
	logger.Info("kept")
	logger.Error("kept too")

	var levels []Level
	for _, m := range ctx.core.logger.Buffer() {
		levels = append(levels, m.Level)
	}
	if len(levels) != 2 || levels[0] != LevelInfo || levels[1] != LevelError {
		t.Fatalf("expected info+error only (default target), got %v", levels)
	}
}

func TestSlogHandlerHonorsLevelIntercept(t *testing.T) {
	ctx := New()
	intercepted := ctx.Intercept("logger", LoggerIntercept{Level: LevelError})
	logger := slog.New(NewSlogHandler(intercepted))

	if logger.Enabled(context.TODO(), slog.LevelWarn) {
		t.Fatal("warn must be disabled below the error intercept")
	}
	logger.Warn("dropped")
	logger.Error("kept")
	if got := len(ctx.core.logger.Buffer()); got != 1 {
		t.Fatalf("expected only the error record, got %d", got)
	}
}

func TestSlogHandlerAttrsAndGroups(t *testing.T) {
	ctx := New()
	logger := slog.New(NewSlogHandler(ctx)).
		With("service", "billing").
		WithGroup("request").With("id", 7)

	logger.Info("handled")

	m := ctx.core.logger.Buffer()[0]
	got := FormatMessage(m)
	for _, want := range []string{"handled", "service=billing", "request.id=7"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in %q", want, got)
		}
	}
}

func TestSlogHandlerGroupAttrsFlatten(t *testing.T) {
	ctx := New()
	logger := slog.New(NewSlogHandler(ctx))
	logger.Info("with group", slog.Group("g", slog.Int("a", 1)))

	m := ctx.core.logger.Buffer()[0]
	if got := FormatMessage(m); !strings.Contains(got, "g.a=1") {
		t.Fatalf("expected flattened group key, got %q", got)
	}
}

func TestSlogHandlerReachesExporters(t *testing.T) {
	ctx := New()
	var out bytes.Buffer
	dispose := ctx.core.logger.AddExporter(NewConsoleExporter(&out))
	defer dispose()

	logger := slog.New(NewSlogHandler(ctx, "worker"))
	logger.Info("hello", "user", "ada")

	if got := out.String(); !strings.Contains(got, "[info] worker: hello user=ada") {
		t.Fatalf("unexpected console output %q", got)
	}
}

func TestLoggerSlog(t *testing.T) {
	ctx := New()
	ctx.Logger("app").Slog().Error("boom", "code", 500)

	buffer := ctx.core.logger.Buffer()
	if len(buffer) != 1 || buffer[0].Name != "app" || buffer[0].Level != LevelError {
		t.Fatalf("expected one error message from app, got %+v", buffer)
	}
}
