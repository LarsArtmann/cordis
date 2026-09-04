# Roadmap

Status of the multi-language ports relative to the TypeScript reference
implementation (`packages/core`). Go is the flagship; Rust and Zig follow
its architecture.

## Parity matrix

| Feature | Go | Rust | Zig |
| ------- | -- | ---- | --- |
| Context tree (extend) | DONE | DONE | DONE |
| Isolation realms + shared labels | DONE | DONE | DONE |
| Intercept (per-scope service config) | DONE | - | - |
| Effects: nested, labeled, LIFO rollback, introspection | DONE | DONE | partial (no labels tree introspection) |
| Events: emit / parallel / serial / bail / waterfall | DONE | DONE | emit + bail |
| Event filters + global listeners | DONE | DONE | DONE |
| Fiber states, dispose, restart, update | DONE | DONE | DONE |
| Inject reactivity (pending / unload / reload in place) | DONE | DONE | DONE |
| Registry (size / has / delete, snapshot restore) | DONE | DONE | - |
| Config validation | DONE | - | - |
| Fiber Await | DONE | - | - |
| Batch transactions | DONE | DONE | - |
| Logger service (levels, exporters, buffer) | DONE | - | - |
| Concurrent access safety | DONE (race tested) | single-threaded | single-threaded |

## Planned, in priority order

### Phase 2: native-max API redesign (user directive, 2026-08-22)

**Use each language's native features to the max; do not port TypeScript
1:1.** Semantics parity stays (fiber lifecycle, drain queue, realms,
rollback); API surfaces become native. Detailed tasks live in
`TODO_LIST.md`; intentionally divergent designs must be documented here as
they land.

- Go: type-keyed services, typed events, stdlib `context.Context` per
  fiber, `slog` integration
- Rust: TypeId-keyed services and events, RAII disposers, `Plugin` trait
- Zig: comptime plugin construction, type-keyed services/events

### Go

1. `internal/listener`, `internal/dispatch`, `internal/get`, `internal/set`
   interception events (needed by loader and hmr ports).
2. Service accessor/mixin system (Property.Accessor upstream).
3. Callable services and tracker based effect attribution
   (Service.tracker upstream): effects created through a service are
   attributed to the calling fiber.
4. Port `packages/loader` (config file driven plugin management).
5. Port `packages/hmr` (hot module replacement).
6. Port `packages/timer` and `packages/group` conveniences.
7. slog.Handler adapter bridging the logger service into log/slog.

### Rust

1. Thread-safe variant (`Arc`/`Mutex` core) behind a feature flag, keeping
   the single-threaded crate as the default.
2. True parallel dispatch with scoped threads.
3. Effect introspection parity (`EffectMeta` trees are implemented; expose
   nested labels on more registration kinds).
4. Config validation + `update` with typed configs.
5. Batch API (currently transitions drain per call; add `Context::batch`).

### Zig

1. Registry view (size / delete) on top of the runtime map.
2. serial / waterfall / parallel dispatch modes.
3. Early disposal handles (Disposer) for `on` and `provide`.
4. Batch transactions.
5. Effect labels exposed for introspection.

### Repo

1. CI jobs for Go, Rust and Zig in `.github/workflows`.
2. Cross-language golden tests: one shared scenario file executed by every
   port.
