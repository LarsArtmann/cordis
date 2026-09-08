# Roadmap

Status of the multi-language ports relative to the TypeScript reference
implementation (`packages/core`). Go is the flagship; Rust and Zig follow
its architecture.

## Parity matrix

| Feature                                                     | Go                 | Rust                              | Zig                     |
| ----------------------------------------------------------- | ------------------ | --------------------------------- | ----------------------- |
| Context tree (extend)                                       | DONE               | DONE                              | DONE                    |
| Isolation realms + shared labels                            | DONE               | DONE                              | DONE                    |
| Intercept (per-scope service config)                        | DONE               | DONE                              | -                       |
| Effects: nested, labeled, LIFO rollback, introspection      | DONE               | DONE                              | DONE                    |
| Events: emit / parallel / serial / bail / waterfall         | DONE               | DONE                              | DONE                    |
| Event filters + global listeners                            | DONE               | DONE                              | DONE                    |
| Fiber states, dispose, restart, update                      | DONE               | DONE                              | DONE                    |
| Inject reactivity (pending / unload / reload in place)      | DONE               | DONE                              | DONE                    |
| Registry (size / has / delete, snapshot restore)            | DONE               | DONE                              | view only (no snapshot) |
| Interception events (internal/get\|set\|listener\|dispatch) | DONE               | -                                 | -                       |
| Status events (internal/status)                             | DONE               | DONE                              | -                       |
| Config validation                                           | DONE               | DONE                              | -                       |
| Fiber Await (+ stdlib-context variant in Go)                | DONE               | n/a (drain settles synchronously) | -                       |
| Batch transactions                                          | DONE               | DONE                              | DONE                    |
| Logger service (levels, exporters, buffer)                  | DONE               | -                                 | -                       |
| Accessor / mixin derived services                           | DONE               | -                                 | -                       |
| Callable services + tracker                                 | DONE               | -                                 | -                       |
| Timer (interval, debounce, throttle)                        | DONE               | -                                 | -                       |
| Loader (config entries, watch/reload)                       | DONE               | -                                 | -                       |
| HMR (implementation swap, rollback)                         | DONE               | -                                 | -                       |
| Concurrent access safety                                    | DONE (race tested) | thread-safe build (Mutex)         | single-threaded         |

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

1. Parity reassessment after the 2026-09-08 upstream sync: decide how the
   three-stage reload (#111), include journal (#121), bare-specifier
   resolution (#123), `hmr.watch()` (#128) and the replayed `3-stage-hmr`
   loader semantics map onto `go/loader`/`go/hmr` — port, or document the
   divergence here.
2. Native API phase 3 (idea): generic methods (`ctx.Get[T]()`) are feasible
   under Go 1.27 (research-verified 2026-09-08, including the interface
   limitation) but would fragment the surface against the named
   `Context` methods; free-function deprecation timeline undecided — see
   Open decisions.
3. Logger golden scenario (the logger service has no golden coverage).

### Rust

1. `internal/plugin` + `internal/update` interception events (Go parity).
2. Effect introspection parity (`EffectMeta` trees are implemented; expose
   nested labels on more registration kinds).
3. Logger service.

### Zig

Landed since the foundation: registry view, serial / waterfall / parallel
dispatch modes, batch coalescing, effect labels with introspection, typed
events, shared isolation labels.

1. Snapshot/restore and status events (Rust parity).
2. Accessor/mixin derived services (Go parity).
3. Logger service.
4. Loader / hmr equivalents — pending the module-layout decision.

### Repo

Bounded repo tasks (CI guards, lint gates, post-push verification) live in
`TODO_LIST.md`; `nix flake check` remains the local port gate until a CI
job enforces it remotely.

### Upstream sync (2026-09-08)

`main` was rebased onto upstream `caab04e`; the fork's TS tree now carries
the three-stage reload (#111), include journal (#121), bare-specifier
resolution (#123), `hmr.watch()` (#128) and the upstream `3-stage-hmr`
fix branch replayed on top (`b4650df`: commit-based loader entry changes,
atomic include writes). Local TS suite: 248/248. The parity gaps this
opens for the ports are tracked in the Go section above.

### Open decisions (user-gated)

- **Push policy:** local `main` diverged from `origin/main` after the
  rebase; completing the sync needs force-with-lease approval.
- **`yarn.lock` policy:** commit a generated lockfile (reproducible CI) vs
  stay lock-free tracking upstream; a missing lockfile broke installs
  twice in fork history.
- **TS toolchain stance:** track upstream exactly (their pins, their
  breakage) vs fork-pinned dependencies; currently the fork matches
  upstream except a deliberate `@types/node ^26.5.0` bump.
- **hmr fixture style:** keep fixtures byte-identical to upstream (their
  specs string-replace into them) vs fork-styling fixtures and rewriting
  the specs' replace patterns.
- **One-session-per-worktree convention** for concurrent agents.
- **Oxlint policy for upstream TS** (report-only today).
- **Generic-method API deprecation timeline** (see Go section).

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

Bidirectional feedback started: #1 (lock-free-callback regression test)
and #2 (RequireNoResidue helper) were filed from Kernovia's port experience.
Ongoing interest: Kernovia's loader/hmr are production-proven candidates for
this repo's "Loader / hmr equivalents" gap — the adoption decision (Go/No-Go
with gates) is recorded in Kernovia's ADR-004 appendix. When evaluating the
module-layout decision, factor in a consumer that exercises the calculus
daily.

### Go 1.27 adoption (2026-09-07)

Adopted: `go 1.27` module directive with flake `go_1_27` and CI
`go-version: stable`; timer tests run in a `testing/synctest` bubble on the
virtual clock (`synctest.Sleep` is new in 1.27), turning 0.46 s of real
sleeps into ~2 ms and making debounce/throttle boundaries immune to CI load;
`strings.CutLast` in plugin name derivation; `stdversion` vet (automatic
under `go test`).

Deliberately NOT adopted: generic methods (new in 1.27) for the typed API.
`Provide[T]/Get[T]/On[E]` stay package-level functions because `Context`
already owns the method names `Provide/Get/On/Once/Emit` for the named
(dynamic-name) API, Go has no overloading, and splitting typed lookups
across methods and functions would fragment the surface. Interfaces also
cannot declare or be implemented by generic methods in 1.27, so typed
methods could not participate in the framework's interfaces. Revisit only
if the named forms ever move off `Context`.

Considered, no current use: explicit `encoding/json/v2` API migration (v1
is already v2-backed; migrating risks upstream parity for zero need),
`goroutineleak` profile as a test gate, stdlib `uuid` (realm keys are
uint64), `simd` experiments.
