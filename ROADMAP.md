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

### Phase 2: native-max API redesign (user directive, 2026-08-22) — LANDED 2026-09-04

**Use each language's native features to the max; do not port TypeScript
1:1.** Semantics parity stays (fiber lifecycle, drain queue, realms,
rollback); API surfaces become native. Detailed tasks live in
`TODO_LIST.md`; intentionally divergent designs are documented here as
they land.

Landed native APIs:

- **Go**: type-keyed services (`Provide[T]`/`Get[T]`/`TryGet[T]` with
  `ServiceName[T]`), typed events (`On[E]`/`Once[E]`/`Emit[E]` with
  `EventName[E]`), stdlib `context.Context` per fiber
  (`Fiber.StdContext()`/`Fiber.Done()`), slog bridge
  (`NewSlogHandler`, `Logger.Slog`), collision-free isolate labels
  (`map[any]isolateKey`).
- **Rust**: typed services (`provide`/`get::<T>()`/`try_get` keyed by
  `type_name`), typed events (`on::<E>`/`once::<E>`/`emit::<E>`), RAII
  `Guard` (dispose on drop, `detach()` to opt out), `Plugin` trait with
  associated `Config` (closure form preserved as `FnPlugin`/`start_fn`).
- **Zig**: comptime plugin construction (`TypedPlugin(name, Config, apply,
  inject)` — the type is the registry identity), typed services/events
  keyed by `@typeName`, plain cleanup attachment (`Context.attach`),
  domain errors split from allocation failures (OOM panics, std style).

Divergences from TS behavior, by design:

- Typed services and events derive their names from *type identity*
  (reflect string / `type_name` / `@typeName`). Two distinct types with
  identical derived names (structurally identical anonymous types, or
  Rust's best-effort `type_name`) would share a slot; prefer named types.
- Rust's `Plugin` trait registry identity is per *type*
  (`plugin_type_id::<P>()`), while `FnPlugin` values are per
  *construction* — both match "one plugin definition, one runtime".
- Zig's `TypedPlugin` registry identity is the address of the comptime
  view embedded in the returned type.
- String event names are not restricted in code, but the convention is
  that only the framework's `internal/` namespace uses them; application
  events should be typed.
- Sibling notification order is deterministic (creation order) in all
  three ports, matching the insertion-ordered maps upstream; Go sorts by
  fiber uid where its map iteration would otherwise be random.

Cross-language assurance: one golden scenario (`golden/`) executed by the
Go, Rust and Zig test suites with a byte-identical expected trace, plus
`nix flake check` derivations running all three suites.

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

1. First green `ports.yml` CI run (actions verified; requires a push).
2. More golden scenarios covering events and the logger.
