# Features

Honest inventory of the cordis fork: the TypeScript original in `packages/`
plus the Go (flagship), Rust and Zig ports. Status vocabulary:

- **DONE** — implemented and tested
- **PARTIAL** — implemented, known gaps listed
- **PLANNED** — on the roadmap, not started
- **CONSIDERING** — idea worth evaluating, no commitment

## Core semantics (all three ports)

| Feature | Go | Rust | Zig |
| ------- | -- | ---- | --- |
| Context tree (New/Extend/Isolate/WithFilter) | DONE | DONE | DONE |
| Drain queue (synchronous settling, Batch transactions) | DONE | DONE | partial (no batch) |
| Effect tree: nested, labeled, LIFO rollback, introspection | DONE | DONE | PARTIAL (no labels tree) |
| Events: emit / parallel / serial / bail / waterfall | DONE | DONE | PARTIAL (emit + bail) |
| Event filters + global listeners | DONE | DONE | DONE |
| Fiber states (pending/loading/active/failed/disposed/unloading) | DONE | DONE | DONE |
| Dispose / restart / update | DONE | DONE | DONE |
| Inject reactivity (pending → unload → reload in place) | DONE | DONE | DONE |
| Registry (size / has / delete, snapshot restore) | DONE | DONE | — |
| Config validation | DONE | — | — |
| Fiber Await | DONE | — | — |
| Logger service (levels, exporters, buffer) | DONE | — | — |
| Root fiber restart semantics | DONE | DONE | DONE |
| Deterministic sibling notification order | DONE | DONE | DONE |
| Concurrent access safety | DONE (race tested) | single-threaded | single-threaded |

## Native API layer (phase 2)

| Feature | Go | Rust | Zig |
| ------- | -- | ---- | --- |
| Type-keyed services (primary service API) | DONE (`Provide[T]`/`Get[T]` via `ServiceName[T]`) | DONE (`provide`/`get::<T>()` via `type_name`) | DONE (`provide`/`getTyped` via `@typeName`) |
| Typed events (primary event API) | DONE (`On[E]`/`Once[E]`/`Emit[E]`) | DONE (`on::<E>`/`once::<E>`/`emit::<E>`) | DONE (`onTyped`/`emitTyped`) |
| Named services/events (dynamic names, realm contracts) | DONE (`ProvideNamed`-style method form) | DONE (`*_named`) | DONE (`*Named`) |
| Plugin definition forms | DONE (`NewPlugin[C]` generics) | DONE (`Plugin` trait + associated `Config`; `FnPlugin` closures) | DONE (`TypedPlugin` comptime constructor; runtime `Plugin`) |
| RAII disposal | — (idiomatic Go: explicit Disposers) | DONE (`Guard` with `detach()`) | — (planned) |
| Stdlib context per fiber | DONE (`Fiber.StdContext()`, `Fiber.Done()`) | — | — |
| slog integration | DONE (`NewSlogHandler`, `Logger.Slog`) | — | — |
| Collision-free isolate labels | DONE (`map[any]isolateKey`) | PARTIAL (string labels, `isolate_shared`) | PARTIAL (string labels, `isolateShared`) |
| Domain errors split from allocation errors | n/a | n/a | DONE |

## Cross-language assurance

| Feature | Status |
| ------- | ------ |
| Golden scenario (`golden/`, one spec, three runners) | DONE |
| CI jobs for Go, Rust, Zig (`.github/workflows/ports.yml`) | DONE (workflow verified locally; first green run pending push) |
| `nix flake check` derivations running all three suites | DONE |

## TypeScript original (`packages/`)

Unmodified upstream monorepo: core, loader, hmr, timer, group, logger
packages with their own CI (`.github/workflows/build.yml`).

## Planned

- Go: interception events for loader/hmr ports, service accessor/mixin
  system, callable services, port of `packages/loader`, `packages/hmr`,
  `packages/timer`, `packages/group`
- Rust: thread-safe variant behind a feature flag, true parallel dispatch,
  config validation, batch API
- Zig: registry view, serial/waterfall/parallel dispatch, early disposal
  handles, batch transactions, effect label introspection

See `ROADMAP.md` for the full prioritized list and the parity matrix.
