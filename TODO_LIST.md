# TODO List

Short- and mid-term actionable tasks. Long-term direction and open decisions
live in `ROADMAP.md`; completed work is logged in `CHANGELOG.md`, never here.

**Prime directive: native-max APIs, not TS 1:1 ports.**

## Upstream tracking

- [ ] Validate the deliberate manifest divergence through CI: the working
      tree now runs yarn 4.18.0 + TS ^7.0.2 + vitest ^5 (248/248 locally,
      2026-09-08) while upstream pins yarn 4.14.1 + TS ^5.9.3 + vitest ^4.
      The `upstream-parity` job has not seen the reformat + fixture restore
      yet — push, confirm it is green, and only then decide whether to
      sync `packages/**` to upstream `f8ea3cd` (rc.10 version set) and bump
      `UPSTREAM_PIN` (`.github/workflows/ports.yml:89`) in the same commit
      (source: docs/status/2026-09-08_21-18 §b2/§c1/§f2).

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

CONTESTED (2026-09-08, see commit `2ac1be1`): the three `packages/**` /
`ports.yml` items below are entangled with a separate session's
bump-and-restyle commit on `main` that contradicts the repo's documented
pins. Resolve that first, then pick these back up.

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

Done 2026-09-08: the markdownlint config is now a real gate (flake
`markdown` check + `test-markdown` app; trailing commas fixed,
`packages/` excluded), and GitHub Release pages exist for `go/v0.1.0` and
`rust/v0.2.0`.

## User-gated

Standing decisions (yarn.lock policy, oxlint policy for upstream TS,
one-session-per-worktree, TS toolchain stance, coverage/bench gate policy,
generic-method deprecation timeline, release surface) live in
`ROADMAP.md` → Open decisions — not duplicated here.
