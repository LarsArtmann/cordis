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
- **Inject reactivity**: a fiber stays pending until its injected services
  are available, unloads when one disappears and reloads in place when it
  returns.
- **Isolation realms**: `ctx.Isolate("name")` shadows a service name without
  leaking in either direction; shared labels share realms.
- **Events**: emit, parallel, serial, bail and waterfall dispatch, with
  filtered emission and fiber-bound subscriptions.
- **Transactions**: `ctx.Batch` coalesces cascading updates so dependents
  never observe torn intermediate states.
- **Logger**: named, leveled loggers with pluggable exporters and a bounded
  message buffer.

## Example

```go
ctx := cordis.New()

database := cordis.NewPlugin("database", func(ctx *cordis.Context, cfg DatabaseConfig) error {
	_, err := ctx.Provide("database", &Database{DSN: cfg.DSN})
	return err
})

users := cordis.NewPlugin("user-service", func(ctx *cordis.Context, _ struct{}) error {
	db := cordis.MustGet[*Database](ctx, "database")
	_, err := ctx.On("user-created", func(args ...any) any {
		db.Notify(args[0].(string))
		return nil
	})
	return err
}).Inject("database") // stays pending until "database" is provided

cordis.Start(ctx, users, struct{}{})
cordis.Start(ctx, database, DatabaseConfig{DSN: "postgres://localhost/app"})
ctx.Emit("user-created", "lars")
```

## Design notes vs the TypeScript original

- Plugin config is statically typed through generics.
- Microtask scheduling becomes a synchronous drain: transitions settle
  before the outermost framework call returns.
- All shared state is guarded by one mutex and user callbacks always run
  lock-free, so the framework is safe for concurrent use.
- Panics in plugin bodies and cleanups are recovered, logged through the
  framework logger and surface as `StateFailed` fibers, mirroring error
  routing upstream.

## Test

```sh
go test ./...
```
