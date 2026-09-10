# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Releases are cut as port tags (`go/v*`, `rust/v*`); the TypeScript packages
in `packages/` track upstream and are not released from this fork.

## [Unreleased]

### Added

- **Rust port: interception events** — `internal/get` (fallback services
  via a `GetResult` waterfall around failed lookups), `internal/set`
  (veto/observe/rewrite service registrations), `internal/listener`
  (replace listener registrations) and `internal/dispatch` (observe every
  non-internal dispatch), mirroring the Go contracts with an
  ownership-carrying `SetOutcome` cell where Go hands a `Disposer`
  through `any`.
- **Rust port: logger service** — `Context::logger` (explicit name >
  `LoggerIntercept` > fiber name), `Level`, typed `Arg`s, the
  `Exporter` trait, `Context::add_exporter`/`clear_exporters`/
  `logger_buffer`/`set_logger_buffer_size`, `format_message` with the
  upstream printf verbs and a `ConsoleExporter`. The framework error
  channel now dispatches into the logger; `Context::logged_errors`
  reads the buffer's error entries back.
- **Rust port: labeled cleanups** — `Context::attach_labeled` exposes the
    cleanup's label in the `Fiber::effects` introspection tree, matching
    Go's `Context.Cleanup(label, run)`; `attach` keeps its generic label.
- **Zig port: intercept** — `Context.intercept(name, value)` /
  `intercepted(name)` per-scope service configuration, honoring
  `LoggerIntercept` values.
- **Zig port: internal events** — `event_status` (a `StatusChange` per
  fiber state transition, same emission order as Go/Rust),
  `event_plugin` (fiber created/disposed) and the `event_update`
  waterfall wrapping `Fiber.update` (rewrite via `next.invoke`, veto by
  not invoking). Root updates now fail with `error.RootUpdate` and root
  `restart` rolls the root scope back instead of falling into the plugin
  load path.
- **Zig port: registry snapshot/restore** — `Context.snapshot` /
  `restore` with a stashing `Registry.delete`, mirroring the Go and Rust
  semantics (delta disposed and stashed, missing runtimes restarted from
  the stash on the calling context, pending fibers requeued).
- **Zig port: config validation** — `ValidatedPlugin(name, Config,
  validate, apply, inject)`; a rejected config fails the start with
  `error.Validation` before any fiber exists (the Rust shape).
- **Zig port: logger service** — `Context.logger`, `Level`, `Message`,
  `Exporter.bind`, `Context.addExporter`/`clearExporters`/
  `loggerBuffer`/`setLoggerBufferSize`, `formatMessage` and
  `ConsoleExporter`; the error channel dispatches into it and
  `loggedErrors` reads the error entries back (now fallible:
  `Error![][]const u8`).
- **Zig port: accessors and mixins** — comptime `accessor(S, V, ctx,
  name, get, set)` and `mixin(S, V, ...)` publishing derived services
  that follow the source's lifecycle, with a typed `Member(S, V)`
  write-back handle (`set` returns `error.ReadOnlyAccessor` without a
  write function).
- **Zig port: named registration labels** — `ctx.on(name)`,
  `ctx.provide(name)` and `ctx.once(name)` now carry their names in the
  `effects()` introspection tree like the Go and Rust ports, plus
  `Context.attachLabeled`.

- Panic-allowlist CI gate (`scripts/panic-allowlist.sh`): production panic
  sites in the Go, Rust and Zig ports are pinned to the reviewed,
  channel-less set with per-file counts and rationales. A new panic fails
  the gate until the allowlist grows with a rationale; a removed one fails
  until the entry shrinks. Wired as the flake `panic-allowlist` check, the
  `test-panic-allowlist` app, a step of `test`, and a dedicated Ports CI
  job.
- Regression tests pinning the panic-free sweep behavior: Go
  `TestIsolateUncomparableLabelConfigErrors` (uncomparable label decoded
  from entry config surfaces as an error, not a crash) and Rust
  `inject_after_clone_returns_plugin_shared` (cloned `FnPlugin` reports
  `PluginShared`; the unshared original still injects).
