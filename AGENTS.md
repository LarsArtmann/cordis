# AGENTS.md — cordis (multi-language fork)

This fork of [cordiverse/cordis](https://github.com/cordiverse/cordis) adds
Go, Rust and Zig ports next to the TypeScript original in `packages/`.
**Go is the flagship port** and the reference implementation for the others.

## Layout

- `packages/` — TypeScript original (yarn workspaces, vitest).
- `go/` — Go module `github.com/LarsArtmann/cordis/go`, package `cordis`.
- `rust/` — Cargo crate `cordis` (single-threaded, `Rc`/`RefCell`).
- `zig/` — Zig module (0.16), arena-based memory, tested via build.zig.
- `PORTS.md` — shared port architecture. `ROADMAP.md` — parity matrix.

## Build and test

Use the flake (`nix run .#test`, `.#test-go`, `.#test-rust`, `.#test-zig`)
or run directly:

- Go: `cd go && go test ./...` (also: `go vet`, `-race` clean, ~85% coverage)
- Rust: `cd rust && cargo test` (clippy clean, `cargo clippy --all-targets`)
- Zig: `cd zig && zig build test` (leak-checked via testing.allocator)

### Environment gotcha (this machine)

`GOCACHE` defaults to `/mnt/buildcache/go-build` and `~/.cache/go-build` is a
dangling home-manager symlink. Both are broken. Always run Go with
`export GOCACHE=/tmp/gocache` (the flake apps do this automatically).
Zig is not installed globally; use `nix run nixpkgs#zig -- ...` or the flake
devShell.

## Port architecture (all three languages share this)

**Prime directive (user, 2026-08-22): use each language's native features to
the max. Do NOT port TypeScript 1:1.** Semantics parity (fiber lifecycle,
drain queue, realms, rollback) is the constraint; API surfaces must be
native: type-keyed services and typed events (Go generics / Rust TypeId /
Zig comptime), stdlib `context.Context` per fiber in Go, slog integration,
RAII disposers in Rust, comptime plugin construction in Zig. Where a native
design intentionally diverges from TS behavior, document the divergence in
ROADMAP.md.

1. **Drain queue instead of microtasks.** Public API calls are wrapped in
   `core.enter()`/`core.leave()`. Fiber state transitions are queued and
   coalesced, then executed when the outermost call returns. This replaces
   the TS microtask queue and makes torn intermediate states unobservable.
   `Context.Batch` (Go) groups multiple mutations into one transaction.
2. **No locks/borrows during user callbacks.** All framework state lives in
   one `core` guarded by a single mutex (Go) or RefCell (Rust). Listeners,
   plugin bodies and cleanups always run unlocked; they snapshot state under
   the lock first. Never hold core.mu while calling user code in Go.
3. **Effect tree.** Every registration (listener, service, nested plugin,
   raw cleanup) is a labeled item in a dispose bag. Effect bodies collect
   into a child bag via `ctx.collect` (Go: goroutine-safe by passing a
   derived context, not fiber-global state). Disposal is LIFO everywhere.
4. **Realm keyed services.** Services are stored by a uint64 realm key, not
   by name. `Isolate` maps a name to a fresh key in the child scope; lookups
   walk the parent chain and fall back to the root realm (lazily assigned).
   Shared labels map name+label to one synthetic key.
5. **Fiber state machine.** pending/loading/active/failed/disposed/
   unloading, driven by `transition()` from the drain queue. Dependency
   resolution snapshots a generation counter and retries when the store
   changed mid-resolution (Go).
6. **Plugin identity.** Go: the `*Plugin[C]` value (typed config via
   generics). Rust: process-global atomic id. Zig: the plugin's address.

## Semantics deliberately adapted from TS

- Go drains synchronously: after `Start()` returns, an appliable plugin has
  already run. TS runs it on a microtask.
- Cleanups are synchronous `func()`. Async cleanup is the user's goroutine
  concern.
- `Emit` panics propagate (Go); `Parallel` joins errors (Go errors.Join).
- Plugin apply errors move the fiber to `StateFailed`, roll back partial
  effects and are routed to the logger, never thrown across the framework
  boundary.
