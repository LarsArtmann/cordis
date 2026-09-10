# Roadmap

Status of the multi-language ports relative to the TypeScript reference
implementation (`packages/core`). Go is the flagship; Rust and Zig follow
its architecture.

## Parity matrix

| Feature                                                     | Go                 | Rust                              | Zig                               |
| ----------------------------------------------------------- | ------------------ | --------------------------------- | --------------------------------- |
| Context tree (extend)                                       | DONE               | DONE                              | DONE                              |
| Isolation realms + shared labels                            | DONE               | DONE                              | DONE                              |
| Intercept (per-scope service config)                        | DONE               | DONE                              | DONE                              |
| Effects: nested, labeled, LIFO rollback, introspection      | DONE               | DONE                              | DONE                              |
| Events: emit / parallel / serial / bail / waterfall         | DONE               | DONE                              | DONE                              |
| Event filters + global listeners                            | DONE               | DONE                              | DONE                              |
| Fiber states, dispose, restart, update                      | DONE               | DONE                              | DONE                              |
| Inject reactivity (pending / unload / reload in place)      | DONE               | DONE                              | DONE                              |
| Registry (size / has / delete, snapshot restore)            | DONE               | DONE                              | DONE                              |
| Interception events (internal/get\|set\|listener\|dispatch) | DONE               | DONE                              | -                                 |
| Status events (internal/status)                             | DONE               | DONE                              | DONE                              |
| Config validation                                           | DONE               | DONE                              | DONE                              |
| Fiber Await (+ stdlib-context variant in Go)                | DONE               | n/a (drain settles synchronously) | n/a (drain settles synchronously) |
| Batch transactions                                          | DONE               | DONE                              | DONE                              |
| Logger service (levels, exporters, buffer)                  | DONE               | DONE                              | DONE                              |
| Accessor / mixin derived services                           | DONE               | -                                 | DONE                              |
| Callable services + tracker                                 | DONE               | -                                 | -                                 |
| Timer (interval, debounce, throttle)                        | DONE               | -                                 | -                                 |
| Loader (config entries, watch/reload)                       | DONE               | -                                 | -                                 |
| HMR (implementation swap, rollback)                         | DONE               | -                                 | -                                 |
| Concurrent access safety                                    | DONE (race tested) | thread-safe build (Mutex)         | single-threaded                   |

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
  associated `Config` (closure form preserved as `FnPlugin`/`start_fn`),
  logger service (`Context::logger`, `Exporter` trait, `format_message`,
  2026-09-10), interception events with an ownership-carrying
  `SetOutcome` cell (2026-09-10).
