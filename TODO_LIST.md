# TODO List

Short- and mid-term actionable tasks. Long-term direction and open decisions
live in `ROADMAP.md`; completed work is logged in `CHANGELOG.md`, never here.

**Prime directive: native-max APIs, not TS 1:1 ports.**

## Upstream tracking

Nothing pending — the pin is `f8ea3cd` (bumped 2026-09-09 with its rc.10
workspace manifests adopted byte-for-byte), the manifest divergence is back
to the single deliberate `@types/node ^26.5.0` delta, and CI is green on the
set (the 2026-09-08 bump-and-restyle divergence turned out to break the dts
build under TypeScript 7 and was reverted; see AGENTS.md → TS workspace
gotchas).

## Parity

Nothing pending — Zig runs all four golden scenarios (cascade runner landed
2026-09-08 on top of its typed registry).

## Rust follow-ups

Nothing pending — the `deps_ready` allowlist comment now carries a verified
`go/fiber.go` citation, the `once` scrutinee hardening and the `Fiber::name`
one-lock shape are pinned in `rust/tests/thread_safe.rs`, root-dispose's
no-`internal/plugin` contract is pinned in `rust/tests/parity.rs`, and the
crate docs narrate the internal events and the root guard (all landed
2026-09-08).

## Repo guards and small fixes

Nothing pending (2026-09-09): the hmr fixture/spec replace-literal canary
(`scripts/hmr-fixture-canary.mjs`) and the manifest divergence guard
(`scripts/manifest-parity.mjs`) are wired into the `upstream-parity` CI job,
`UPSTREAM_PIN` lives in the tracked one-line `.github/UPSTREAM_PIN` file the
workflow reads, and the `package.json` exclusion is a documented, guarded
decision (allowlist: root `@types/node` only).

Earlier: the markdownlint gate (flake `markdown` check + `test-markdown`
app) and GitHub Release pages for `go/v0.1.0` and `rust/v0.2.0` landed
2026-09-08.

## User-gated

Standing decisions (yarn.lock policy, oxlint policy for upstream TS,
one-session-per-worktree, TS toolchain stance, coverage/bench gate policy,
generic-method deprecation timeline, release surface) live in
`ROADMAP.md` → Open decisions — not duplicated here.
