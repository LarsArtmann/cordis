# AGENTS.md — cordis (multi-language fork)

This fork of [cordiverse/cordis](https://github.com/cordiverse/cordis) adds
Go, Rust and Zig ports next to the TypeScript original in `packages/`.
**Go is the flagship port** and the reference implementation for the others.

## Layout

- `packages/` — TypeScript original (yarn workspaces, vitest).
- `go/` — Go module `github.com/LarsArtmann/cordis/go`, package `cordis`
  plus the `timer`, `group`, `loader` and `hmr` subpackages.
- `rust/` — Cargo crate `cordis` (single-threaded `Rc`/`RefCell` by
  default, opt-in `thread-safe` Mutex build).
- `zig/` — Zig module (0.16), arena-based memory, tested via build.zig.
- `PORTS.md` — shared port architecture. `ROADMAP.md` — parity matrix.

## Build and test

Use the flake (`nix run .#test`, `.#test-go`, `.#test-rust`, `.#test-zig`)
or run directly:

- Go: `cd go && go test ./...` (also: `go vet`, `-race` clean; statement
  coverage ≈90% for every package including loader — 90.8% measured
  2026-09-08).
  Requires Go 1.27 (see gotcha below). Timer tests run in a
  `testing/synctest` bubble (virtual clock, ~2 ms, deterministic); write
  new timing tests the same way instead of `time.Sleep`.
  The repo root is itself a minimal Go module (`go.mod` + `doc.go` stub,
  no buildable code) so repo-root Go tooling (`go generate/test/vet
  ./...`, e.g. buildflow's go-generate/test-race steps) resolves; do not
  delete it and do not put port code in it — the port lives in the nested
  `go/` module.
- Rust: `cd rust && cargo test` (clippy clean on both feature variants:
  `cargo clippy --all-targets [--features thread-safe]`, gated in the flake
  check/apps and ports.yml). Coverage baseline: 86.4% lines / 86.1% regions
  (cargo-llvm-cov 0.8.5, 2026-09-08; code files 83–90%, `sync.rs` is cfg
  plumbing and `lib.rs` doc-only) next to Go's ≈90% statement coverage —
  reproduce with `nix shell nixpkgs#cargo-llvm-cov nixpkgs#llvm -c sh -c
  'export LLVM_COV=$(command -v llvm-cov) LLVM_PROFDATA=$(command -v
  llvm-profdata); cd rust && cargo llvm-cov --summary-only'`. Benchmarks:
  `cargo bench` (`benches/core.rs`, mirrors the six `go/bench_test.go`
  hot paths, dependency-free harness, best-of-five).
- Zig: `cd zig && zig build test` (leak-checked via testing.allocator);
  `cd zig && zig build docs` must also pass — the doc-emission gate is wired
  into the flake's zig check next to `zig fmt --check`.

### Environment gotcha (this machine)

`GOCACHE` defaults to `/mnt/buildcache/go-build` and `~/.cache/go-build` is a
dangling home-manager symlink. Both are broken. Always run Go with
`export GOCACHE=/tmp/gocache` (the flake apps do this automatically).
The system `go` is 1.26.x with `GOTOOLCHAIN=local`, so it refuses the
module's `go 1.27` directive; use the flake devShell (pins `go_1_27`) or
`nix shell nixpkgs#go_1_27 -c sh -c 'export GOCACHE=/tmp/gocache; cd go &&
go test ./...'`.
Zig is not installed globally; use `nix run nixpkgs#zig -- ...` or the flake
devShell. The devShell also ships `gcc` (needed for `go test -race`, which
requires cgo) and resolves `GOCACHE` via `shellHook` (an attr using
`builtins.getEnv "HOME"` breaks under pure eval, where getEnv returns "").
`govalid` on PATH: `~/go/bin/govalid` is rebuilt with Go 1.27; the
system-profile copy is built with Go 1.26 and warns about x/tools skew.

## TS workspace gotchas (learned in the 2026-09-07 upstream rebase)

- **Toolchain pins are load-bearing.** VERIFIED WORKING SET (2026-09-09,
  from-scratch install + CI-sequence build + 248/248 suite): the
  manifests are the upstream pin's bytes and the fork's ONLY delta is root
  `@types/node ^26.5.0` — yarn 4.14.1, TypeScript ^5.9.3, vitest ^4.1.5,
  vite ^7.3.2, esbuild ^0.28.0, eslint ^8.57.1 (all upstream values).
  The `upstream-parity` CI job enforces this via
  `scripts/manifest-parity.mjs` (manifest guard with an in-script
  allowlist). **TypeScript 7 is fatal, confirmed twice** (2026-09-08):
  7.0.2 compiles tests (vitest does not typecheck) but the dts build dies
  with `TS2665: Module 'cordis' resolves to an untyped module at
  lib/index.js` — the workspace's cross-package `declare module 'cordis'`
  augmentation does not resolve under TS 7's rewritten resolver; three CI
  runs were red on it. yarn 4.18.0 was only ever needed to make the TS 7
  tarball installable at all — with TS 5.9.3 stay on upstream's yarn
  4.14.1. js-yaml 5 remains fatal for the include build (`yaml.Type` gone;
  hit twice). There is NO yarn.lock by design (upstream does the same), so
  every install re-resolves: a bad bump breaks `yarn install` for everyone,
  immediately. TS dep bumps are upstream decisions — the manifest guard
  fails CI on divergence outside the allowlist.
- **Single-step `yarn build` from a clean tree fails; the CI two-step
  sequence is green.** From a fully clean state (no `lib/`, no
  `tsconfig.tsbuildinfo`), one `yarn build` deterministically reports ~35
  dts errors (`@Inject('loader')` loses the loader augmentation) in
  hmr/include — a yakumo-tsc project-reference quirk, reproduced on node 24
  and 26 with identical dependency sets. CI's sequence (`yarn build core`
  then `yarn build`) is green, as is any `yarn build` after a prior
  successful build. Do not debug "impossible" clean-build failures before
  trying the two-step sequence; trash `packages/*/lib` +
  `packages/*/tsconfig.tsbuildinfo` before verifying any toolchain change.
- **Test fixtures are string-coupled to the specs.** hmr specs mutate fixture
  sources via literal `content.replace("value = 'initial'", ...)`. Fork-style
  reformatting of `packages/hmr/tests/*` fixtures (yml + plugin `.ts` files)
  silently no-ops those replaces and every reload test times out. ALL test
  fixtures (hmr + include, yml and plugin `.ts`) are byte-identical to
  upstream and CI-guarded by the `upstream-parity` byte check, plus the
  `scripts/hmr-fixture-canary.mjs` fast-fail guard (every spec replace
  literal must exist in a fixture; wired into the same CI job) — it catches
  a mass fixture reformat in milliseconds instead of ~190 s of waitFor
  timeouts. Its matching is any-fixture: it cannot pin a literal to one
  exact fixture when several fixtures share it.
- **packages/** style == upstream's, enforced by CI.** The `upstream-parity`
  job in `ports.yml` pins the last-synced upstream commit (`UPSTREAM_PIN`, a
  one-line tracked file at `.github/UPSTREAM_PIN` so pin bumps review as
  one-line diffs): non-TS files must be byte-identical to the pin; TS/JS
  may differ only by prettier-normalizable formatting (both trees
  normalized with the pinned prettier, then diffed); `package.json` files
  are excluded from both guards BY DESIGN (manifests are the deliberate
  divergence surface) and gated instead by the manifest guard
  (`scripts/manifest-parity.mjs`). Bump `.github/UPSTREAM_PIN` and sync the
  manifests in the same commit as every `packages/**` sync. Prettier is
  NOT a style gate for packages (upstream style is not prettier-stable: 64
  files fail `--check` under every plausible config) — the fork's old
  `prettier --print-width 100` double-quote/semicolon formatting of
  packages was reverted to upstream style in `0542b6d`. CI pins prettier
  3.9.6 (`npx prettier@3.9.6`), the same version nixpkgs shipped on
  2026-09-08 — keep the two in lockstep.
- **Stale `lib/` builds shadow src/.** Cross-package imports resolve through
  each package's `lib/index.js`. After changing TS sources run
  `nix develop -c yarn build` before debugging "impossible" test failures —
  the tests may still be running the old bundle.
- TS dep bumps are upstream decisions, but three bump-everything incidents
  (2026-09-08) hardened the protocol: (1) check the npm registry before
  calling a version fake — chokidar 5, vitest 5, eslint 10 and js-yaml 5
  all exist; (2) verify each major empirically against the harness with a
  from-scratch install AND the CI two-step build — vite 8 breaks the suite,
  eslint 10 breaks `yarn lint` (.eslintrc removal), js-yaml 5 breaks the
  include build (`yaml.Type` gone), TS 7 breaks the dts build (TS2665,
  above); (3) fixture bytes are string-coupled to specs — reformat them
  and every reload test times out. Since 2026-09-09 the manifest guard
  makes (2) a CI gate instead of a protocol.

## Zig 0.16 std gotchas (all verified against 0.16.0 on 2026-09-08)

- **`std.fs.Dir` is gone; it is `std.Io.Dir` now.** The filesystem API moved
  into `std.Io` (`std.Io.Dir`, `std.Io.File`); `std.fs` keeps only
  `path` and a few re-exported constants. `std.fs.cwd()` →
  `std.Io.Dir.cwd()`.
- **Initialize ArrayLists with `.empty`.** `items`/`capacity` have no field
  defaults, so `.{}` fails with "missing struct field: items";
  `std.ArrayListUnmanaged` is now an alias of `std.ArrayList`, and lists are
  unmanaged — methods take the allocator (`list.append(gpa, x)`).
- **Non-zig `@import` is an error and `@embedFile` cannot leave the module
  root** ("no module named 'data.txt'" / "embed of file outside package
  path"). To embed files living outside the module (e.g. `../golden/`), copy
  them into the build cache with `b.addWriteFiles()` and reference them from
  a generated `.zig` shim using `@embedFile` — the `golden_data` pattern in
  `zig/build.zig`.
- **`zig build -femit-docs` does not exist** on the build runner in 0.16.
  Add a `docs` step instead: `Compile.getEmittedDocs()` (a `Compile` method,
  not `Module`) piped through `b.addInstallDirectory` — note
  `InstallDir.Options` takes `source_dir`, not `source`.
- **`orderedRemove` poisons vacated slots with `undefined` (0xAA in Debug).**
  Iterating a slice captured before a remove reads the poison and panics
  with index-out-of-bounds. Snapshot-then-mutate instead: `Registry.delete`
  copies the fiber ids out and drops the registry entry before disposing
  (same order as Go's `Registry.Delete` and Rust's `delete_id`).

## Port architecture (all three languages share this)

**Prime directive (user, 2026-08-22): use each language's native features to
the max. Do NOT port TypeScript 1:1.** Semantics parity (fiber lifecycle,
drain queue, realms, rollback) is the constraint; API surfaces must be
native: type-keyed services and typed events (Go generics / Rust TypeId /
Zig comptime), stdlib `context.Context` per fiber in Go, slog integration,
RAII disposers in Rust, comptime plugin construction in Zig. Where a native
design intentionally diverges from TS behavior, document the divergence in
ROADMAP.md.

1. **Drain queue instead of microtasks.** Public API calls are wrapped in
   `core.enter()`/`core.leave()`. Fiber state transitions are queued and
   coalesced, then executed when the outermost call returns. This replaces
   the TS microtask queue and makes torn intermediate states unobservable.
   `Context.Batch` (Go) groups multiple mutations into one transaction.
2. **No locks/borrows during user callbacks.** All framework state lives in
   one `core` guarded by a single mutex (Go) or RefCell (Rust). Listeners,
   plugin bodies and cleanups always run unlocked; they snapshot state under
   the lock first. Never hold core.mu while calling user code in Go.
3. **Effect tree.** Every registration (listener, service, nested plugin,
   raw cleanup) is a labeled item in a dispose bag. Effect bodies collect
   into a child bag via `ctx.collect` (Go: goroutine-safe by passing a
   derived context, not fiber-global state). Disposal is LIFO everywhere.
4. **Realm keyed services.** Services are stored by a uint64 realm key, not
   by name. `Isolate` maps a name to a fresh key in the child scope; lookups
   walk the parent chain and fall back to the root realm (lazily assigned).
   Shared labels map name+label to one synthetic key.
5. **Fiber state machine.** pending/loading/active/failed/disposed/
   unloading, driven by `transition()` from the drain queue. Dependency
   resolution snapshots a generation counter and retries when the store
   changed mid-resolution (Go).
6. **Plugin identity.** Go: the `*Plugin[C]` value (typed config via
   generics). Rust: process-global atomic id. Zig: the plugin's address.

## Semantics deliberately adapted from TS

- Go drains synchronously: after `Start()` returns, an applicable plugin has
  already run. TS runs it on a microtask.
- Cleanups are synchronous `func()`. Async cleanup is the user's goroutine
  concern.
- `Emit` panics propagate (Go); `Parallel` joins errors (Go errors.Join).
- Plugin apply errors move the fiber to `StateFailed`, roll back partial
  effects and are routed to the logger, never thrown across the framework
  boundary.
- Sibling notification order is deterministic (fiber creation order) in all
  three ports; Go sorts by uid where map iteration would be random.

## Native-max API layer (phase 2, landed 2026-09-04)

The PRIMARY service/event APIs are type-keyed; named/string forms remain
for dynamic names and the `internal/` event namespace:

- Go: `Provide[T](ctx, v)` / `Get[T]` / `TryGet[T]` / `MustGet[T]`,
  `On[E](ctx, func(E))` / `Once[E]` / `Emit[E]`; `ServiceName[T]()` /
  `EventName[E]()` derive the realm-space name; named lookups are
  `GetNamed`/`MustGetNamed` and the `Context.Provide/On/Emit` methods.
- Go fibers own a stdlib context: `Fiber.StdContext()` (cancelled on
  unload/restart/dispose, renewed on load) and `Fiber.Done()`.
- Go logger bridges slog: `NewSlogHandler(ctx, name...)`, `Logger.Slog()`.
- Rust: `provide::<T>` / `get::<T>()` / `try_get`, `on::<E>` / `once::<E>`
  / `emit::<E>`; string forms are `*_named`; `FnPlugin` closures via
  `plugin()` + `start_fn`, trait plugins via `impl Plugin` (associated
  `Config`) + `start` (registry id: `plugin_type_id::<P>()`); RAII `Guard`
  (dispose on drop, `detach()` to keep).
- Zig: `provide(ptr)` / `getTyped(T)` / `onTyped(E, data, f)` /
  `emitTyped(E, &event)` keyed by `@typeName`; string forms are `*Named`;
  comptime plugins via `TypedPlugin(name, Config, apply, inject)` (the
  returned TYPE is the registry identity); runtime `Plugin` values for
  dynamic cases (with optional `data: ?*const anyopaque` context);
  `Context.attach(data, f)` registers plain cleanups. Zig domain errors
  (`Error{InactiveEffect, DuplicateService, PluginFailed}`) exclude OOM:
  allocation failure panics (std style).

Golden scenarios (four: lifecycle `scenario.txt`, events
`scenario-events.txt`, cascade `scenario-cascade.txt`, dispatch
`scenario-dispatch.txt`) are executed by `go/golden_test.go`,
`rust/tests/golden.rs` and `zig/tests/golden.zig` (Zig embeds the files at
build time via `zig/build.zig`). All three ports run all four
byte-identically; the Zig cascade runner shares the lifecycle runner's
op interpreter (`runLifecycleScenario` in `zig/tests/golden.zig`).
Regenerate with `GOLDEN_UPDATE=1` on the Go runner and re-verify Rust and
Zig. Changing semantics? Fix the port, not the golden file. The loader has
no Rust/Zig port, so its watch/reload transcript is pinned by a Go-only
golden trace instead: `go/loader/watch_golden_test.go` +
`go/loader/testdata/watch-golden.txt` (same `GOLDEN_UPDATE=1` escape
hatch; do not move it into `golden/`).

**Untracked files are invisible to `nix flake check`:** the flake source
is the live working tree but Nix flakes in a git repo only copy git-tracked
files into the store, so new files must be `git add`ed before the flake
checks can see them (cost one confusing "NotFound in sandbox" round trip
on 2026-09-08).

## Upstream facts

- The fork tracks `main` of [cordiverse/cordis](https://github.com/cordiverse/cordis)
  exactly inside `packages/`; TS dep bumps belong upstream. After touching
  TS manifests, check `git diff <upstream> -- '**/package.json'`; dep bumps
  are upstream decisions.
- Upstream commits **no yarn.lock** and installs with `yarn --no-immutable`,
  so its CI resolves floating dependency ranges on every run. To reproduce
  TS behavior locally: `nix shell nixpkgs#nodejs_24 nixpkgs#corepack -c sh -c
  'export PATH=/tmp/corepack-bin:$PATH; yarn install && yarn build'`.
  Generated `yarn.lock` / `node_modules/` must never be committed.
- Upstream main's `packages/hmr` test suite used to flake (11 waitFor
  timeouts — an upstream cache-invalidation bug, fixed on their unmerged
  `3-stage-hmr` branch). The fork replayed that branch on top of the rebase
  (`b4650df`): commit-based loader entry changes, atomic include writes,
  `hmr.watch()`. Local TS suite is green (248/248) and CI confirmed it:
  both Build and Ports ran green on `3da7d0f` (runs 34267336684 /
  34267336671, 2026-09-08), including the `flake` and `upstream-parity`
  jobs.