- CI `upstream-parity` hardening: a manifest divergence guard
  (`scripts/manifest-parity.mjs`) that fails on any `package.json`
  difference against the pinned upstream commit outside an explicit
  allowlist (currently only root `@types/node`), and an hmr
  fixture-coupling canary (`scripts/hmr-fixture-canary.mjs`) that fails in
  milliseconds when a spec `.replace()` literal no longer exists in any
  fixture (previously a silent no-op followed by ~190 s of waitFor
  timeouts).
- The upstream pin now lives in the tracked one-line `.github/UPSTREAM_PIN`
  file that `ports.yml` reads, so pin bumps review as one-line diffs.

### Changed

- **Breaking, all ports: panic-free typed-errors sweep.** Recoverable
  panic sites became typed errors; misuse became compile-time
  impossibilities. Go: `Waterfall` takes the terminal function as a typed
  parameter (arg-misuse no longer compiles); `Isolate` returns
  `(*Context, error)` with uncomparable labels reported as errors instead
  of panics; `randomID` returns `(string, error)`, propagated through the
  loader with entry/group context. Rust: the fiber arena stores `Rc`
  directly (the never-taken `.expect` is structurally gone);
  `FnPlugin.inject` returns `Result<Self>` via the new
  `Error::PluginShared` (injecting after clone is an error, not a panic).
  Zig: `Error.OutOfMemory` is threaded through every fallible
  registration and the scope constructors (`extend`, `isolate`,
  `isolateShared`, `withFilter`) plus `realmFilter` (now `Error!Filter`).
  Golden scenarios remain byte-identical across all three ports.
- Zig reachability audit: `isolateShared`, `provideNamed`, `restart`,
  `update` and `startPlugin` can no longer reach an `@panic` through
  `sharedKey`, `rootKey`, `queue` or `bindCleanup`; the error log drops
  its line under allocation failure instead of aborting the drain;
  `isolateKey` keeps its infallible `u64` form for query paths with an
  internal fallible variant for error-channel callers. Zig dispatch-path
  aborts: 27 → 15, each pinned by the allowlist gate.
- Upstream pin bumped `caab04e` → `f8ea3cd` (rc.10); its entire delta is
  the eight workspace manifests, adopted byte-for-byte.

### Fixed

- Go port: the unload guard was missing — a service withdrawal notified
  dependent fibers but did not settle them before the provider's remaining
  cleanups ran, so in the canonical pool idiom (one effect owning a pool
  and its service) `pool.destroy()` executed before dependents handed
  their handles back. Upstream awaits dependents inside the provide
  disposer (`Promise.allSettled`), the calculus's L-Unload guard
  (Theorem 70). The provide cleanup now drains the notified dependents
  synchronously (`core.settlePending`), matching upstream interleaving;
  pinned by `go/teardown_order_test.go` (fiber dispose, dependency
  chains leaf-first, direct effect disposal inside a batch, restart).
  Rust and Zig still queue dependents without the guard — tracked as
  their top parity task in `ROADMAP.md`.
- Go loader: an uncomparable `Isolate` label decoded from entry config
  (YAML/JSON user input) used to panic the process; it is now a returned
  error wrapped with the entry name.
- Build CI red since 2026-09-08 (`2ac1be1`): the TypeScript 7 toolchain
  bump broke the dts build (`TS2665: Module 'cordis' resolves to an
  untyped module`) on three consecutive pushes while tests stayed green
  (vitest never typechecks). Reverted the whole bump-and-restyle set to
  the upstream pins (yarn 4.14.1, TypeScript ^5.9.3, vitest ^4.1.5,
  esbuild ^0.28.0); the only manifest delta is again the deliberate
  `@types/node ^26.5.0`.
- `upstream-parity` semantic guard failure: restored
  `packages/core/src/context.ts` (stray blank line) and
  `packages/core/src/registry.ts` (re-braced single-statement `if`) to
  upstream bytes — both survive prettier normalization and had drifted in
  the reformat pass.

### Added (prior entries)

- Multi-language ports of the cordis core — Go (flagship), Rust and Zig —
  with semantics parity (fiber lifecycle, drain queue, isolation realms,
  LIFO rollback) and native, type-keyed APIs per language (from
  2026-08-22).
