# TODO List

Short- and mid-term actionable tasks. Long-term direction and open decisions
live in `ROADMAP.md`; completed work is logged in `CHANGELOG.md`, never here.

**Prime directive: native-max APIs, not TS 1:1 ports.**

## Upstream tracking

The 2026-11-14 read-only assessment
(`docs/status/2026-11-14_16-31_upstream-rc10-rc13-assessment.md`) found
upstream at rc.13 (77 commits, +21.5k lines) with one verified port gap
(#242 mid-dispatch listener removal in all three ports). The sync itself is
user-gated on three policy questions (plugin → component rename, yarn.lock
adoption, hmr re-port scope — report section (g)); until it lands, do not
touch `packages/`.

- [ ] Watch upstream `perf/traceable-underscore` and
      `refactor/migrate-server` branches for the next delta (report
      2026-11-14_16-31 §f47). — Low / S / Upstream
- [ ] Parity test: no injected metadata for a fiber's own provisions
      (#209 second half) — write the test against current port semantics
      when the rc.13 sync lands, or earlier if a session wants the
      coverage now (report 2026-11-14_16-31 §f15). — Medium / S / Parity
- [ ] Working tree awaits the commit instruction (loader label
      validation + entry split, Zig OOM sweep + three real fixes, Rust
      FiberId/Arg, greeter examples, CI workflows, doc pass), then watch
      Ports CI (2026-11-14 session).

## Panic-free surface follow-ups

Closed 2026-11-14 (evidence in CHANGELOG [Unreleased]): the typed-event
guard error-channel decision (ROADMAP.md → "Typed-event guard error
channel" — keep the contract-bound panic; a Result-listener variant is
the only sanctioned escape hatch, deferred to the breaking-surface
release window), the Zig OOM sweep (`zig/tests/oom.zig`, the 0.17
`std.testing` failure-allocator harness sweeping every registration
allocation index for the fallible public APIs; three real bugs
fixed: init logger leak, `provideNamed` abort, hooks poison entry), the
per-site `// dispatch: no error channel` markers (all 17 sites), loader
isolate-label validation with a field-precise error, Rust `FiberId`
encapsulation, `cargo bench` re-run (no perf regression), the
fallibility doc pass (PORTS/README/FEATURES), and the AGENTS
rebase-checklist note for the Go `Waterfall`/`Isolate` divergences. The
Zig dispatch-abort end state stays user-gated in ROADMAP.

## Rust follow-ups

Nothing pending — the miri job joined Ports CI (golden/typed/parity
under `-Zmiri-ignore-leaks -Zmiri-disable-isolation`, verified locally
2026-11-14: typed 4.9 s, golden 6.3 s) and the logger `Arg`
`From<u32>`/`From<usize>` conversions landed with a parity test. The
`deps_ready` allowlist citation, the `once` scrutinee hardening, the
`Fiber::name` one-lock shape, the root-dispose contract and the crate
docs were already landed (2026-09-08).

## Repo guards and small fixes

Nothing pending — `rust/deny.toml` is committed (`f6bf182`) and
`cargo-audit`/`cargo-deny` now ship in the flake devShell (2026-11-14),
so buildflow stops using the `nix run` fallback for them. The
workflow job-count review verdict (2026-11-14): the `flake` job
deliberately duplicates the go/rust/zig jobs with a hermetic
`-race -count=3` profile — fast feedback vs hermetic gate, keep both;
the zig job now pins 0.17.1 (tracking the nixpkgs zig the flake check
uses; the port needs 0.17-only APIs) and runs the fmt gate. erraudit
re-verified 2026-11-14: 0 violations across `go/...` after the loader
changes. The 2026-09-09_01-54 leftovers were dispositioned by the
docs-gates audit-all replay (0 unharvested historical items outside the
rc.13 assessment, which is routed above).

## Environment audit

Re-ran `scripts/vulnix-audit.sh direct` after the 2026-11-09 nixpkgs
bump (2026-11-14): 9 findings across 3 packages, down from 4 packages /
11 CVEs. The four tracked targets cleared exactly as predicted —
nixpkgs now ships jq 1.8.2, perl 5.42.3 and libmicrohttpd 1.0.10, and
zlib left the closure entirely. Verified verdicts (2026-11-14, NVD +
upstream advisories): jq's HIGH trio is CVE-2026-49839 / CVE-2026-40164
/ CVE-2026-32316, all fixed in jq 1.8.2 — the IDs named in the
2026-09-28 report were wrong (CVE-2025-2810 is Draeger ICMHelper,
CVE-2026-21734 is Imagination DDK, CVE-2026-21735 does not exist); perl
CVE-2026-57432 is real and fixed in 5.40.5/5.42.3; libmicrohttpd
CVE-2025-62689 is real, affected ≤ 1.0.2, fixed at commit level /
1.0.3; CVE-2023-6992 affects the CLOUDFLARE zlib fork — not zlib-ng and
not vanilla zlib, so the 2026-09-28 "zlib-ng mismatch" suspicion was
wrong. `yarn npm audit` (2026-11-14): 7 findings, all in upstream-owned
manifests — vite 7.3.6 (1 high launch-editor, 2 moderate, 1 low),
esbuild 0.27.7 (1 low), eslint 8.57.1 deprecation (1 moderate); TS dep
bumps belong upstream, nothing to do in the fork. The residual
direct-scope findings (ShellCheck, cargo 1.99.1, go 1.27.3) are the
same NVD product-name collision class behind the old audit's "~60% of
findings were NVD false positives" wording — that estimate belonged to
the whole-store closure scope; the direct scope's residuals are the
same class — open the NVD entry before acting on any of them.

- [ ] Cold-cache smoke test of `scripts/vulnix-audit.sh` (NVD download
      path; the warm path is verified 2026-11-14). — S/M
- [ ] Bucket the residual direct-scope findings: ShellCheck
      CVE-2021-28794 vs ShellCheck 0.11.0 (vulnix range says affected;
      open the entry before acting — likely a stale range), the 7
      cargo-1.99.1 CVEs (expected product-name collision class), and
      go-1.27.3 CVE-2023-49292 (4.9; verify the range against 1.27.3). —
      Low / S

## Structure

- [ ] Split the remaining over-350-line files when the next semantic
      work touches them (go/fiber.go 759, go/coverage_test.go 660,
      zig/src/cordis.zig — one comptime module by design — and the
      table-heavy test files). go/loader/entry.go is done: split into
      entry.go / entry_lifecycle.go / entry_options.go (165/211/165
      lines) on 2026-11-14 with the golden traces untouched.
      buildflow's `file-size-check` step is skipped until then
      (.buildflow.yml). — Medium / M

## Quality backlog

- [ ] Golden scenario: status/plugin event ops in the shared DSL runner
      (byte-identical across ports; report 2026-09-10_08-25 §14 — a
      different "v2" idea than the snapshot/restore ops that landed as
      #5). — Low / M

Closed 2026-11-14: `nix run .#test` and `nix develop -c buildflow` ran
end-to-end green (the full local CI mirror, f#20), erraudit stamped at
0 violations; the logger golden scenario — the report's planned
"fifth golden run", landed as the #6 run in all three ports
(`golden/scenario-logger.txt`) — and the snapshot/restore golden v2
(#5, Rust + Zig — the Go port has no snapshot API; FEATURES.md
corrected). The loader's `Tree.recordError` last-writer-wins docs plus
the sequential + concurrent-reload pin tests also landed. The FEATURES
"Planned" section no longer lists bounded work.

- [ ] On each nixpkgs/flake bump, re-check `nix flake check` output for
      new nix deprecations (clean on the 2026-11-14 run; formatter
      naming fixed at `51cddf2`). — Low / S / Nix
- [ ] Go: godoc polish pass (every exported symbol has an
      example-quality comment; report 2026-08-22_10-19 §30). — Low / M /
      Docs
- [ ] `govulncheck ./...` one-off for the Go port (source-level reachability,
      complements vulnix's binary-closure view; report
      2026-09-10_09-28 §12). — Low / S

Closed 2026-11-14: the greeter example app in all four trees with CI
smoke steps; the gopls infertypeargs cleanups
(`go/coverage_test.go`, `go/accessor.go`); the nightly multi-seed run
for the debounce/throttle property; the Zig `restart`/`update` event
cross-links; the loader `recordError` last-writer-wins docs + pin
tests. The LSP environment row is closed as stale: system Go is 1.27.3
and gopls/golangci diagnostics verified working on this machine
(2026-11-14) — the original root cause was the broken GOCACHE, repaired
the same day; no wrapper needed.

## Cross-repo

- [ ] (repo: cordiverse/cordis) Root-cause the single-step clean
      `yarn build` failure (bisect atsc/dtsc/tsconfig-utils in a
      sandbox) and file it upstream (report
      2026-09-09_02-51 §8; quirk documented in AGENTS.md). — Medium / M
- [ ] (repo: cordiverse/cordis) Draft the TS 7 TS265/TS2665
      augmentation issue upstream — after building a minimal reproducer
      (verify-before-filing; report 2026-09-09_02-51 §9). — Medium / S
- [ ] (repo: crush-config) Propose the oxfmt/strict-JSONC trap to
      `references/lessons.md` (needs a commit there; reports
      2026-10-09_17-58 §29, 2026-10-09_20-25 §40 partial). — Medium / S
- [ ] (repo: crush-config) Propose fleet default `.oxfmtrc.json`
      ignoring strict-JSON `*.jsonc` (report
      2026-10-09_20-25 §41). — Medium / S
- [ ] (repo: LarsArtmann/BuildFlow) `type-check` resolves only the FIRST
      tsconfig dir (`tools/providers/tsconfig_check.go:25` via
      `firstMatchDir`), so this 16-package workspace gets one package
      checked and a false green; verified 2026-11-15 by re-enabling the
      step (it runs in `packages/cli-cordis` only). Ask for per-module
      tsc fan-out, the shape #42 gave cargo. — Medium / S
- [ ] (repo: LarsArtmann/BuildFlow) `vitest-test` runs bare
      `vitest run` (`tools/providers/js_providers.go:116`) with no way to
      select the repo's test script or package manager; verified
      2026-11-15: 13 failures (5x yarn-not-in-PATH, 7 hmr waitFor
      timeouts, one lib/-vs-src stack) where `yarn test` is green. —
      Medium / M
- [ ] (repo: LarsArtmann/BuildFlow) Low priority: tools removed by
      `skip_steps` are also listed as "language mismatch (project: …)"
      in the not-applicable summary (`execution/filtered_tools.go:246`),
      contradicting the skip line next to them; same class as closed
      #43/#37. — Low / S

## User-gated

Standing decisions (yarn.lock policy, oxlint policy for upstream TS,
one-session-per-worktree, TS toolchain stance, coverage/bench gate
policy, generic-method deprecation timeline, release surface, the
breaking-surface release policy, the Zig dispatch-abort end state, the
Go `Must*` fate, and the rc.13 sync's three policy questions) live in
`ROADMAP.md` → Open decisions — not duplicated here.