- The root `README.md` is a fork-owned real file (user demand, 2026-09-07;
  replaced upstream's symlink to `packages/core/README.md`). Never recreate
  the symlink and never put fork content into `packages/**/README.md` —
  `packages/core/README.md` must stay byte-identical to upstream.
- Rebase onto upstream `caab04e` completed 2026-09-08 (all 32 fork commits
  replayed). Upstream switched the TS style to double quotes + semicolons
  with prettier-ish wrapping; upstream's delta to loader/hmr/include at the
  conflict points was format-only, so fork TS content is authoritative for
  semantics and future conflicts should re-apply fork deltas onto upstream
  blobs. Upstream `hmr.watch()` (#128) was already in the fork base.
  2026-09-09: pin bumped to `f8ea3cd` (rc.10) — its entire delta vs
  `caab04e` is the eight workspace manifests, which the fork adopted
  byte-for-byte; the guarded tree was already identical.
- RESOLVED (2026-09-09, final): the TS 7 saga had two independent
  breakages. (1) `yarn install` hard-crashed with `typescript: ^7.0.2`
  under yarn 4.14.1 (every TS 7 tarball ships only `bin/tsc`, no
  `lib/_tsc.js`, and yarn's builtin `compat/typescript` patch lstats that
  file) — the 2026-09-08 "fix" was upgrading yarn to 4.18.0, which made
  TS 7 installable. (2) But TS 7 then broke the DTS BUILD (TS2665, see
  the toolchain bullet) — three CI runs red, invisible locally because
  vitest never typechecks and a single-step clean `yarn build` was masked
  by the two-step quirk above. Final resolution: manifests back to
  upstream's exact set (yarn 4.14.1, TS ^5.9.3, vitest ^4.1.5), only
  `@types/node ^26.5.0` diverges, now machine-guarded by
  `scripts/manifest-parity.mjs`. `.yarnrc.yml` is upstream's two lines
  again (the yarn-4.18-era `npmMinimalAgeGate: 0` line is gone). Do not
  re-ship TS 7 without a from-scratch install plus the CI two-step build.

## Repo hygiene facts

- Lint config lives in `.markdownlint.jsonc` (MD013/MD010-in-code-blocks
  off, keepachangelog duplicate headings allowed; strict JSON, no trailing
  commas) and is GATED since 2026-09-08: the flake `markdown` check and the
  `test-markdown` app (also part of `nix run .#test`) run markdownlint over
  the tree. `.markdownlintignore` excludes `docs/status/` +
  `docs/planning/` (frozen history), all of `packages/` (upstream-owned,
  byte-parity-guarded), and vendored/generated dirs (`node_modules/`,
  `target/`, `zig-out/`). `.oxlintrc.json` (`ignorePatterns` skips
  generated `lib/`/`dist/`), and `.buildflow.yml` round out the configs.
  `dprint.json` excludes `packages/**` — upstream owns
  formatting there; never let a formatter rewrite upstream files.
- Run `nix develop -c buildflow` on this machine: buildflow's per-module Go
  fan-out needs a working `go` matching `go/go.mod` (1.27), and the system
  Go is older with `GOTOOLCHAIN=local`. A stale toolchain makes buildflow
  skip module discovery and run every Go step at the repo root, which fails
  with "cannot find main module". Buildflow has no per-step workdir knob
  (verified against `buildflow config init`), which is why the root Go
  module stub exists.
- KNOWN buildflow limitation (2026-09-08): its nix steps enumerate every
  `.#checks.<system>.*` attribute via `nix flake show` and try to build
  them, so `nix-build` + the `nix-hash-fix` cascade fail on this
  x86_64-linux machine with "platform mismatch" for the aarch64-darwin
  checks. Repo-side workarounds all fail: pure eval forbids
  `builtins.currentSystem` (Nix removed it), and emptying `checks` would
  gut `nix flake check` (which passes — it skips incompatible systems).
  Fix belongs in buildflow: filter enumerated checks to the running system
  or skip platform-mismatch errors. `nix flake check` is the port gate.
- erraudit (per-module, via ~/go/bin, go 1.27 build) is green on the Go
  port. Deliberate error discards use `//nolint:erraudit // reason`
  (verified: the directive suppresses); genuine findings get real fixes
  (context-preserving `%w` wrapping, reporting close/rollback errors).
- Rust: `pedantic`+`nursery` are `deny` (Cargo.toml). All findings were
  cleared under nixpkgs rustc 1.97 on 2026-09-08; expect new nursery lints
  after toolchain bumps (fix code, don't weaken the config). The
  `thread-safe` feature's `significant_drop` findings were cleared the same
  day: lock scopes were tightened where semantics-preserving (`queue`,
  `once`, `get_named`, `restore`, the state-machine merges) and three
  snapshot-consistency holds plus the `deps_ready` snapshot keep explicit
  allowlists with rationale — `cargo clippy --all-targets --features
  thread-safe` is green and gated in the flake and Ports CI.
- The `thread-safe` Rust build swaps `RefCell` for `std::sync::Mutex`
  (non-reentrant). **Never hold a core borrow across another core borrow** —
  nested same-thread locking deadlocks instead of panicking
  (`Fiber::name` did exactly this, pinned by
  `fiber_name_never_self_deadlocks` in `rust/tests/thread_safe.rs`). Audit
  every new borrow scope against this rule; a deadlock shows up as a hung
  test suite, not an error. The `once` scrutinee hardening (take the holder
  cell before disposing) is defense-in-depth: no current dispose path
  re-enters the cell, so no test can fail against the old shape by
  construction; `once_listener_may_dispose_itself_synchronously` pins the
  observable contract instead. The `deps_ready` allowlist comment cites
  Go's `resolveDeps` faithfully (Go locks only its lookup loop + retries
  via generation counter; Rust makes the snapshot atomic).
- Root disposal fires no `internal/plugin` (the root owns no runtime); its
  rollback cascades to root-scoped plugins, which fire their own disposal
  events — pinned by `root_dispose_emits_no_plugin_event`
  (`rust/tests/parity.rs`).
- `use BorrowExt as _` imports look unused in default builds (an autofix
  deleted one once) but are required under `thread-safe`, where the
  `Rc`/`RefCell` aliases become `Arc`/`Mutex`. Verify both feature
  variants before declaring any import dead.
- Rust test binaries that can block need `timeout N cargo test ...` locally;
  CI enforces `timeout-minutes` (15 Ports / 20 Build).