- Go ecosystem packages: `timer` (synctest-tested), `group`, `loader`
  (config-driven start, watch/reload with rollback) and `hmr` (module swap
  with all-or-nothing rollback).
- Go service extensions: accessor/mixin derived services, callable services
  with tracker attribution, `Fiber.Err`, cancellable await, typed inject
  sugar (`Inject1/2/3`).
- Rust: opt-in `thread-safe` feature (Mutex core, stress-tested), registry
  snapshot/restore, `internal/status` events, intercept and config
  validation.
- Zig: all five dispatch modes, batch transactions, effect scopes with
  introspection, disposers, registry view; typed registry operations
  (`Registry.hasTyped`/`deleteTyped`) keyed by `TypedPlugin` identity, and
  a `zig build docs` emission gate enforced by the flake's zig check.
- Cross-language assurance: four golden scenarios executed
  byte-identically by the Go, Rust and Zig runners (lifecycle, events,
  cascade and dispatch), `nix flake check` derivations, Go benchmarks, a
  randomized LIFO-disposal property test and the
  `scripts/parity-matrix.sh` navigator.
- Go loader: `Resolver.ReplaceType[C]` sugar mirroring `RegisterType[C]`,
  composed with it on a shared `TypedRegistration` builder that hmr's
  `SwapType` and the tests use as well; a Go-only golden transcript of the
  watch/reload lifecycle (`go/loader/testdata/watch-golden.txt`, the
  loader has no Rust/Zig port); and an hmr concurrency storm test racing
  parallel `Swap` calls against `Tree.Create`/`Remove`.
- CI: a `flake` job running `nix flake check` so the flake gate (including
  the `-race -count=3` canary the local gate now shares) is enforced
  remotely; an `upstream-parity` job that pins the last-synced upstream
  commit and guards `packages/**` (byte-identical non-TS files,
  prettier-normalized TS/JS semantic parity, `dprint.json` excludes
  covering `packages/**`); and a Go fuzz target for the loader's JSON
  config layer (`EncodeConfig`/`DecodeConfig` idempotent roundtrip).
- Go timer: a randomized debounce/throttle property test whose
  event-simulation oracle runs on a fixed seed inside the synctest bubble.
- Regression tests pinning the wrapped error messages of
  `Accessor`/`Mixin` and `Tree.Create`/`Move` (context prefix plus
  `errors.Is`-preserved cause).
- Go 1.27 adoption: `testing/synctest` virtual-clock timer tests,
  `strings.CutLast` in plugin name derivation.