- **Zig**: comptime plugin construction (`TypedPlugin(name, Config, apply,
  inject)` — the type is the registry identity), validated plugins
  (`ValidatedPlugin(name, Config, validate, apply, inject)`), typed
  services/events keyed by `@typeName`, plain cleanup attachment
  (`Context.attach` / `attachLabeled`), registry snapshot/restore with a
  stashing delete, internal events (`event_status`, `event_plugin`,
  `event_update` waterfall in `Fiber.update`), logger service
  (`Context.logger`, `Exporter.bind`, 2026-09-10), comptime accessors
  (`accessor(S, V, ...)` / `mixin(S, V, ...)` with a typed `Member(S, V)`
  write-back handle), `OutOfMemory` threaded through every fallible
  registration and scope constructor (2026-09-10; dispatch callbacks keep
  the abort path, see the panic-free surface note below).

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
- Panic-free surface (2026-09-10, user directive "typed errors over
  panics"): every framework panic carrying a runtime misuse or
  environmental failure became a typed error or a compile error.
  (1) Go `Waterfall` takes the terminal function as a typed parameter
  instead of upstream's last-`any`-argument convention, so a missing or
  mistyped terminal is a compile error; dispatch-observer args are
  unchanged. (2) Go `Isolate` returns `(*Context, error)` because the
  loader feeds config-defined labels straight in: an uncomparable label
  is user input, not a programmer mistake. (3) Go `loader` id generation
  returns the crypto/rand error instead of panicking. (4) Rust
  `FnPlugin.inject` returns `Result` with the new `Error::PluginShared`
  variant (typestate would break the cheap-clone registry identity
  model); the fiber arena stores `Rc` values directly, dropping the
  never-taken `expect`. (5) Zig `extend`/`isolate`/`isolateShared`/
  `withFilter` return `Error!*Context`, all fallible registrations return
  `error.OutOfMemory`, and a reachability audit (2026-09-10) closed every
  path from a fallible public API to an `@panic`: `sharedKey`, `rootKey`,
  `queue` and `bindCleanup` became fallible, `realmFilter` returns
  `Error!Filter`, and the error log drops its line under allocation
  failure instead of aborting the drain that reports a plugin failure.
  What deliberately remains: the typed event
  arity/type guards (Go/Rust) fire inside listener wrappers at dispatch,
  where the shared `func(E)`/`Fn(&E)` callback contract has no error
  channel in any port; they guard the typed/string boundary and are
  pinned by tests. The remaining Zig aborts (17 after the 2026-09-10
  parity pass: emit/bail/serial/waterfall dispatch, `effects()`,
  `Registry.delete` including its stash write, `isolateKey`,
  `Fiber.dispose`, and the `event_update` terminal's queue write) keep
  the distinct message `cordis: out of memory in dispatch` and are pinned
  by `scripts/panic-allowlist.sh`. Go `Must*`
  helpers are opt-in sugar over error-returning APIs, the idiomatic Go
  escape hatch.
- Rust rejects root-fiber updates with a typed `Error::RootUpdate` where
  Go returns a plain error string; both roll the root scope back in place
  with their identity intact. Zig now returns `error.RootUpdate` too
  (2026-09-10), and its `Fiber.update` runs the `event_update` waterfall
  like the other ports — a null config clears the stored config directly,
  with nothing to intercept.
- Interception contracts follow each language's ownership model (landed
  2026-09-10): Go's `internal/set`/`internal/listener` hand a `Disposer`
  through the `any` channel; Rust's listeners fill a shared
  `SetOutcome` cell (`Rc<RefCell<..>>`/`Arc<Mutex<..>>`) because a
  `Disposer` cannot travel through `Rc<dyn Any>` by value; Zig has no
  interception events yet (only the loader-relevant `get`/`set` pair is
  on its roadmap when a Zig loader is decided).
- Config validation failure shapes: Go fails the created fiber
  (`StateFailed`, logged); Rust and Zig (2026-09-10) return the typed
  error (`Error::Validation`) before any fiber exists.
- Logger shape divergences (2026-09-10): Rust's `Arg` is a typed enum
  (`Arg::Json` carries pre-serialized `%o` payloads — the crate is
  dependency free) and does not expand joined/wrapped errors like Go,
  whose flat expansion relies on `errors.Join`/`Unwrap`. Zig has no
  ambient clock in 0.16 (clocks moved to `std.Io`), so `Message.time`
  carries the tree's monotonic sequence, and `formatMessage` space-joins
  the pre-rendered arguments because Zig formats at the call site through
  `std.fmt` — the printf verb pipeline is a Go/Rust surface.
- Zig registration labels now carry their names (`ctx.on(name)`,
  `ctx.provide(name)`, `ctx.once(name)`) matching the Go/Rust
  introspection trees; `attach` grew the labeled variant `attachLabeled`
  like Go's `Context.Cleanup(label, run)` and Rust's
  `Context::attach_labeled`.
- String event names are not restricted in code, but the convention is
  that only the framework's `internal/` namespace uses them; application
  events should be typed.
- Sibling notification order is deterministic (creation order) in all
  three ports, matching the insertion-ordered maps upstream; Go sorts by
  fiber uid where its map iteration would otherwise be random.

Cross-language assurance: four golden scenarios (`golden/`) executed by the
Go, Rust and Zig test suites with byte-identical expected traces, plus
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

Nothing pending from the previous list — effect introspection
(`attach_labeled`), the logger service and the
`internal/get|set|listener|dispatch` interception events landed
2026-09-10. The loader prerequisites (`get`/`set` interception) now
exist; a Rust loader/hmr port is the remaining gap, pending the same
module-layout decision as Zig.

### Zig

Landed since the foundation: registry view, serial / waterfall / parallel
dispatch modes, batch coalescing, effect labels with introspection, typed
events, shared isolation labels; and on 2026-09-10: intercept
(`Context.intercept`/`intercepted`), registry snapshot/restore with a
stashing delete, `internal/status`/`plugin` events plus the
`internal/update` waterfall, config validation (`ValidatedPlugin`), the
logger service and comptime accessors/mixins.

1. `internal/get|set|listener|dispatch` interception events — needed only
   once a Zig loader/hmr port is decided.
2. Loader / hmr equivalents — pending the module-layout decision.

### Repo

Bounded repo tasks (CI guards, lint gates, post-push verification) live in
`TODO_LIST.md`; `nix flake check` remains the local port gate until a CI
job enforces it remotely.

### Upstream sync (2026-09-08, pin bumped 2026-09-09)

`main` was rebased onto upstream `caab04e`; the fork's TS tree now carries
the three-stage reload (#111), include journal (#121), bare-specifier
resolution (#123), `hmr.watch()` (#128) and the upstream `3-stage-hmr`
fix branch replayed on top (`b4650df`: commit-based loader entry changes,
atomic include writes). Local TS suite: 248/248. On 2026-09-09 the pin
moved to `f8ea3cd` (rc.10 version set) — a manifest-only upstream delta,
adopted byte-for-byte. The parity gaps this
opens for the ports are tracked in the Go section above.

### Open decisions (user-gated)

- **`yarn.lock` policy:** commit a generated lockfile (reproducible CI) vs
  stay lock-free tracking upstream; a missing lockfile broke installs
  twice in fork history.
- **TS toolchain stance:** track upstream exactly (their pins, their
  breakage) vs fork-pinned dependencies. Resolved in practice 2026-09-09:
  the fork matches upstream exactly except a deliberate
  `@types/node ^26.5.0` bump, now machine-enforced by the
  `upstream-parity` manifest guard. TypeScript 7 is empirically fatal for
  the dts build (TS2665, three red CI runs on 2026-09-08) — re-evaluate
  only after upstream adopts a TS 7 that builds their own workspace.
- **hmr fixture style:** ~~keep fixtures byte-identical to upstream (their
  specs string-replace into them) vs fork-styling fixtures and rewriting
  the specs' replace patterns.~~ Resolved 2026-09-08: fixtures (all of
  `packages/hmr/tests/`) stay byte-identical to upstream — fork-styling
  them no-ops the specs' literal replaces and timed out 17 tests; the
  `upstream-parity` CI job enforces it.
- **One-session-per-worktree convention** for concurrent agents.
- **Oxlint policy for upstream TS** (report-only today).
- **Generic-method API deprecation timeline** (see Go section).
- **Coverage and bench policy:** make the measured baselines (Go ≈90%,
  Rust 86.4%) enforced gates with a floor, and keep per-port bench
  baselines separate until one shared harness methodology exists, or
  record-only?
- **Panic-free sweep release policy (2026-09-10):** the breaking surface
  (Go `Waterfall`/`Isolate`, Rust `FnPlugin.inject`, Zig fallible scope
  constructors + `realmFilter`) — major version bump + port tags now, or
  ride to the next planned release? The CHANGELOG `[Unreleased]` section
  already records the surface.
- **Zig dispatch end-state (2026-09-10):** is the channel-less dispatch
  abort (17 reviewed `@panic` sites, allowlist-gated) a permanent end
  state, or should the shared dispatch callback contract change across
  all three ports (envelope values / error-carrying listeners) for
  literal zero panics outside `Must*` sugar?
- **Go `Must*` fate (2026-09-10):** delete `MustGet`/`MustGetNamed`/
  `MustRegister` (breaking wiring code) or keep them as the documented,
  explicitly opt-in panic sugar?

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

Measured locally (2026-09-08, x86_64-linux, 1.26.7 vs 1.27 back-to-back):
the release note's size-specialized allocation routines — "reducing the
cost of some small (<80 byte) memory allocations by up to 30%"
(go.dev/doc/go1.27) — reproduce as 20–37% faster small allocations on
quiet runs (32-byte struct: 13.95 → 8.76 ns/op; 72-byte: 19.04 →
13.70 ns/op), noise-dominated when the desktop is loaded. Framework hot
paths under 1.27 (`go test -bench . -benchmem`): start/dispose ≈2.4 µs,
provide+get+dispose ≈0.61 µs, get ≈23 ns, emit ≈65 ns, 5-hop waterfall
≈0.21 µs; the Rust port posts the same six benchmarks in
`rust/benches/core.rs` (start/dispose ≈0.56 µs, get ≈24 ns, waterfall
≈0.30 µs, same machine, best of five).

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
