# Cordis Ports

This directory tree contains the multi-language ports of Cordis. The Go port
is the flagship and the reference implementation for the other languages.

| Language | Directory        | Status                                                                                                                                                                                                                                                                                    | Test command               |
| -------- | ---------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------- |
| Go       | [`go/`](go/)     | Core complete: contexts, fibers, effects, events (all five dispatch modes), services with isolation realms, plugin registry with inject reactivity, logger, timer, group, loader (watch/reload), hmr (swap + rollback), accessor/mixin, callable services + tracker, typed inject helpers | `cd go && go test ./...`   |
| Rust     | [`rust/`](rust/) | Core complete (single-threaded core plus a `thread-safe` Mutex build): contexts, fibers, effects, events, services, isolation, inject reactivity, validation, intercept, snapshot/restore, status events                                                                                  | `cd rust && cargo test`    |
| Zig      | [`zig/`](zig/)   | Core complete: contexts, fibers, effects with introspection, events (all five dispatch modes), batch, services with isolation realms, registry view, disposers, comptime typed APIs (`TypedPlugin`)                                                                                       | `cd zig && zig build test` |

All three ports share one architecture:

- **Drain queue instead of microtasks.** Fiber state transitions are
  coalesced and settle before the outermost framework call returns, so torn
  intermediate states are unobservable. `Batch` (Go) / `batch` (Rust) groups
  multiple mutations into one transaction.
- **Lock-free user callbacks.** Listeners, plugin bodies and cleanups never
  run while framework-internal state is locked or borrowed, so user code may
  freely re-enter the framework.
- **Realm keyed services.** `isolate` creates service realms backed by
  per-realm keys, the counterpart of the realm symbols upstream.
- **Native-max API layer.** The primary service and event APIs are keyed by
  type identity (Go `reflect` + generics, Rust `type_name`, Zig
  `@typeName`); named/string forms remain for dynamic names and the
  `internal/` event namespace. Plugin definitions use each language's
  native form: Go generics (`NewPlugin[C]`), a Rust trait with an
  associated `Config` (plus `FnPlugin` closures), a Zig comptime
  constructor (`TypedPlugin`). See `ROADMAP.md` for the documented
  divergences.
- **Six golden scenarios.** `golden/scenario.txt`, `scenario-events.txt`,
  `scenario-cascade.txt`, `scenario-dispatch.txt`, `scenario-snapshot.txt`
  and `scenario-logger.txt` are executed by the runners in the
  per-scenario matrix (`golden/README.md`): #1–#4 and #6 in all three
  ports, #5 (registry snapshot/restore) in Rust and Zig — the Go port has
  no snapshot API yet.
  Each runner must emit the exact traces in `golden/expected*.txt`,
  pinning the shared semantics across ports. The Go loader additionally
  pins its watch/reload lifecycle in a Go-only golden transcript
  (`go/loader/testdata/watch-golden.txt`).
- **Shared registration builder.** Go's `Resolver.RegisterType[C]` and
  `ReplaceType[C]` (and hmr's `SwapType`) compose on one
  `TypedRegistration` builder, so every typed registration path shares
  the same decode/validation plumbing.

## Fallibility shapes

Each port surfaces domain errors in its native form. The shapes below are
the shared contract (verified 2026-11-14); the typed-event dispatch guards
panic in every port by the same reviewed contract — listener callbacks
have no error channel, and a wrong emission shape is a caller programming
error (`scripts/panic-allowlist.sh` is the machine gate; ROADMAP.md holds
the decision record).

- **Go** — fallible APIs return `(T, error)`; sentinel `*Error` codes wrap
  with `%w`; the typed guards in `go/typed.go` panic (6 reviewed sites).
- **Rust** — fallible APIs return `Result<T, Error>` (`crate::Result`);
  the typed guards in `rust/src/events.rs` panic (2 reviewed sites).
- **Zig** — fallible APIs return `Error!T` with
  `Error{InactiveEffect, DuplicateService, PluginFailed, OutOfMemory}`;
  allocation failures thread `error.OutOfMemory` through every
  registration path (`zig/tests/oom.zig` sweeps each registration
  allocation index), while the 17 dispatch/void-query sites abort with
  the greppable `cordis: out of memory in dispatch`.

See [ROADMAP.md](ROADMAP.md) for the parity matrix and planned work.
