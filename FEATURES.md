# Features

Honest inventory of the cordis fork: the TypeScript original in `packages/`
plus the Go (flagship), Rust and Zig ports. Status vocabulary:

- **FULLY_FUNCTIONAL** — code present and working (suite green at time of writing)
- **PARTIALLY_FUNCTIONAL** — ships, but with known gaps (listed)
- **BROKEN** — code exists but does not work
- **PLANNED** — no code yet

## Core semantics

| Feature                                                         | Go                             | Rust                                                | Zig                       |
| --------------------------------------------------------------- | ------------------------------ | --------------------------------------------------- | ------------------------- |
| Context tree (New/Extend/Isolate/Intercept/WithFilter)          | FULLY_FUNCTIONAL               | FULLY_FUNCTIONAL                                    | FULLY_FUNCTIONAL          |
| Drain queue (synchronous settling)                              | FULLY_FUNCTIONAL               | FULLY_FUNCTIONAL                                    | FULLY_FUNCTIONAL          |
| Batch transactions                                              | FULLY_FUNCTIONAL               | FULLY_FUNCTIONAL                                    | FULLY_FUNCTIONAL          |
| Effect tree: nested, labeled, LIFO rollback, introspection      | FULLY_FUNCTIONAL               | FULLY_FUNCTIONAL                                    | FULLY_FUNCTIONAL          |
| Events: emit / parallel / serial / bail / waterfall             | FULLY_FUNCTIONAL               | FULLY_FUNCTIONAL                                    | FULLY_FUNCTIONAL          |
| Event filters + global listeners                                | FULLY_FUNCTIONAL               | FULLY_FUNCTIONAL                                    | FULLY_FUNCTIONAL          |
| Fiber states (pending/loading/active/failed/disposed/unloading) | FULLY_FUNCTIONAL               | FULLY_FUNCTIONAL                                    | FULLY_FUNCTIONAL          |
| Dispose / restart / update                                      | FULLY_FUNCTIONAL               | FULLY_FUNCTIONAL                                    | FULLY_FUNCTIONAL          |
| Inject reactivity (pending → unload → reload in place)          | FULLY_FUNCTIONAL               | FULLY_FUNCTIONAL                                    | FULLY_FUNCTIONAL          |
| Interception events (`internal/get\|set\|listener\|dispatch\|plugin\|update`) | FULLY_FUNCTIONAL | PARTIALLY_FUNCTIONAL (`plugin`, `update`; `get\|set\|listener\|dispatch` planned) | PLANNED                   |
| Status events (`internal/status`)                               | FULLY_FUNCTIONAL               | FULLY_FUNCTIONAL                                    | PLANNED                   |
| Registry view (size / has / delete)                             | FULLY_FUNCTIONAL               | FULLY_FUNCTIONAL                                    | FULLY_FUNCTIONAL          |
| Registry snapshot / restore                                     | FULLY_FUNCTIONAL               | FULLY_FUNCTIONAL                                    | PLANNED                   |
| Config validation                                               | FULLY_FUNCTIONAL               | FULLY_FUNCTIONAL                                    | PLANNED                   |
| Fiber Await (Go: plus stdlib-context variant)                   | FULLY_FUNCTIONAL               | n/a (drain settles synchronously)                   | —                         |
| Logger service (levels, exporters, buffer)                      | FULLY_FUNCTIONAL               | PLANNED                                             | PLANNED                   |
| Root fiber restart semantics                                    | FULLY_FUNCTIONAL               | FULLY_FUNCTIONAL                                    | FULLY_FUNCTIONAL          |
| Deterministic sibling notification order                        | FULLY_FUNCTIONAL               | FULLY_FUNCTIONAL                                    | FULLY_FUNCTIONAL          |
| Concurrent access safety                                        | FULLY_FUNCTIONAL (race-tested) | FULLY_FUNCTIONAL via the opt-in `thread-safe` build | Single-threaded by design |

## Native API layer (phase 2)

