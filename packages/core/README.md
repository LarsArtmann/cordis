<div align="center">

# Cordis

**A meta-framework of spatiotemporal composability, beyond TypeScript.**

[![Ports CI](https://github.com/LarsArtmann/cordis/actions/workflows/ports.yml/badge.svg)](https://github.com/LarsArtmann/cordis/actions/workflows/ports.yml)
[![Go 1.26+](https://img.shields.io/badge/go-1.26%2B-00ADD8.svg)](go/go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-yellow.svg)](LICENSE)

**Go** (flagship) · **Rust** · **Zig** · **TypeScript** (original)

</div>

## What is Cordis?

Cordis organizes an application as a tree of **contexts**: hierarchical scopes
that carry services, event listeners and effects. Plugins run inside
**fibers**, effect scopes with a lifecycle. When a fiber stops, everything it
registered (services, listeners, timers, nested plugins, cleanups) rolls back
automatically in reverse order. No manual teardown, ever.

Fibers declare service dependencies through **inject**. The framework
activates a fiber when its dependencies appear, unloads it when one
disappears, and reloads it in place when it returns. The set of running code
is a pure function of the set of available services: that is the temporal
dimension of Cordis.

This fork ports the Cordis core from its TypeScript origin
([cordiverse/cordis](https://github.com/cordiverse/cordis)) to compiled,
memory-safe languages. **Go is the flagship port** and the reference
implementation; Rust and Zig follow its architecture. The TypeScript original
remains intact in [`packages/`](packages/).

| Language | Directory | Status | Test command |
| -------- | --------- | ------ | ------------ |
| Go | [`go/`](go/) | Core complete: full lifecycle, all five event dispatch modes, isolation realms, registry, logger; race-tested, ~85% coverage | `cd go && go test ./...` |
| Rust | [`rust/`](rust/) | Foundation: contexts, fibers, effects, events, services, isolation, inject reactivity | `cd rust && cargo test` |
| Zig | [`zig/`](zig/) | Foundation: contexts, fibers, events, services, isolation, inject reactivity | `cd zig && zig build test` |
| TypeScript | [`packages/`](packages/) | Reference implementation | `yarn test` (see [core](packages/core/README.md)) |

See [ROADMAP.md](ROADMAP.md) for the full parity matrix.

## The Go port in 30 seconds

```go
package main

import (
	"fmt"

	cordis "github.com/LarsArtmann/cordis/go"
)

type DatabaseConfig struct{ DSN string }

type Database struct{ DSN string }

func (db *Database) Query(q string) string { return "rows for " + q + " via " + db.DSN }

var DatabasePlugin = cordis.NewPlugin("database", func(ctx *cordis.Context, cfg DatabaseConfig) error {
	_, err := ctx.Provide("database", &Database{DSN: cfg.DSN})
	return err
})

var UserServicePlugin = cordis.NewPlugin("user-service", func(ctx *cordis.Context, _ struct{}) error {
	db := cordis.MustGet[*Database](ctx, "database")
	_, err := ctx.On("user-created", func(args ...any) any {
		fmt.Println(db.Query("SELECT * FROM users"))
		return nil
	})
	return err
}).Inject("database") // stays pending until "database" is provided

func main() {
	ctx := cordis.New()

	// Starts pending: the database service does not exist yet.
	if _, err := cordis.Start(ctx, UserServicePlugin, struct{}{}); err != nil {
		panic(err)
	}

	// Providing the database activates the user service in the same call.
	if _, err := cordis.Start(ctx, DatabasePlugin, DatabaseConfig{DSN: "postgres://localhost/app"}); err != nil {
		panic(err)
	}

	ctx.Emit("user-created")
	// Output: rows for SELECT * FROM users via postgres://localhost/app
}
```

The user service was started first, yet it runs correctly: inject resolved
the ordering for you. Dispose the database fiber and the user service unloads
and reloads when it comes back, with listeners and services rolled back in
between. Plugin configuration is statically typed through generics; invalid
config is a compile error, not a runtime surprise.

## Shared architecture

All three ports implement the same invariants:

- **Drain queue instead of microtasks.** Fiber state transitions are queued,
  coalesced and executed before the outermost framework call returns. Torn
  intermediate states are unobservable. `Batch` (Go) / `batch` (Rust) groups
  multiple mutations into one transaction.
- **Lock-free user callbacks.** Listeners, plugin bodies and cleanups never
  run while framework state is locked or borrowed, so user code may freely
  re-enter the framework from any goroutine.
- **Realm-keyed services.** `Isolate` shadows a service behind a fresh realm
  key without leaking in either direction; shared labels opt into sharing.

Details: [PORTS.md](PORTS.md).

## Get started

**Go** (flagship, module `github.com/LarsArtmann/cordis/go`):

```sh
go get github.com/LarsArtmann/cordis/go
```

```go
import cordis "github.com/LarsArtmann/cordis/go"
```

Guide: [go/README.md](go/README.md).

**Rust** (crate `cordis`, single-threaded by design):

```sh
cargo add --git https://github.com/LarsArtmann/cordis cordis
```

**Zig** (0.16, arena-based, no package registry yet): vendor [`zig/`](zig/)
into your project or add it as a git submodule.

All three suites run through the Nix flake: `nix run .#test` (or
`.#test-go`, `.#test-rust`, `.#test-zig`).

## Documentation

- [PORTS.md](PORTS.md) - shared port architecture and per-language status
- [ROADMAP.md](ROADMAP.md) - parity matrix and planned work
- [go/README.md](go/README.md) - Go port guide and design notes
- [Cordis primer](https://deepseek-harness.github.io/deepseek-harness/reference/cordis-primer) - framework documentation (TypeScript)
- Paper: _A Programming Paradigm for Spatiotemporal Composability_
  ([arXiv](https://arxiv.org/abs/2608.25512), [repository](https://github.com/cordiverse/paper))

## Status

Cordis is under active development. The API is not yet stable and may change
without notice. The Go port covers the full core feature matrix (see
[ROADMAP.md](ROADMAP.md)); Rust and Zig implement the foundation with the
remaining features prioritized there.

CI ([ports.yml](.github/workflows/ports.yml)) runs `go vet` plus race-enabled
tests, clippy with warnings denied, and leak-checked Zig tests on every push
and pull request.

## Relationship to upstream

This repository is a fork of [cordiverse/cordis](https://github.com/cordiverse/cordis)
that tracks `main` upstream. The TypeScript packages stay intact and keep
building; the ports live alongside them in `go/`, `rust/` and `zig/`.

## License

[MIT](LICENSE) © Shigma and contributors. The port code carries the same
license.
