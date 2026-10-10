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

Flake apps: `nix run .#test` (everything), `.#test-go`, `.#test-rust`,
`.#test-zig`. Directly:

- Go: `cd go && go test ./...` (also `go vet`; `-race` clean; coverage
  91.4% total, every package 88.8–91.7%, 2026-09-10 panic-free sweep).
  Requires Go 1.27 (gotcha below). Timer tests run in a `testing/synctest`
  bubble (virtual clock, ~2 ms, deterministic); write timing tests that
  way, not `time.Sleep`. The repo root is a minimal Go module (`go.mod` +
  `doc.go` stub, no buildable code) so repo-root Go tooling (`go
  generate/test/vet ./...`) resolves; do not delete it or put port code
  in it — the port lives in the nested `go/` module.
- Rust: `cd rust && cargo test` (clippy clean on both feature variants,
  `cargo clippy --all-targets [--features thread-safe]`, gated in the
  flake check/apps and ports.yml). Coverage baseline 86.7% lines / 86.1%
  regions (cargo-llvm-cov 0.9.0, 2026-09-10; code files 82–91%, `sync.rs`
  cfg plumbing, `lib.rs` doc-only) — reproduce from `rust/` with `nix
  shell nixpkgs#cargo-llvm-cov nixpkgs#llvm -c sh -c 'export
  LLVM_COV=$(command -v llvm-cov) LLVM_PROFDATA=$(command -v
  llvm-profdata); cargo llvm-cov --summary-only'`. Benchmarks: `cargo bench` (`benches/core.rs`,
  mirrors the six `go/bench_test.go` hot paths, dependency-free harness,
  best-of-five).
- Zig: `cd zig && zig build test` (leak-checked via testing.allocator);
  `zig build docs` must also pass — the doc-emission gate is wired into
  the flake's zig check next to `zig fmt --check`.

### Environment gotcha (this machine)

`GOCACHE` defaults to `/mnt/buildcache/go-build`; `~/.cache/go-build` is a
dangling home-manager symlink — both broken, always run Go with
`export GOCACHE=/tmp/gocache` (flake apps do this). System `go` is 1.26.x
with `GOTOOLCHAIN=local`, refusing the module's `go 1.27` directive: use
the flake devShell (pins `go_1_27`) or `nix shell nixpkgs#go_1_27 -c sh -c
'export GOCACHE=/tmp/gocache; cd go && go test ./...'`. Zig is not global
(`nix run nixpkgs#zig` or the devShell, which also ships `gcc` for
`go test -race`/cgo and resolves `GOCACHE` via `shellHook` — an attr using
`builtins.getEnv "HOME"` breaks under pure eval). `~/go/bin/govalid` is
rebuilt with Go 1.27; the system-profile copy is Go 1.26 and warns about
x/tools skew.

## TS workspace gotchas (learned in the 2026-09-07 upstream rebase)

- **Toolchain pins are load-bearing.** VERIFIED WORKING SET (2026-09-09,
  from-scratch install + CI two-step build + 248/248 suite): manifests are
  the upstream pin's bytes; the fork's ONLY delta is root
  `@types/node ^26.5.0` — yarn 4.14.1, TypeScript ^5.9.3, vitest ^4.1.5,
  vite ^7.3.2, esbuild ^0.28.0, eslint ^8.57.1 (all upstream values),
  enforced by the `upstream-parity` CI job via `scripts/manifest-parity.mjs`
  (manifest guard, in-script allowlist). **TypeScript 7 is fatal, twice
  confirmed** (2026-09-08): 7.0.2 compiles tests (vitest never typechecks)
  but the dts build dies with `TS2665: Module 'cordis' resolves to an
  untyped module at lib/index.js` — the cross-package
  `declare module 'cordis'` augmentation does not resolve under TS 7's
  resolver; three red CI runs. Stay on upstream's yarn 4.14.1 (4.18.0 was
  only for the TS 7 experiment; see the saga below). js-yaml 5 stays
  fatal for the include build (`yaml.Type` gone). NO yarn.lock
  by design (upstream does the same): every install re-resolves, so a bad
  bump breaks `yarn install` for everyone immediately.