- Rust: `internal/plugin` + `internal/update` interception events
  (`EVENT_PLUGIN`/`EVENT_UPDATE`) with the Go listener contract — the
  plugin event fires before the fiber's first transition and again before
  disposal rolls its effects back; the update event runs as a waterfall
  (`fiber, config, no_save, next`) whose terminal stores the settled
  config and queues the restart. Root-fiber guards arrive with it:
  updating the root now returns `Error::RootUpdate` and restarting it
  rolls the root scope back in place with its identity intact (both
  previously drained the root's effects and corrupted its uid), and
  `Fiber::name()` keeps the captured plugin name for dying fibers
  instead of falling back to `"root"`.
- Rust: `benches/core.rs` mirroring the six `go/bench_test.go` hot paths
  (`cargo bench`, dependency-free harness) and a cargo-llvm-cov coverage
  baseline of 86.4% lines / 86.1% regions (2026-09-08) next to Go's
  ≈90% statement coverage. The Go 1.27 "up to 30% faster small
  allocations" release-note claim was verified against go.dev and
  reproduced locally (20–37% on quiet A/B runs, 1.26.7 vs 1.27), with
  numbers recorded in ROADMAP.

### Changed (prior entries)

- Rust thread-safe build: lock scopes tightened where semantics-preserving
  (`queue`, `notify_dependents`, `once`, `get_named`, `restore`, the state
  machine's guard merges); the snapshot-consistency reads and the
  `deps_ready` dependency check keep explicit allowlisted holds with
  rationale. `cargo clippy --all-targets --features thread-safe` is green
  and now gated in the flake check, the flake apps and Ports CI, so
  neither feature variant can regress silently.
- Rebased onto upstream `caab04e` (2026-09-08): inherited the three-stage
  reload (#111), include journal reconciliation (#121), bare-specifier
  resolution (#123) and `hmr.watch()` (#128), plus the upstream
  `3-stage-hmr` fix branch replayed on top — the loader now commits
  `EntryChange` records instead of rewriting config files, and include
  writes are atomic.
- Root `README.md` is now a fork-owned sales page;
  `packages/core/README.md` is byte-identical to upstream again.
- TypeScript workspace toolchain upgrade, verified against the full
  248/248 suite plus `yarn lint` (2026-09-08): `packageManager` bumped to
  yarn 4.18.0 — the version that can install TypeScript 7 (4.14.1's
  builtin compat patch crashes on the missing `lib/_tsc.js`);
  `typescript ^7.0.2`, `vitest ^5.0.0`, `esbuild ^0.28.2`, hmr's
  `chokidar ^5`/`@babel/code-frame ^8`. vite stays at `^7.3.2`
  (vite 8's module-runner breaks the suite: SyntaxError on
  decorator.spec.ts plus hmr waitFor timeouts), js-yaml at upstream's
  `^4.1.0` (js-yaml 5 removes `yaml.Type`, which the include build
  calls), and eslint at upstream's `^8.57.1` (eslint 10 removed
  `.eslintrc` support; CI runs `yarn lint` over the legacy config).
  Test fixtures (hmr plugin `.ts`, include fixtures) restored to
  upstream bytes — the spec `.replace()` literals are coupled to them.

- `Tree.Await` now routes fiber failures it observes into the entry's
  error sink, making `Await` + `Errors()` a complete failure picture
  instead of discarding runtime (post-start) fiber errors.
- `ErrInactiveEffect` is declared as the `error` interface so `errors.Is`
  call sites match the sentinel guard; a Go 1.27 `go fix` modernizer sweep
  (`rangeint`, `SplitSeq`, `CutPrefix`, `reflect.TypeFor`, `maps.Copy`)
  is applied across the Go port.
- hmr rollback errors carry the failed fiber's own `Fiber.Err()` detail
  (`hmr: entry <id> failed under the new implementation: <cause>`).

### Fixed (prior entries)

- Zig `Registry.delete` iterated the live runtime list while disposing
  fibers mutated (and, with the last fiber, freed) that same list; the stale
  slice read poisoned memory and crashed on plugins with two or more
  fibers. Fiber ids are now snapshotted and the registry entry dropped
  before disposal, matching Go's `Registry.Delete` and Rust's `delete_id`.
- Thread-safe Rust: `Fiber::name` self-deadlock (nested core locks) that
  hung CI Ports runs for over an hour; lock-order invariant documented.
- Go loader: `PollWatcher` captured its baseline at the first poll, so a
  config written right after `Serve` was never detected; the baseline is
  now primed in the constructor.
- hmr test fixtures restored byte-identical to upstream after a formatting
  pass silently broke the specs' literal string replaces (every reload
  test timed out).
- TS workspace manifests realigned to upstream after a dep sync reintroduced
  `typescript ^7.0.2` (every `yarn install` crashed on yarn's `lib/_tsc.js`
  compat lstat) and carried fork-era bumps upstream never adopted
  (`js-yaml ^5` broke `yaml.Type` at build time, plus `chokidar ^5`,
  `supports-color ^11` and friends); the root manifest keeps only the
  deliberate `@types/node ^26.5.0` delta.
- `packages/core` formatting churn reverted to upstream bytes (extra blank
  line, re-braced guard, reformatted `bin.js`) so the upstream-parity
  guards hold; include plugin fixtures and `base.yml` restored to pin
  bytes; `tmp-*` test debris gitignored.
- `timer.IntervalFunc` no longer schedules a callback after its disposer
  ran: the pump goroutine owns callback dispatch and re-checks the stop
  signal after each tick. A slow callback now delays the pump (ticks
  dropped, standard `time.Ticker` semantics) instead of queueing a
  post-disposal invocation; an in-flight invocation still completes and
  the disposer does not wait for it.

### Releases

- `go/v0.1.0` (2026-09-05): first Go module tag — core plus ecosystem.
- `rust/v0.2.0` (2026-09-05): registry snapshot/restore and status events.
