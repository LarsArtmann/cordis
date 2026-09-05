# Roadmap

Status of the multi-language ports relative to the TypeScript reference
implementation (`packages/core`). Go is the flagship; Rust and Zig follow
its architecture.

## Parity matrix

| Feature                                                | Go                 | Rust                              | Zig                                    |
| ------------------------------------------------------ | ------------------ | --------------------------------- | -------------------------------------- |
| Context tree (extend)                                  | DONE               | DONE                              | DONE                                   |
| Isolation realms + shared labels                       | DONE               | DONE                              | DONE                                   |
| Intercept (per-scope service config)                   | DONE               | DONE                              | -                                      |
| Effects: nested, labeled, LIFO rollback, introspection | DONE               | DONE                              | partial (no labels tree introspection) |
| Events: emit / parallel / serial / bail / waterfall    | DONE               | DONE                              | DONE                                   |
| Event filters + global listeners                       | DONE               | DONE                              | DONE                                   |
| Fiber states, dispose, restart, update                 | DONE               | DONE                              | DONE                                   |
| Inject reactivity (pending / unload / reload in place) | DONE               | DONE                              | DONE                                   |
| Registry (size / has / delete, snapshot restore)       | DONE               | DONE                              | -                                      |
| Status events (internal/status)                        | DONE               | DONE                              | -                                      |
| Config validation                                      | DONE               | DONE                              | -                                      |
| Fiber Await (+ stdlib-context variant in Go)           | DONE               | n/a (drain settles synchronously) | -                                      |
| Batch transactions                                     | DONE               | DONE                              | DONE                                   |
| Logger service (levels, exporters, buffer)             | DONE               | -                                 | -                                      |
| Accessor / mixin derived services                      | DONE               | -                                 | -                                      |
| Callable services + tracker                            | DONE               | -                                 | -                                      |
| Timer (interval, debounce, throttle)                   | DONE               | -                                 | -                                      |
| Loader (config entries, watch/reload)                  | DONE               | -                                 | -                                      |
| HMR (implementation swap, rollback)                    | DONE               | -                                 | -                                      |
| Concurrent access safety                               | DONE (race tested) | thread-safe build (Mutex)         | single-threaded                        |

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

- Typed services and events derive their names from _type identity_
  (reflect string / `type_name` / `@typeName`). Two distinct types with
  identical derived names (structurally identical anonymous types, or
  Rust's best-effort `type_name`) would share a slot; prefer named types.
- Rust's `Plugin` trait registry identity is per _type_
  (`plugin_type_id::<P>()`), while `FnPlugin` values are per
  _construction_ — both match "one plugin definition, one runtime".
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

Landed since the foundation: registry view, serial / waterfall / parallel
dispatch modes, batch coalescing, effect labels with introspection, typed
events, shared isolation labels.

1. Snapshot/restore and status events (Rust parity).
2. Accessor/mixin derived services (Go parity).
3. Disposer-returning `on` / `provide` for early disposal.
4. Loader / hmr equivalents — pending the module-layout decision.

### Repo

1. First green `ports.yml` CI run (actions verified; requires a push).
2. More golden scenarios covering events and the logger.

### Release cadence

- **Go** (`github.com/LarsArtmann/cordis/go`): tagged `go/v0.1.x` per
  feature milestone; patch bumps for fixes. Module consumers upgrade via
  `go get github.com/LarsArtmann/cordis/go@latest`.
- **Rust** (`cordis` crate): `v0.1.x` during the foundation phase, `v0.2.0`
  when the registry snapshot/restore and status events landed.
- **Zig**: tagged together with the repo when the foundation phase
  completes (0.16 build-zig test runner).
- Cadence: cut a release whenever a milestone (M-task) lands and CI is
  green; do not batch unrelated changes into one release.

### Kernovia convergence (2026-09-04)

Kernovia (`github.com/larsartmann/kernovia`) — the compiled, statically
typed micro-kernel that proves this calculus in production shape — now
treats this fork as its **executable semantic oracle**: `go/v0.1.0` is
pinned, and its golden scenarios run against Kernovia's reactive stack
byte-exact (`pkg/testing/cordisparity` in the Kernovia repo, ADR-004).
Scenario v1 (27 ops incl. realms skipped-with-attribution) and
scenario-cascade (nested plugins, delete, registry-size) are green there;
scenario-events is blocked on Kernovia's realm + dispatch work.

Bidirectional feedback started: #1 (lock-free-callback regression test) and
#2 (RequireNoResidue helper) were filed from Kernovia's port experience.
Ongoing interest: Kernovia's loader/hmr are production-proven candidates for
this repo's "Loader / hmr equivalents" gap — the adoption decision (Go/No-Go
with gates) is recorded in Kernovia's ADR-004 appendix. When evaluating the
module-layout decision, factor in a consumer that exercises the calculus
daily.