- **Single-step `yarn build` from a clean tree fails; the CI two-step
  sequence is green.** From fully clean state, one `yarn build`
  deterministically reports ~35 dts errors (`@Inject('loader')` loses the
  loader augmentation) in hmr/include — a yakumo-tsc project-reference
  quirk, reproduced on node 24 and 26 with identical dependency sets.
  `yarn build core` then `yarn build` is green, as is any build after a
  prior successful one. Try the two-step sequence before debugging
  "impossible" clean-build failures; trash `packages/*/lib` +
  `packages/*/tsconfig.tsbuildinfo` before verifying toolchain changes.
- **Test fixtures are string-coupled to the specs.** hmr specs mutate
  fixture sources via literal `content.replace("value = 'initial'", ...)`;
  fork-style reformatting of `packages/hmr/tests/*` fixtures (yml + plugin
  `.ts`) silently no-ops those replaces and every reload test times out.
  ALL fixtures (hmr + include, yml and plugin `.ts`) are byte-identical to
  upstream, CI-guarded by the `upstream-parity` byte check plus
  `scripts/hmr-fixture-canary.mjs` fast-fail (every spec replace literal
  must exist in a fixture; same CI job) — milliseconds instead of ~190 s
  of waitFor timeouts. Its matching is any-fixture: it cannot pin a
  literal to one fixture when several share it.
