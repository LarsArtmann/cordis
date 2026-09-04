# cordis for Go

The Go port of [Cordis](https://github.com/cordiverse/cordis), a
meta-framework of spatiotemporal composability. This is the flagship port
and the reference implementation for the Rust and Zig ports.

```go
import cordis "github.com/LarsArtmann/cordis/go"
```

## What it gives you

- **Contexts**: hierarchical scopes carrying services, event listeners and
  effects.
- **Fibers**: every plugin instance runs in an effect scope with a
  lifecycle. When a fiber unloads, everything it registered rolls back in
  reverse order.
- **Typed services**: the primary service API keys services by their type
  (`cordis.Provide(ctx, db)` / `cordis.MustGet[*Database](ctx)`), so
  providers and consumers cannot drift apart on a hand written string.
- **Typed events**: `cordis.On(ctx, func(e UserCreated) {...})` /
  `cordis.Emit(ctx, event)` dispatch by event type; string event names
  remain for the framework's `internal/` namespace.
- **Inject reactivity**: a fiber stays pending until its injected services
  are available, unloads when one disappears and reloads in place when it
  returns.
- **Isolation realms**: `ctx.Isolate("name")` shadows a service name without
  leaking in either direction; shared labels (any comparable value) share
  realms.
- **Events**: emit, parallel, serial, bail and waterfall dispatch, with
  filtered emission and fiber-bound subscriptions.
- **Transactions**: `ctx.Batch` coalesces cascading updates so dependents
  never observe torn intermediate states.
- **Stdlib integration**: every fiber activation owns a `context.Context`
  (`fiber.StdContext()`, `fiber.Done()`) cancelled on unload, restart and
  disposal; the logger service bridges into `log/slog`
  (`cordis.NewSlogHandler`, `Logger.Slog`).
- **Logger**: named, leveled loggers with pluggable exporters and a bounded
  message buffer.

## Example

```go
ctx := cordis.New()

type UserCreated struct{ Name string }

database := cordis.NewPlugin("database", func(ctx *cordis.Context, cfg DatabaseConfig) error {
	_, err := cordis.Provide(ctx, &Database{DSN: cfg.DSN})
	return err
})

users := cordis.NewPlugin("user-service", func(ctx *cordis.Context, _ struct{}) error {
	db := cordis.MustGet[*Database](ctx)
	_, err := cordis.On(ctx, func(e UserCreated) {
		db.Notify(e.Name)
	})
	return err
}).Inject(cordis.ServiceName[*Database]()) // stays pending until the database appears

cordis.Start(ctx, users, struct{}{})
cordis.Start(ctx, database, DatabaseConfig{DSN: "postgres://localhost/app"})
cordis.Emit(ctx, UserCreated{Name: "lars"})
```

Plugin bodies that own goroutines should derive them from the fiber's
stdlib context:

```go
go worker(ctx.Fiber().StdContext()) // cancelled on unload/dispose
```

## Design notes vs the TypeScript original

- Plugin config is statically typed through generics; services and events
  are keyed by type identity (named forms remain for dynamic names and
  realm contracts).
- Microtask scheduling becomes a synchronous drain: transitions settle
  before the outermost framework call returns.
- All shared state is guarded by one mutex and user callbacks always run
  lock-free, so the framework is safe for concurrent use.
- Panics in plugin bodies and cleanups are recovered, logged through the
  framework logger and surface as `StateFailed` fibers, mirroring error
  routing upstream.
- Isolate labels are stored by value in their own table (`map[any]isolateKey`),
  mirroring the realm symbols upstream; no string formatting can make two
  distinct labels collide.

## Test

```sh
go test ./...
```

The cross-language golden scenario (shared with the Rust and Zig ports)
runs as `TestGoldenScenario`; regenerate its expectation with
`GOLDEN_UPDATE=1 go test -run TestGoldenScenario ./...` and re-verify the
other ports afterwards.
