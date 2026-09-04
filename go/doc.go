// Package cordis is the Go port of Cordis, a meta-framework of
// spatiotemporal composability.
//
// Cordis organizes an application into a tree of contexts. Each context is a
// scope that carries services, event listeners and effects. Every plugin
// instance runs inside a fiber, an effect scope with a lifecycle. When a
// fiber is disposed, everything it registered (event listeners, provided
// services, nested plugins, plain cleanup functions) is rolled back in
// reverse order. Fibers declare service dependencies through inject, and the
// framework activates, unloads and reloads them as dependencies appear and
// disappear. This is the temporal dimension: the set of running code is a
// pure function of the set of available services.
//
// The port keeps the semantics of the TypeScript original and adapts the
// surface to idiomatic Go:
//
//   - Plugin configuration is statically typed through generics.
//   - Asynchronous microtask scheduling becomes a synchronous drain: state
//     transitions are coalesced and executed before the outermost framework
//     call returns, so no torn intermediate states are ever observable.
//   - All shared state is guarded by a single mutex and user callbacks are
//     always invoked without locks held, so listeners and plugins may freely
//     call back into the framework from any goroutine.
//
// A minimal example:
//
//	ctx := cordis.New()
//	logger := cordis.NewPlugin("logger", func(ctx *cordis.Context, cfg LoggerConfig) error {
//		ctx.Provide("logger", &Logger{level: cfg.Level})
//		return nil
//	})
//	cordis.Start(ctx, logger, LoggerConfig{Level: 2})
//
// See ROADMAP.md for the parity status against the TypeScript implementation.
package cordis
