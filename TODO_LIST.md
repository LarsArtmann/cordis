# TODO List

Short- and mid-term actionable tasks. Long-term direction and open decisions
live in `ROADMAP.md`; completed work is logged in `CHANGELOG.md`, never here.

**Prime directive: native-max APIs, not TS 1:1 ports.**

## Upstream tracking

- [ ] Sync `packages/**` to current upstream `main` (upstream moved past the
      `caab04e` pin: rc.10 version set, hmr src evolution), bump
      `UPSTREAM_PIN` (`.github/workflows/ports.yml:89`) in the same commit,
      run all gates (source: docs/status/2026-09-08_21-18 §b2/§c1/§f2).

## Parity

- [ ] Zig: cascade golden runner — wire `golden/scenario-cascade.txt` into
      `zig/build.zig` + `zig/tests/golden.zig` on top of the typed registry
      (`hasTyped`/`deleteTyped`, landed in `75fb408`); then flip the
      `golden/README.md` matrix cell and the FEATURES/README rows to
      FULLY_FUNCTIONAL (source: docs/status/2026-09-08_18-16 §b1/§f4 —
      prerequisite done, runner missing; golden/README matrix is the source
      of truth until then).

## Rust follow-ups (source: docs/status/2026-09-08_21-11 §f1–f5, §c6)

- [ ] Verify Go's `depsReady` locking against `go/fiber.go` and correct the
      `deps_ready` allowlist comment in `rust/src/` (or keep it with a
      verified citation).
- [ ] `once` scrutinee regression test under `--features thread-safe` that
      fails with the guard-spanning shape — or document the fix as
      defense-in-depth in the commit/AGENTS.
- [ ] Root-dispose-emits-no-`EVENT_PLUGIN` test (by design; currently pinned
      only by code reading).
- [ ] `EVENT_PLUGIN`/`EVENT_UPDATE` + root guard narrative in
      `rust/src/lib.rs` (crate docs stop at the `pub use` line).

## Repo guards and small fixes

- [ ] Fixture/spec replace-literal canary in `packages/hmr`: a tiny test
      asserting every spec `.replace()` literal exists in its fixture —
      catches the fixture-coupling hazard in milliseconds instead of ~190 s
      of waitFor timeouts (source: docs/status/2026-09-08_21-18 §f5).
- [ ] Move `UPSTREAM_PIN` from `ports.yml` YAML into a tracked one-line file
      the workflow reads, so pin bumps review as one-line diffs
      (source: docs/status/2026-09-08_21-18 §f6).
- [ ] Decide and document the `package.json` exclusion in `upstream-parity`
      — the one unguarded drift surface left (source:
      docs/status/2026-09-08_21-18 §f8).
- [ ] Wire `.markdownlint.jsonc` into a gate or delete it: nothing in
      buildflow, flake or CI invokes it (verified 2026-09-08), and it has
      trailing commas strict parsers reject (source:
      docs/status/2026-09-08_21-11 §c7/§f25).
- [ ] Thread-safe Rust regression test for the `Fiber::name` nested-borrow
      deadlock (fixed in `d9cc834`; `rust/tests/thread_safe.rs` has no
      pinning test; source: docs/status/2026-09-08_04-04 §f40).
- [ ] GitHub Release pages with notes for `go/v0.1.0` and `rust/v0.2.0`
      (tags exist, pages do not; source:
      docs/status/2026-09-05_03-03 §f3).

## User-gated

Standing decisions (yarn.lock policy, oxlint policy for upstream TS,
one-session-per-worktree, TS toolchain stance, coverage/bench gate policy,
generic-method deprecation timeline, release surface) live in
`ROADMAP.md` → Open decisions — not duplicated here.