- **packages/** style == upstream's, enforced by CI.** `upstream-parity`
  pins the last-synced upstream commit (`UPSTREAM_PIN`, a one-line tracked
  file at `.github/UPSTREAM_PIN` so pin bumps review as one-line diffs):
  non-TS files byte-identical to the pin; TS/JS may differ only by
  prettier-normalizable formatting; `package.json` files excluded from
  both guards BY DESIGN (manifests are the deliberate divergence surface),
  gated by the manifest guard instead. Bump `UPSTREAM_PIN` and sync
  manifests in the same commit as every `packages/**` sync. Prettier is
  NOT a style gate for packages (upstream style is not prettier-stable: 64
  files fail `--check` under every plausible config); the fork's old
  `prettier --print-width 100` formatting was reverted to upstream style
  in `0542b6d`. CI pins prettier 3.9.6 (`npx prettier@3.9.6`), the version
  nixpkgs shipped 2026-09-08 — keep the two in lockstep.
- **Stale `lib/` builds shadow src/.** Cross-package imports resolve
  through each package's `lib/index.js`; after changing TS sources run
  `nix develop -c yarn build` before debugging "impossible" test failures.
- Bump-everything protocol (three incidents, 2026-09-08): (1) check the
  npm registry before calling a version fake — chokidar 5, vitest 5,
  eslint 10, js-yaml 5 all exist; (2) verify each major empirically with a
  from-scratch install AND the CI two-step build — vite 8 breaks the
  suite, eslint 10 breaks `yarn lint` (.eslintrc removal), js-yaml 5
  breaks the include build, TS 7 breaks the dts build; (3) fixture bytes
  are string-coupled to specs — reformat them and every reload test times
  out.

## Zig 0.16 std gotchas (all verified against 0.16.0 on 2026-09-08)

- **`std.fs.Dir` is gone; it is `std.Io.Dir` now.** The filesystem API
  moved into `std.Io` (`std.Io.Dir`, `std.Io.File`); `std.fs` keeps only
  `path` plus a few constants (`std.fs.cwd()` → `std.Io.Dir.cwd()`).
- **Initialize ArrayLists with `.empty`.** `items`/`capacity` have no
  field defaults (`.{}` fails: "missing struct field: items");
  `std.ArrayListUnmanaged` is an alias of `std.ArrayList`; lists are
  unmanaged — methods take the allocator (`list.append(gpa, x)`).
- **Non-zig `@import` is an error and `@embedFile` cannot leave the module
  root** ("no module named 'data.txt'" / "embed of file outside package
  path"). To embed files outside the module (e.g. `../golden/`), copy them
  into the build cache with `b.addWriteFiles()` and reference them from a
  generated `.zig` shim using `@embedFile` — the `golden_data` pattern in
  `zig/build.zig`.
- **`zig build -femit-docs` does not exist** on the 0.16 build runner. Add
  a `docs` step: `Compile.getEmittedDocs()` (a `Compile` method, not
  `Module`) piped through `b.addInstallDirectory` — `InstallDir.Options`
  takes `source_dir`, not `source`.
- **`orderedRemove` poisons vacated slots with `undefined` (0xAA in
  Debug).** Iterating a slice captured before a remove reads the poison
  and panics. Snapshot-then-mutate: `Registry.delete` copies the fiber ids
  out and drops the registry entry before disposing (same order as Go's
  `Registry.Delete` and Rust's `delete_id`).

## Port architecture (all three languages share this)

**Prime directive (user, 2026-08-22): use each language's native features
to the max. Do NOT port TypeScript 1:1.** Semantics parity (fiber
lifecycle, drain queue, realms, rollback) is the constraint; API surfaces
must be native: type-keyed services and typed events (Go generics / Rust
TypeId / Zig comptime), stdlib `context.Context` per fiber in Go, slog
integration, RAII disposers in Rust, comptime plugin construction in Zig.
Intentional divergences from TS behavior are documented in ROADMAP.md.

1. **Drain queue instead of microtasks.** Public API calls are wrapped in
   `core.enter()`/`core.leave()`; fiber state transitions are queued and
   coalesced, then executed when the outermost call returns. Replaces the
   TS microtask queue; torn intermediate states are unobservable.
   `Context.Batch` (Go) groups multiple mutations into one transaction.
2. **No locks/borrows during user callbacks.** All framework state lives
   in one `core` guarded by a single mutex (Go) or RefCell (Rust);
   listeners, plugin bodies and cleanups always run unlocked, snapshotting
   state under the lock first. Never hold core.mu while calling user code.
3. **Effect tree.** Every registration (listener, service, nested plugin,
   raw cleanup) is a labeled item in a dispose bag; effect bodies collect
   into a child bag via `ctx.collect` (Go: goroutine-safe via a derived
   context, not fiber-global state). Disposal is LIFO everywhere.
4. **Realm keyed services.** Services are stored by a uint64 realm key,
   not by name. `Isolate` maps a name to a fresh key in the child scope;
   lookups walk the parent chain, falling back to the root realm (lazily
   assigned). Shared labels map name+label to one synthetic key.
5. **Fiber state machine.** pending/loading/active/failed/disposed/
   unloading, driven by `transition()` from the drain queue. Dependency
   resolution snapshots a generation counter and retries when the store
   changed mid-resolution (Go).
6. **Plugin identity.** Go: the `*Plugin[C]` value (typed config via
   generics). Rust: process-global atomic id. Zig: the plugin's address.

## Semantics deliberately adapted from TS

- Go drains synchronously: after `Start()` returns, an applicable plugin
  has already run (TS: microtask). Cleanups are synchronous `func()`;
  async cleanup is the user's goroutine concern. `Emit` panics propagate
  (Go); `Parallel` joins errors (Go errors.Join).
- Panic-free surface (2026-09-10): Waterfall misuse is a compile error
  (typed terminal parameter), `Isolate` reports uncomparable config labels
  as an error, Rust `FnPlugin.inject` returns `Error::PluginShared`, Zig
  threads `error.OutOfMemory` through all fallible registrations; a
  reachability audit closed every path from a fallible public API to a Zig
  `@panic` (`sharedKey`/`rootKey`/`queue`/`bindCleanup`/`realmFilter`
  fallible; the error log drops its line instead of aborting). Remaining
  panics are contract-bound, pinned by
  `scripts/panic-allowlist.sh`: typed event guards run inside listener
  wrappers (no error channel in any port's callback contract), 17 Zig
  dispatch/void-query aborts keep `cordis: out of memory in dispatch`
  (details in ROADMAP.md).
- Plugin apply errors move the fiber to `StateFailed`, roll back partial
  effects, and are routed to the logger — never thrown across the
  framework boundary.
- Sibling notification order is deterministic (fiber creation order) in
  all three ports; Go sorts by uid where map iteration would be random.

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
  / `emit::<E>`; string forms `*_named`; `FnPlugin` closures via
  `plugin()` + `start_fn`; trait plugins via `impl Plugin` (associated
  `Config`) + `start` (registry id `plugin_type_id::<P>()`); RAII `Guard`
  (dispose on drop, `detach()` to keep).
- Zig: `provide(ptr)` / `getTyped(T)` / `onTyped(E, data, f)` /
  `emitTyped(E, &event)` keyed by `@typeName`; string forms `*Named`;
  comptime plugins via `TypedPlugin(name, Config, apply, inject)` (the
  returned TYPE is the registry identity); runtime `Plugin` values for
  dynamic cases (optional `data: ?*const anyopaque` context);
  `Context.attach(data, f)` registers plain cleanups. Fallibility: domain
  errors are `Error{InactiveEffect, DuplicateService, PluginFailed,
  OutOfMemory}` — scope constructors (`extend`, `isolate`,
  `isolateShared`, `withFilter`) and fallible registrations return them;
  the abort/allowlist boundary is pinned by `scripts/panic-allowlist.sh`
  (see the panic-free-surface bullet above).

Golden scenarios (four: lifecycle `scenario.txt`, events
`scenario-events.txt`, cascade `scenario-cascade.txt`, dispatch
`scenario-dispatch.txt`) run in `go/golden_test.go`,
`rust/tests/golden.rs` and `zig/tests/golden.zig` (Zig embeds them via
`zig/build.zig`); all three ports run all four byte-identically, and the
Zig cascade runner shares the lifecycle op interpreter
(`runLifecycleScenario`). Regenerate with `GOLDEN_UPDATE=1` on the Go
runner, then re-verify Rust and Zig. Changing semantics? Fix the port,
not the golden file. The loader has no Rust/Zig port: its watch/reload
transcript is pinned by a Go-only golden trace,
`go/loader/watch_golden_test.go` + `go/loader/testdata/watch-golden.txt`
(same `GOLDEN_UPDATE=1` escape hatch; do not move it into `golden/`).

**Untracked files are invisible to `nix flake check`:** flakes in a git
repo only copy git-tracked files into the store — `git add` new files or
flake checks cannot see them (cost one "NotFound in sandbox" round trip,
2026-09-08).

## Upstream facts

- The fork tracks `main` of
  [cordiverse/cordis](https://github.com/cordiverse/cordis) exactly inside
  `packages/`; TS dep bumps belong upstream (after touching TS manifests,
  check `git diff <upstream> -- '**/package.json'`).
