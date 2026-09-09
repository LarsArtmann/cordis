# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Releases are cut as port tags (`go/v*`, `rust/v*`); the TypeScript packages
in `packages/` track upstream and are not released from this fork.

## [Unreleased]

### Added

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

- Upstream pin bumped `caab04e` → `f8ea3cd` (rc.10); its entire delta is
  the eight workspace manifests, adopted byte-for-byte.

### Fixed

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
