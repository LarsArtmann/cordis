# TODO List

Short- and mid-term actionable tasks. Long-term direction and open decisions
live in `ROADMAP.md`; completed work is logged in `CHANGELOG.md`, never here.

**Prime directive: native-max APIs, not TS 1:1 ports.**

## Panic-free surface follow-ups (from the 2026-09-10 sweep)

Source: `docs/status/2026-09-10_06-32_panic-free-typed-errors-sweep.md`.
Already done this pass (dropped from the report's list): Zig reachability
audit (dispatch aborts 27 → 15), the panic-allowlist CI gate
(`scripts/panic-allowlist.sh`), the CHANGELOG `[Unreleased]` entry.

- [ ] Design an error channel for the typed-event guards (Go `go/typed.go`
      6 sites, Rust `rust/src/events.rs` 2 sites): envelope value or
      `Result`-listener variant, decided once for all three ports
      (report f#7).
- [ ] Zig: `std.testing.failure_allocator` tests asserting
      `error.OutOfMemory` from the fallible public APIs (`isolateShared`,
      `provideNamed`, `restart`, `update`, the `bindCleanup` paths)
      (report c6/f#8).
- [ ] Zig: `// dispatch: no error channel` markers on the remaining 15
      `@panic` sites in `zig/src/cordis.zig` (report f#9).
- [ ] Loader: validate `Isolate` label comparability at config-decode time
      with a field-precise error, before `buildContext` (report f#11).
- [ ] Rust: encapsulate `FiberId` (`pub` field → `pub(crate)` + accessor)
      so out-of-range indexing is unrepresentable outside the crate
      (`rust/src/core.rs`, report f#13).
- [ ] Re-run `cargo bench` (`rust/benches/core.rs`) to confirm no perf
      change from the arena `Option` removal and the Waterfall arg
      reordering (report f#18/f#19).
- [ ] Run `nix run .#test` and `nix develop -c buildflow` end-to-end to
      mirror the full local CI sequence on this machine (report f#20).
- [ ] Next upstream rebase checklist: note that Go `Waterfall`/`Isolate`
      signatures diverge from TS shapes by design (report f#22).
- [ ] One doc pass consolidating the per-port fallibility shapes (Go
      `(T, error)`, Rust `Result`, Zig `Error!T`) into PORTS.md,
      FEATURES.md and the fork README port table (report f#23/f#24/f#40/f#47).
- [ ] LSP environment fix: per-project gopls/golangci-lint wrapper pinned
      to the flake's Go 1.27; zero usable diagnostics on this machine all
      session (report c8/e8).
- [ ] Working tree awaits the commit instruction (28 swept files + status
      report + this pass's audit/gate/doc changes), then watch Ports CI
      (report f#2/f#21).

## Upstream tracking

Nothing pending — the pin is `f8ea3cd` (bumped 2026-09-09 with its rc.10
workspace manifests adopted byte-for-byte), the manifest divergence is back
to the single deliberate `@types/node ^26.5.0` delta, and CI is green on the
set (the 2026-09-08 bump-and-restyle divergence turned out to break the dts
build under TypeScript 7 and was reverted; see AGENTS.md → TS workspace
gotchas).

## Parity

The 2026-09-10 Rust/Zig catch-up landed: Rust effect introspection
(`attach_labeled`), logger service and `internal/get|set|listener|dispatch`
interception events; Zig intercept, snapshot/restore + stashing delete,
`internal/status`/`plugin`/`update` events, config validation
(`ValidatedPlugin`), logger service and comptime accessors/mixins.
Remaining Go-only surface: callable services + tracker, timer, loader,
hmr. Remaining Rust/Zig rows live in `ROADMAP.md` → Planned (loader/hmr
gated on the module-layout decision).

## Rust follow-ups

Nothing pending beyond the sweep items above — the `deps_ready` allowlist
comment now carries a verified `go/fiber.go` citation, the `once` scrutinee
hardening and the `Fiber::name` one-lock shape are pinned in
`rust/tests/thread_safe.rs`, root-dispose's no-`internal/plugin` contract is
pinned in `rust/tests/parity.rs`, and the crate docs narrate the internal
events and the root guard (all landed 2026-09-08).

## Repo guards and small fixes

Nothing pending beyond the sweep items above (2026-09-10): the panic
allowlist gate joined the fixture canary and the manifest guard; the hmr
fixture/spec replace-literal canary (`scripts/hmr-fixture-canary.mjs`) and
the manifest divergence guard (`scripts/manifest-parity.mjs`) are wired into
the `upstream-parity` CI job, `UPSTREAM_PIN` lives in the tracked one-line
`.github/UPSTREAM_PIN` file the workflow reads, and the `package.json`
exclusion is a documented, guarded decision (allowlist: root `@types/node`
only).

Earlier: the markdownlint gate (flake `markdown` check + `test-markdown`
app) and GitHub Release pages for `go/v0.1.0` and `rust/v0.2.0` landed
2026-09-08.

## Environment audit follow-up (from the 2026-09-10 vulnix run)

Source: `docs/status/2026-09-10_09-08_vulnix-environment-audit.md`.
No in-repo action was required: every genuine finding is latest-release
build/dev tooling whose fix lands with the next `nix flake update`
(~60% of findings were NVD product-name false positives).

- [ ] Re-run `scripts/vulnix-audit.sh direct` after the next nixpkgs
      bump; expect glibc 2.43, python 3.14.7, coreutils 9.12, gzip 1.15,
      binutils 2.47 to clear their entries (report §f1). — Low / S
- [ ] One-off dependency-side audits vulnix cannot see: `yarn npm
      audit` for the TS workspace (Go/Rust ports are dependency-free)
      (report §f2). — Low / S

## User-gated

Standing decisions (yarn.lock policy, oxlint policy for upstream TS,
one-session-per-worktree, TS toolchain stance, coverage/bench gate policy,
generic-method deprecation timeline, release surface) live in
`ROADMAP.md` → Open decisions — not duplicated here. New since the
2026-09-10 sweep: the breaking-surface release policy, the Zig
dispatch-abort end state, and the Go `Must*` fate (same file).
