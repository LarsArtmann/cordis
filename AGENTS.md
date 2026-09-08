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
- Rust: `cd rust && cargo test` (clippy clean, `cargo clippy --all-targets`)
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

- **Toolchain pins are load-bearing.** Root devDependencies must stay at
  `typescript ^5.9.3`, `vitest ^4.1.5`, `vite ^7.3.2`, `eslint ^8.57.1`,
  `esbuild ^0.28.0`. TypeScript 7 breaks yarn-berry's builtin compat patch
  (`lib/_tsc.js` missing) and there is NO yarn.lock, so every install
  re-resolves: a bad bump breaks `yarn install` for everyone, immediately.
  There is no yarn.lock by design (upstream does the same).
- **Test fixtures are string-coupled to the specs.** hmr specs mutate fixture
  sources via literal `content.replace("value = 'initial'", ...)`. Fork-style
  reformatting of `packages/hmr/tests/*` fixtures (yml + plugin `.ts` files)
  silently no-ops those replaces and every reload test times out. Keep hmr
  test fixtures byte-identical to upstream; style passes cover src/ and
  specs only.
- **Fork TS style == `prettier --print-width 100` (defaults otherwise).**
  Verified byte-idempotent across the whole formatted TS tree. Use
  `nix run nixpkgs#prettier -- --print-width 100 --write <files>`.
- **Stale `lib/` builds shadow src/.** Cross-package imports resolve through
  each package's `lib/index.js`. After changing TS sources run
  `nix develop -c yarn build` before debugging "impossible" test failures —
  the tests may still be running the old bundle.
- TS deps (chokidar, js-yaml, ...) must match upstream's versions; sed-style
  "bump everything" passes have twice introduced non-existent versions
  (js-yaml ^5.4.1) or API breaks.

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
build time via `zig/build.zig`). All three traces must be byte-identical.
Regenerate with `GOLDEN_UPDATE=1` on the Go runner and re-verify Rust and
Zig. Changing semantics? Fix the port, not the golden file. The loader has
no Rust/Zig port, so its watch/reload transcript is pinned by a Go-only
golden trace instead: `go/loader/watch_golden_test.go` +
`go/loader/testdata/watch-golden.txt` (same `GOLDEN_UPDATE=1` escape
hatch; do not move it into `golden/`, which is three-runner-only).

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
  `hmr.watch()`. Local TS suite is green (248/248); treat the next CI Build
  run as the confirmation, and gate on Ports until then.
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
- RESOLVED (2026-09-08): `yarn install` used to hard-crash with
  `typescript: ^7.0.2` (every TS 7 tarball ships only `bin/tsc`, no
  `lib/_tsc.js`, and yarn 4.14.1's builtin `compat/typescript` patch
  lstats that file). Upstream has since reverted to `^5.9.3`; the fork
  matches upstream's pins and installs cleanly. The fork's only manifest
  delta is a deliberate `@types/node ^26.5.0` bump (tsc-verified).
  TS dep bumps remain upstream decisions — re-check
  `git diff <upstream> -- '**/package.json'` after every sync.

## Repo hygiene facts

- Lint config lives in `.markdownlint.jsonc` (MD013/MD010-in-code-blocks
  off, keepachangelog duplicate headings allowed), `.markdownlintignore`
  (`docs/status/`, `docs/planning/` are frozen history;
  `packages/core/README.md` is upstream-frozen; `node_modules/` is vendored),
  `.oxlintrc.json` (`ignorePatterns` skips generated `lib/`/`dist/`), and
  `.buildflow.yml`. `dprint.json` excludes `packages/**` — upstream owns
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
  `thread-safe` feature still carries pre-existing `significant_drop`
  nursery findings — clippy is only gated on default features; both
  feature variants' test suites are green.
- The `thread-safe` Rust build swaps `RefCell` for `std::sync::Mutex`
  (non-reentrant). **Never hold a core borrow across another core borrow** —
  nested same-thread locking deadlocks instead of panicking
  (`Fiber::name` did exactly this). Audit every new borrow scope against
  this rule; a deadlock shows up as a hung test suite, not an error.
- `use BorrowExt as _` imports look unused in default builds (an autofix
  deleted one once) but are required under `thread-safe`, where the
  `Rc`/`RefCell` aliases become `Arc`/`Mutex`. Verify both feature
  variants before declaring any import dead.
- Rust test binaries that can block need `timeout N cargo test ...` locally;
  CI enforces `timeout-minutes` (15 Ports / 20 Build).