| Feature                                                | Go                                             | Rust                                                 | Zig                                                         |
| ------------------------------------------------------ | ---------------------------------------------- | ---------------------------------------------------- | ----------------------------------------------------------- |
| Type-keyed services (primary service API)              | FULLY_FUNCTIONAL (`Provide[T]`/`Get[T]`)       | FULLY_FUNCTIONAL (`provide`/`get::<T>()`)            | FULLY_FUNCTIONAL (`provide`/`getTyped`)                     |
| Typed events (primary event API)                       | FULLY_FUNCTIONAL (`On[E]`/`Once[E]`/`Emit[E]`) | FULLY_FUNCTIONAL (`on::<E>`/`once::<E>`/`emit::<E>`) | FULLY_FUNCTIONAL (`onTyped`/`onceTyped`/`emitTyped`)        |
| Named services/events (dynamic names, realm contracts) | FULLY_FUNCTIONAL (`*Named`, `Context.*`)       | FULLY_FUNCTIONAL (`*_named`)                         | FULLY_FUNCTIONAL (`*Named`)                                 |
| Plugin definition forms                                | FULLY_FUNCTIONAL (`NewPlugin[C]` generics)     | FULLY_FUNCTIONAL (`Plugin` trait; `FnPlugin`)        | FULLY_FUNCTIONAL (`TypedPlugin` comptime; runtime `Plugin`) |
| RAII disposal                                          | — (idiomatic Go: explicit Disposers)           | FULLY_FUNCTIONAL (`Guard` with `detach()`)           | PLANNED                                                     |
| Stdlib context per fiber                               | FULLY_FUNCTIONAL (`StdContext`/`Done`)         | —                                                    | —                                                           |
| slog integration                                       | FULLY_FUNCTIONAL (`NewSlogHandler`)            | —                                                    | —                                                           |
| Collision-free isolate labels                          | FULLY_FUNCTIONAL (`map[any]isolateKey`)        | FULLY_FUNCTIONAL (`(name, label)` hash map)          | FULLY_FUNCTIONAL (content-hashed pair keys)                 |
| Domain errors split from allocation errors             | n/a                                            | n/a                                                  | FULLY_FUNCTIONAL (OOM panics, std style)                    |

## Go ecosystem packages

| Feature                                                              | Status           | Evidence                      |
| -------------------------------------------------------------------- | ---------------- | ----------------------------- |
| `timer`: AfterFunc/Await/Interval/IntervalFunc/Throttle/Debounce     | FULLY_FUNCTIONAL | `go/timer/` (synctest-tested) |
| `group`: id-keyed child fibers, diffed Update, rollback              | FULLY_FUNCTIONAL | `go/group/`                   |
| `loader`: resolver, entries, groups, tree, JSON config, watch/reload | FULLY_FUNCTIONAL | `go/loader/`                  |
| `hmr`: module generations, declare graph, swap, rollback             | FULLY_FUNCTIONAL | `go/hmr/`                     |
| Accessor / mixin derived services with write-back                    | FULLY_FUNCTIONAL | `go/accessor.go`              |
| Callable services + tracker (`ProvideService`, `Callable`)           | FULLY_FUNCTIONAL | `go/callable.go`              |
| API polish: `Fiber.Err`, `AwaitContext`, `Inject1/2/3`               | FULLY_FUNCTIONAL | `go/fiber.go`, `go/inject.go` |
| Benchmarks (start/dispose, provide+get, emit, waterfall)             | FULLY_FUNCTIONAL | `go/bench_test.go`            |
| Benchmarks (same six hot paths, release profile)                     | FULLY_FUNCTIONAL | `rust/benches/core.rs`        |

## Cross-language assurance

| Feature                                                      | Status                                                                                                      |
| ------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------- |
| Golden scenarios, byte-identical across Go/Rust/Zig          | FULLY_FUNCTIONAL (4: lifecycle, events, cascade, dispatch — `golden/`)                                      |
| DSL parser unit tests in all three runners                   | FULLY_FUNCTIONAL                                                                                            |
| Ports CI (`ports.yml`: race tests, clippy, leak-checked Zig) | FULLY_FUNCTIONAL (green runs recorded in CHANGELOG history)                                                 |
| `nix flake check` derivations for all three suites           | FULLY_FUNCTIONAL                                                                                            |
| TypeScript suite (`packages/`)                               | FULLY_FUNCTIONAL locally: 248/248 after the 2026-09-08 rebase repair; CI verification pending the next push |

## TypeScript original (`packages/`)

Tracks `upstream/main` (`caab04e`, rebased 2026-09-08). Inherited upstream
features: three-stage reload (#111), include journal reconciliation (#121),
bare-specifier resolution (#123), `hmr.watch()` (#128), plus the
`3-stage-hmr` line replayed on top (`b4650df`: commit-based loader entry
changes, atomic include writes).

## Planned

No code yet; bounded work is tracked in `TODO_LIST.md`, direction in
`ROADMAP.md`:

- Zig: registry snapshot/restore, status events, accessor/mixin, logger
- Rust: `internal/get|set|listener|dispatch` interception, logger service
- Golden scenario #4 (dispatch-mode parity)

See `ROADMAP.md` for the full parity matrix and the documented native-max
divergences.