- Upstream commits **no yarn.lock**, installs with `yarn --no-immutable`
  (floating ranges resolved on every run). Local TS reproduction:
  `nix shell nixpkgs#nodejs_24 nixpkgs#corepack -c sh -c 'export
  PATH=/tmp/corepack-bin:$PATH; yarn install && yarn build'`. Never commit
  `yarn.lock` / `node_modules/`.
- Upstream's `packages/hmr` suite used to flake (11 waitFor timeouts —
  upstream cache-invalidation bug, fixed on their unmerged `3-stage-hmr`
  branch). The fork replayed that branch (`b4650df`): commit-based loader
  entry changes, atomic include writes, `hmr.watch()`. Local suite green
  (248/248); Build and Ports CI green on `3da7d0f` (2026-09-08, incl.
  flake + upstream-parity jobs).
- The root `README.md` is fork-owned (user demand, 2026-09-07; replaced
  upstream's symlink to `packages/core/README.md`). Never recreate the
  symlink; never put fork content into `packages/**/README.md` —
  `packages/core/README.md` stays byte-identical to upstream.
- Rebase onto upstream `caab04e` completed 2026-09-08 (all 32 fork commits
  replayed). Upstream switched TS style to double quotes + semicolons,
  prettier-ish wrapping; upstream's delta at the conflict points was
  format-only, so fork TS content is authoritative for semantics — future
  conflicts re-apply fork deltas onto upstream blobs. `hmr.watch()` (#128)
  was already in the fork base. 2026-09-09: pin bumped to `f8ea3cd`
  (rc.10); its entire delta vs `caab04e` is the eight workspace manifests,
  adopted byte-for-byte.
- TS 7 saga, RESOLVED final (2026-09-09): two independent breakages. (1)
  `yarn install` hard-crashed with `typescript: ^7.0.2` under yarn 4.14.1
  (TS 7 tarballs ship only `bin/tsc`, no `lib/_tsc.js`, which yarn's
  builtin `compat/typescript` patch lstats) — the 2026-09-08 "fix" was
  yarn 4.18.0, needed only to make TS 7 installable. (2) TS 7 then broke
  the dts build (TS2665, toolchain bullet) — invisible locally: vitest
  never typechecks and the clean-build quirk masked it. Final state:
  upstream's exact manifests, only
  `@types/node ^26.5.0` diverges (machine-guarded), `.yarnrc.yml` back to
  upstream's two lines. Never re-ship TS 7 without a from-scratch install
  plus the CI two-step build.

## Buildflow (quality gate on this machine)

- Run `nix develop -c buildflow`: the per-module Go fan-out needs a
  working `go` matching `go/go.mod` (1.27); a stale toolchain makes it
  skip module discovery and run every Go step at the repo root ("cannot
  find main module"). No per-step workdir knob exists (verified against
  `buildflow config init`) — the root Go module stub exists for this.
  Nix steps system-filter enumerated checks, so the old aarch64-darwin
  "platform mismatch" cascade is fixed upstream; `nix flake check` is
  the port gate. `.go-structure-linter.yaml` suppresses `agent-config`
  (deliberate ~376-line AGENTS.md; expires 2027-01-06).
- skip_steps policy (rationale lives in `.buildflow.yml`):
  `go-auto-upgrade` (wants samber/lo — go.mod is deliberately
  zero-dependency — and GOEXPERIMENT-gated json/v2 whose own finding
  warns of runtime-only failure; port is parity-locked); `eslint-fix`
  (upstream pins ESLint 8 + `.eslintrc`, no flat config; TS linting is
  `yarn lint`); `type-check` (root tsc lacks the workspace
  node_modules; `yarn build` owns dts); cargo steps re-enabled 2026-10-10
  (buildflow now routes them to the `rust/` manifest); `exclude` adds
  `zig-out` + `result` (generated read-only files crash formatters —
  oxfmt Permission denied). `markdown-lint` is skipped in `full` mode,
  so the flake `markdown` check is the only markdownlint gate — and it
  needs STRICT JSON: oxfmt re-adds trailing commas (broke the gate
  2026-10-08; guarded by `.oxfmtrc.json`, dprint is comma-neutral).
- `.golangci.yml` (root, v2: standard set + gofmt/goimports) added
  2026-10-08 for buildflow's go-structure-linter; golangci-lint discovers
  config in parent dirs, so it also governs `go/` (`golangci-lint run`
  clean there: 0 issues, 2026-10-08).
- erraudit (per-module, ~/go/bin, go 1.27 build) is green. Deliberate
  discards use `//nolint:erraudit // reason` (verified: suppresses);
  genuine findings get real fixes (`%w` wrapping, reporting close/rollback
  errors).

## Repo hygiene facts

- Lint configs: `.markdownlint.jsonc` (MD013 off, MD010-in-code-blocks
  off, keepachangelog duplicate headings allowed; strict JSON, no trailing
  commas — see Buildflow) + `.markdownlintignore` (`docs/status/`,
  `docs/planning/` frozen history; all of `packages/` upstream-owned,
  byte-parity-guarded; vendored/generated dirs); `.oxlintrc.json`
  (`ignorePatterns` skips generated `lib/`/`dist/`); `dprint.json`
  (excludes `packages/**` — upstream owns formatting there; never let a
  formatter rewrite upstream files); `.buildflow.yml`.
- Rust: `pedantic`+`nursery` are `deny` (Cargo.toml). All findings cleared
  under nixpkgs rustc 1.97 (2026-09-08); expect new nursery lints after
  toolchain bumps — fix code, don't weaken the config. The `thread-safe`
  feature's `significant_drop` findings cleared the same day: lock scopes
  tightened where semantics-preserving (`queue`, `once`, `get_named`,
  `restore`, the state-machine merges); three snapshot-consistency holds
  plus the `deps_ready` snapshot keep explicit allowlists with rationale —
  clippy both variants green, gated in the flake and Ports CI.
- The `thread-safe` Rust build swaps `RefCell` for `std::sync::Mutex`
  (non-reentrant). **Never hold a core borrow across another core borrow**
  — nested same-thread locking deadlocks instead of panicking
  (`Fiber::name` did exactly this, pinned by
  `fiber_name_never_self_deadlocks` in `rust/tests/thread_safe.rs`); a
  deadlock is a hung test suite, not an error. The `once` scrutinee
  hardening is defense-in-depth (no current dispose path re-enters the
  cell, so no test can fail against the old shape by construction);
  `once_listener_may_dispose_itself_synchronously` pins the observable
  contract. The `deps_ready` allowlist cites Go's `resolveDeps` faithfully
  (Go locks only its lookup loop + generation-counter retries; Rust makes
  the snapshot atomic).
- Root disposal fires no `internal/plugin` (the root owns no runtime); its
  rollback cascades to root-scoped plugins, which fire their own disposal
  events — pinned by `root_dispose_emits_no_plugin_event`
  (`rust/tests/parity.rs`).
- `use BorrowExt as _` imports look unused in default builds (an autofix
  deleted one once) but are required under `thread-safe`, where the
  `Rc`/`RefCell` aliases become `Arc`/`Mutex`. Verify both feature
  variants before declaring any import dead.
- Rust test binaries that can block need `timeout N cargo test ...`
  locally; CI enforces `timeout-minutes` (15 Ports / 20 Build).
- vulnix environment audit: `scripts/vulnix-audit.sh [direct|closure]`
  scans the flake's own derivations (checks + devShell + formatter; apps
  are a subset) against the NVD — the scoped answer to buildflow's noisy
  whole-store runs. Reviewed 2026-09-10
  (`docs/status/2026-09-10_09-08_vulnix-environment-audit.md`): direct
  scope flags 4 packages / 11 CVEs, only coreutils-9.11 (uniq/unexpand, no
  fixed release yet) genuine; closure flags 32 derivations / 120 CVEs,
  ~60% false positives from NVD product-name collisions (MediaWiki
  "Cargo" vs cargo, ecies/go vs the Go toolchain, ...). Nothing actionable
  in-repo: keep vulnix out of CI (NVD fetch + noise), re-run `direct`
  after nixpkgs bumps. Never act on a vulnix finding without opening the
  NVD entry; `vulnix --no-requisites` on a runCommand .drv inspects
  nothing — the scoped recipe extracts the drvs' `inputDrvs` first.
