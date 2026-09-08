# Status Report — 2026-09-09 01:54 CEST

Session: single session, 2026-09-08 ~22:15 → 2026-09-09 ~01:55.
Branch `main` at `2ac1be1`; **39 files modified in the working tree, uncommitted** (no commit instruction given; the verified fix-forward for the toolchain lives only in this tree until committed).

This report covers the TODO_LIST execution sweep requested at session start, the mid-session collision with a concurrent writer, and the TS toolchain upgrade executed on the user's explicit "can't you upgrade?!?!" directive.

Format note: the status-report skill's canonical output is a styled HTML dashboard; the user explicitly requested `.md`, so this report is Markdown by user override.

---

## a) FULLY DONE

| # | Work | Evidence | Scope |
|---|------|----------|-------|
| 1 | Zig cascade golden runner (#3 of 4 scenarios now runs in all three ports) | `zig build test` 33/33, `zig fmt --check` clean, `zig build docs` gate green; cascade trace matched `golden/expected-cascade.txt` byte-identically on first run | `zig/build.zig` (embed 2 files), `zig/tests/golden.zig` (extracted `runLifecycleScenario`/`expectGoldenTrace` helpers + new test) |
| 2 | Parity docs flipped to every-port-runs-all-4 | `golden/README.md` matrix Zig ✅, FEATURES row `FULLY_FUNCTIONAL`, README "one spec, three runners", PORTS.md, AGENTS.md | 5 living docs |
| 3 | `deps_ready` allowlist comment corrected with a verified citation | Read `go/fiber.go:309-336` (`resolveDeps` locks only the store-lookup loop, snapshots states) and `:352-362` (generation-counter re-check); old claim "Go holds its mutex across the same loop" was false | `rust/src/fiber.rs` |
| 4 | Root-dispose `internal/plugin` contract pinned | `root_dispose_emits_no_plugin_event` in `rust/tests/parity.rs`; parity 31/31 under **both** feature variants | `rust/tests/parity.rs` |
| 5 | `once` scrutinee hardening pinned + honestly classified as defense-in-depth | `once_listener_may_dispose_itself_synchronously` (`rust/tests/thread_safe.rs`, Mutex build); AGENTS.md records that no failing test can exist by construction (no dispose path re-enters the holder cell) | `rust/tests/thread_safe.rs`, AGENTS.md |
| 6 | `Fiber::name` nested-borrow deadlock (d9cc834) pinned | `fiber_name_never_self_deadlocks`: 4 threads × 100 concurrent `name()` calls under `--features thread-safe` | `rust/tests/thread_safe.rs` |
| 7 | Rust crate docs: internal-events + root-guard narrative; stale "single-threaded, thread-safe on the roadmap" claim fixed | `cargo doc --no-deps` clean; `rust/src/lib.rs` | `rust/src/lib.rs` |
| 8 | markdownlint became a real gate | `.markdownlint.jsonc` trailing commas fixed (config did not parse before), flake `markdown` check + `test-markdown` app + composite `test`; `.markdownlintignore` now excludes `packages/` (upstream-owned), `target/`, `zig-out/`; fixed the 2 real violations (FEATURES MD060, table alignment) | flake.nix, 2 lint files, FEATURES.md |
| 9 | GitHub Release pages created | [go/v0.1.0](https://github.com/LarsArtmann/cordis/releases/tag/go/v0.1.0), [rust/v0.2.0](https://github.com/LarsArtmann/cordis/releases/tag/rust/v0.2.0) — notes derived from each tag's actual tree | remote only |
| 10 | TS toolchain upgrade, **empirically verified end-to-end** | final manifest: yarn **4.18.0**, typescript **^7.0.2**, vitest **^5.0.0**, vite **^7.3.2**, eslint **^8.57.1**, esbuild ^0.28.2, js-yaml ^4.1.0, chokidar ^5, @babel/code-frame ^8 → `yarn install` ✓ (14 s), `yarn build` ✓, `yarn test` **248/248** ✓ (twice), `yarn lint` ✓ (exit 0), fixtures byte-identical to upstream | `package.json`, `packages/include/package.json`, `packages/hmr/tests/*.ts` + include fixtures (restored), `.yarnrc.yml` |
| 11 | Root-cause research for the upgrade | yarn#7190/#7216 (4.17.1 fixed TS 7 install), npm registry checks (chokidar 5.0.0 / vitest 5.0.0 / eslint 10.10.0 / @babel/code-frame 8.0.0 all exist), reproduce-then-fix for each breakage | AGENTS.md toolchain bullet |
| 12 | Living docs refreshed | TODO_LIST (3 sections harvested), AGENTS (toolchain working set, crash history ×2, bump-everything protocol, markdownlint gate, Rust pins), CHANGELOG (toolchain entry), FEATURES, README, PORTS, golden/README | 7 docs |

## b) PARTIALLY DONE

| Work | Works now | Open | Blocker | Effort |
|------|-----------|------|---------|--------|
| CI validation of the packages divergence | install/build/test/lint all green locally | the `upstream-parity` CI job has never seen the reformat + fixture restore; prettier-normalization equality of reformatted TS vs upstream is unverified (I did not replicate the pinned-prettier dual-tree diff locally) | nothing — needs a push | S |
| Upstream sync (TODO #1) | superseded by the divergence upgrade; upstream `f8ea3cd` (rc.10 version set) not merged | `UPSTREAM_PIN` still `caab04e`; sync decision now interacts with the deliberate manifest deltas | user decision on divergence strategy | M |
| Working-tree commit | all 39 files are the verified fix for main's broken install | main itself is still broken at HEAD for anyone who clones (`yarn install` crashes at `2ac1be1`) | no commit/push instruction (policy: only on explicit request) | S |
| eslint 10 adoption | pinned back to ^8.57.1, CI's `yarn lint` green | migrating the repo to eslint flat config (to unlock 10+) is upstream's harness decision | upstream territory | L |

## c) NOT STARTED

| Planned | Why not started | Still wanted? |
|---------|-----------------|---------------|
| Fixture/spec replace-literal canary (hmr) | deprioritized behind the collision + upgrade; the hazard it guards has now fired **twice** | yes — high |
| `UPSTREAM_PIN` → tracked one-line file | contested with the concurrent writer at the time | yes |
| `package.json` exclusion decision in upstream-parity | now bigger: the manifests carry real deltas, so the guard needs a delta-whitelist design, not just a doc line | yes — redesigned scope |
| Logger golden scenario; Zig snapshot/restore + status events + accessor/mixin + logger; Rust `internal/get\|set\|listener\|dispatch` interception | out of today's scope (FEATURES "Planned") | yes, unchanged |

## d) TOTALLY FUCKED UP

1. **`main` is broken at HEAD.** `2ac1be1` (authored by a concurrent process mid-session, 22:35) ships `typescript ^7.0.2` on yarn 4.14.1: `yarn install` hard-crashes (`ENOENT ... lib/_tsc.js`), so the suite is 0-runnable for anyone at that commit. Severity: blocks every clone/contributor until the working tree lands. Mitigation: this session's verified working tree (39 uncommitted files). Root cause: a bump-everything pass without gate runs — the third such incident in two days.
2. **My first root-dispose test asserted wrong semantics.** I claimed root dispose emits no `EVENT_PLUGIN` *and* leaves plugin fibers untouched; the test failed 2≠1 because root disposal cascades and root-scoped plugins fire their own disposal events. The port was right; my reading was wrong. Fixed by pinning the actual contract (events: start 1, cascade 2, root 0).
3. **I called bumped versions "likely nonexistent" from stale knowledge.** Registry checks proved chokidar 5, vitest 5, eslint 10, @babel 8, js-yaml 5.4.1 all exist. My AGENTS.md even carried the stale "non-existent js-yaml ^5.4.1" claim — corrected this session to the real lesson: registry-check, then verify each major empirically.
4. **I initially raced a live concurrent writer.** Three tool calls into packages/** state before the reflog + 60-second-fresh mtimes revealed an active session committing `2ac1be1` behind me. No damage done, but I should have run `git reflog` the moment the env snapshot ("M FEATURES.md") contradicted live status.
5. **eslint 10 shipped in my "verified" set untested** — until I checked CI's `yarn lint` at report time and found eslint 10 removes `.eslintrc` (the repo's config format). Confirmed failing under 10, green under 8; docs corrected. Lesson: "verified working set" only covers the gates you actually ran.
6. **Known-stale factual claims survived in AGENTS.md from earlier passes** (the "non-existent versions" bullet). Fixed now; the meta-lesson is that AGENTS claims need dates and re-verification triggers.

### What I forgot / would do better (self-critique)

- Forgot to check CI's actual steps (`yarn lint`) before calling the toolchain set "verified" — caught only during this report's evidence pass.
- Forgot to document the `.yarnrc.yml` `npmMinimalAgeGate: 0` artifact when it appeared — now documented (AGENTS toolchain bullet).
- Would check `git reflog` + mtime freshness before the first `git diff` whenever a session-start snapshot disagrees with live state.
- Would run the new goldens/tests with the adversarial expectation "the trace will diverge" rather than "pass on first try" — the one time I pre-verified semantics carefully (root dispose), I still got the contract wrong; test-first honesty beats prediction.
- Malformed `question` payloads (missing `type`) cost two round trips — schema care.

## e) WHAT WE SHOULD IMPROVE

1. **Concurrency protocol for this repo.** Two writers (sessions + the auto-commit daemon) touched the tree in one evening. Before any edit: fresh `git reflog -5` + `git status`, and treat <5-minute mtimes as "someone is live". Impact: avoided thrash; cost when skipped: rework risk. Concrete fix: add the check to AGENTS workflow loop (partially written today).
2. **Kill the bump-everything anti-pattern structurally.** Three incidents (js-yaml ^5, chokidar ^5 round 1, TS7 round 2). The replace-literal canary catches fixture churn; there is no equivalent guard for manifests. Fix: extend upstream-parity to diff manifests against upstream with an explicit, reviewed delta-whitelist (feeds TODO item below).
3. **"Verified working set" needs a gate checklist.** Write down which gates define "works" (install, build, test, lint, parity) and run them all before documenting a set — eslint slipped through exactly this gap.
4. **Self-review before asserting port semantics.** Two of my session's initial claims (Go mutex behavior; root-dispose event behavior) were wrong on first write and only testing/reading caught them. Keep the "verify against the primary source, then write the comment" order.
5. **Status reports:** user prefers `.md` here; the skill defaults to HTML. Either record the preference or expect the override every time.

## f) NEXT TASKS (up to 50, ranked; HARVEST fuel — route to TODO_LIST/ROADMAP)

| # | Task | Impact | Effort | Category |
|---|------|--------|--------|----------|
| 1 | Commit + push the 39-file working tree (fixes main's broken install; the fix exists only uncommitted) | Critical | S | Bug |
| 2 | Confirm `upstream-parity` job green on the reformat + fixture restore after push; investigate any prettier-normalization drift | Critical | S | Quality |
| 3 | Add the fixture/spec replace-literal canary test to `packages/hmr` (assert every spec `.replace()` literal exists in its fixture) | High | S | Quality |
| 4 | Extend upstream-parity to cover manifests with a reviewed delta-whitelist (yarn 4.18/TS 7/vitest 5/@types/node; vite/js-yaml/eslint stay at upstream pins) — replaces the vague "package.json exclusion" TODO | High | M | Quality |
| 5 | Decide divergence strategy: keep the TS toolchain delta permanently vs converge when upstream adopts TS 7; document in ROADMAP | High | S | Documentation |
| 6 | Move `UPSTREAM_PIN` into a tracked one-line file the workflow reads | Medium | S | Cleanup |
| 7 | Grep CI workflows for other load-bearing toolchain assumptions affected by the upgrade (node versions in the matrix vs vitest 5's `^22.12 \|\| ^24 \|\| >=26` engines) | High | S | Bug |
| 8 | Replicate the pinned-prettier (3.9.6) dual-tree normalization locally to pre-validate parity before pushing | Medium | M | Quality |
| 9 | Verify hmr works as a CJS consumer dependency (chokidar 5/@babel 8 are ESM-only; check `packages/hmr` dual-format exports) | High | M | Bug |
| 10 | Time-boxed spike: vitest 5.0.1+/vite 8.1 — retest the two vite-8 failure signatures (decorator SyntaxError, hmr waitFor) for a future re-upgrade | Low | M | Feature |
| 11 | PR the yarn#7190-based TS 7 enablement knowledge upstream (cordiverse pins 4.14.1 + TS 5.9.3) — verify-before-filing applies | Medium | M | Feature |
| 12 | Run docs-health ANNOTATE on `docs/status/2026-09-08_21-11`, `_21-18`, `_18-16`, `_04-04` — their follow-ups were resolved today (deps_ready citation, once pin, root-dispose test, Fiber::name test, zig cascade) | Medium | S | Documentation |
| 13 | HARVEST this report's items into TODO_LIST/ROADMAP | Medium | S | Documentation |
| 14 | Audit remaining `Fiber` accessors (`state()`, `uid()`, `effects()`, `context()`) for the nested core-borrow pattern d9cc834 fixed in `name()` | High | S | Bug |
| 15 | Re-run `cargo llvm-cov` baseline after the new Rust tests and update the recorded 86.4% figure | Low | S | Quality |
| 16 | Add `npmMinimalAgeGate` commentary decision: keep 0 locally only, or set a nonzero gate in CI for supply-chain safety | Low | S | Quality |
| 17 | Add a real-FS watch smoke test for hmr under chokidar 5 (suite covers reload, but v5 watcher semantics deserve one integration pass) | Medium | M | Quality |
| 18 | CHANGELOG "Releases" section: link the two new release pages | Low | S | Documentation |
| 19 | Decide whether markdownlint should also run in remote CI (currently flake-gate-only) | Low | S | Quality |
| 20 | Zig: golden assertion for `hasTyped`/`deleteTyped` typed identity (cascade exercises registry delete; add typed-registry coverage) | Medium | M | Feature |
| 21 | Zig: registry snapshot/restore, status events, accessor/mixin, logger (FEATURES Planned) | Medium | L | Feature |
| 22 | Rust: `internal/get\|set\|listener\|dispatch` interception parity (FEATURES Planned) | Medium | L | Feature |
| 23 | Logger golden scenario (FEATURES Planned; logger has no golden coverage) | Medium | M | Feature |
| 24 | Consider a debug_assertions re-entrancy detector for the `once` holder cell (turn defense-in-depth into a loud failure) | Low | M | Quality |
| 25 | Re-record cross-port bench baselines under one harness contract (the 21-11 report's own "could do better" item) | Low | L | Quality |
| 26 | extract the Zig op interpreter (`runLifecycleScenario`) into a reusable test util if the logger scenario lands | Low | S | Cleanup |
| 27 | Confirm build.yml's corepack flow honors `packageManager` 4.18.0 in the Node 26 leg (engines of vitest 5 allow it; node 24 leg fine) | Medium | S | Bug |
| 28 | Re-verify `@types/node ^26.5.0` delta under TS 7 tsc (was verified under 5.9; build passed, but note it in the delta list) | Low | S | Documentation |
| 29 | Add the "check reflog + mtimes before editing living docs" concurrency rule to AGENTS workflow loop permanently | Medium | S | Documentation |
| 30 | ROADMAP: record the two vite-8 failure signatures verbatim so the next upgrade attempt starts from evidence, not archaeology | Low | S | Documentation |

(30 items — the rest of the 50-slot budget is deliberately unfilled: filler items would be ROADMAP noise, and HARVEST must route hard.)

## g) QUESTIONS I CANNOT ANSWER MYSELF

1. **May I commit and push the 39-file working tree now?** Main is broken at HEAD (`yarn install` crashes at `2ac1be1`); the verified fix exists only locally. I need your explicit commit/push instruction — policy forbids me from deciding this alone. (Items #1–#4 above all unblock on this.)
2. **Should the fork keep the TS toolchain divergence permanently, or is it a stopgap until upstream adopts TS 7?** This decides whether I invest in divergence documentation + delta-whitelist guard (#5, #4) or in an upstream PR (#11).
3. **Do you want the no-yarn.lock policy revisited for CI only** (commit a lockfile, run `yarn install --immutable` in CI, keep local installs floating)? Three manifest incidents in two days all came from floating resolution; only you can change the standing policy.
