package cordis_test

import (
	"testing"

	cordis "github.com/LarsArtmann/cordis/go"
)

// Benchmarks for the hot paths of the core. Run with
// `go test -bench . -benchmem .`.

type benchConfig struct {
	N int
}

type benchEvent struct {
	Seq int
}

func BenchmarkStartDispose(b *testing.B) {
	ctx := cordis.New()
	handle := cordis.NewPlugin("bench", func(ctx *cordis.Context, conf *benchConfig) error {
		_, err := ctx.Cleanup("bench", func() {})
		return err
	})

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		fiber, err := cordis.StartAny(ctx, handle, &benchConfig{N: i})
		if err != nil {
			b.Fatal(err)
		}
		fiber.Dispose()
	}
}

func BenchmarkProvideDisposeGet(b *testing.B) {
	ctx := cordis.New()

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		d, err := cordis.Provide(ctx, &benchConfig{N: i})
		if err != nil {
			b.Fatal(err)
		}
		if _, err := cordis.Get[*benchConfig](ctx); err != nil {
			b.Fatal(err)
		}
		d()
	}
}

func BenchmarkGet(b *testing.B) {
	ctx := cordis.New()
	if _, err := cordis.Provide(ctx, &benchConfig{N: 1}); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := cordis.Get[*benchConfig](ctx); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEventEmit(b *testing.B) {
	ctx := cordis.New()
	if _, err := ctx.On("bench/event", func(args ...any) any { return nil }); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ctx.Emit("bench/event", i)
	}
}

func BenchmarkWaterfallEvent(b *testing.B) {
	ctx := cordis.New()
	for range 5 {
		if _, err := ctx.On("bench/waterfall", func(args ...any) any {
			next, _ := args[len(args)-1].(func(...any) any)
			return next(args[:len(args)-1]...)
		}); err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ctx.Waterfall("bench/waterfall", i, func(args ...any) any { return nil })
	}
}

func BenchmarkTypedEventDispatch(b *testing.B) {
	ctx := cordis.New()
	if _, err := cordis.On[benchEvent](ctx, func(benchEvent) {}); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cordis.Emit(ctx, benchEvent{Seq: i})
	}
}
