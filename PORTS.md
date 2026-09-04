# Cordis Ports

This directory tree contains the multi-language ports of Cordis. The Go port
is the flagship and the reference implementation for the other languages.

| Language | Directory | Status | Test command |
| -------- | --------- | ------ | ------------ |
| Go | [`go/`](go/) | Core complete: contexts, fibers, effects, events (all five dispatch modes), services with isolation realms, plugin registry with inject reactivity, logger | `cd go && go test ./...` |
| Rust | [`rust/`](rust/) | Foundation: contexts, fibers, effects, events, services, isolation, inject reactivity. Single-threaded by design | `cd rust && cargo test` |
| Zig | [`zig/`](zig/) | Foundation: contexts, fibers, events, services, isolation, inject reactivity | `cd zig && zig build test` |

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
- **One golden scenario, three runners.** `golden/scenario.txt` is executed
  by the Go, Rust and Zig test suites; each must emit the exact trace in
  `golden/expected.txt`, pinning the shared semantics across ports.

See [ROADMAP.md](../ROADMAP.md) for the parity matrix and planned work.
