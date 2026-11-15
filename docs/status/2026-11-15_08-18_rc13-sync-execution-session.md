# Status Report: rc.13 Sync & Port-Parity Program — Execution Session

**Date:** 2026-11-15 08:18 CET
**Session:** execution of
`docs/planning/2026-11-14_16-41_upstream-rc13-sync-port-parity-program.md`
(T01–T27, F01–F109), driven by the standing "READ-UNDERSTAND-EXECUTE-VERIFY"
directive. Prior assessment:
`docs/status/2026-11-14_16-31_upstream-rc10-rc13-assessment.md`.
**Concurrent session active** the whole time (their commits: `0c42ee9`;
their uncommitted files: `PORTS.md`, `TODO_LIST.md`,
`docs/status/2026-11-15_07-22_todo-list-execution-sweep.md` — never touched
by me).

**Local commits this session (NOT pushed; origin/main behind by 10):**

| Commit | Content |
| --- | --- |
| `e19481b` | T01: D1–D5 ratified in ROADMAP Open decisions |
| `910684f` | T03–T05: upstream rc.13 sync (packages/ → 68c9d99, yarn.lock, pin, guards green) |
| `0b864af` | T06/T07/T16: TS mechanics CI + buildflow + AGENTS.md facts |
| `d95516a` | T08: Go #242 mid-dispatch disposal |
| `b6c0e48` | T09: Rust #242 mid-dispatch disposal |
| `a22cfaf` | T11–T14 (Go): parity battery + #205 parallel-error label |
| `6edba3b` | T10 + Zig battery: #242 + #196 settle + battery (rewritten commit, see d) |
| `ecdd1e7` | T13: Go #134 internal/config resolution on every reload |

**Second stretch (same day) — session close:**

| Commit | Content |
| --- | --- |
| `bdf294f` | T11–T14 (Rust): battery + #205 label + #196 settle_pending port fix |
| `ca59fcd` | T17: oxlint convergence (.oxlintrc.json deleted), vitest-test skip policy |
| `49d9b46` | T18/T19/T20: hmr D3 memo, registry.replace rejection memo, loader review |
| `b06de59` | T21: DOMAIN_LANGUAGE component note, ROADMAP rc.13 matrix rows, README |
| `cb6d144` | #206 Go: intercept layers merge outer→inner, inject configs underneath |

**Worktree at close:** my remaining files (ROADMAP watch-branches note +
#134 rationale, AGENTS.md lessons + coverage baselines, the upstream
issue drafts, this report update) land in the final close-out commit.
The concurrent session's files (PORTS.md, TODO_LIST.md, their 07-22
report) remain theirs, untouched.

---

## a) FULLY DONE

1. **T01 — Decision gates ratified (defaults, user silent):** D1 keep
   `Plugin` naming; D2 adopt yarn.lock; D3 defer hmr re-port; D4 keep
   `@types/node` delta (now `^26.6.6`); D5 converge oxlint. Recorded in
   ROADMAP §Open decisions + new dated subsection. Worktree rule ratified
   as convention.
2. **T02 — Guard audit:** manifest-parity (HEAD-based, post-commit gate,
   allowlist needs NO widening), fixture-canary (file-name-agnostic but
   flat-dir assumption), CI upstream-parity job (byte + prettier semantic
   + manifest + canary + dprint guard), file-list diff (115 A / 25 D /
   71 M / 13 R in packages/), root-file delta map.
3. **T03–T05 — The sync:** `git rm -r packages` + `restore
   --source=68c9d99` (exact tree equality, zero merge risk); root
   `package.json` adopted then `@types/node ^26.6.6` reapplied (verified:
   the ONLY manifest delta vs pin is that one line); `yarn.lock` adopted
   (install re-resolves exactly @types/node + undici-types); `.gitignore`
   merged (un-ignore lock, `*.tsbuildinfo`, `.agents`, drop
   `.eslintcache`, deliberately NOT ignoring `AGENTS.md`); eslint files
   dropped, `oxlint.config.ts` + new tsconfigs taken; `utils` remnant
   trashed; inventory = 16 packages = pin exactly. Canary fixed honestly
   (recursive walk for the new `tests/fixtures/` layout) — 11 literals in
   14 fixtures. All four guards green locally (manifest guard verified
   post-commit). Smoke `yarn install` green incl. yakumo postinstall.
4. **Surprise found and handled:** commits `c610a16`/`e72e743` had
   pre-bumped fork manifests to TS 7 / vite 8 / vitest 5 / eslint 10 —
   OVERSHOOTING upstream rc.13 (still TS 5.9.3 / vite 7 / vitest 4,
   eslint dropped). The manifest guard was RED (30 divergences) at
   session start. The sync supersedes that experiment; documented in the
   commit message + AGENTS.md.
5. **T06 — TS baseline:** clean-tree two-step build green (`yarn build
   core` then `yarn build`). Full suite: **926 passed / 13 skipped / 939
   total, 0 failed** (was 248/248 on rc.10). First run had 17 failures —
   all environmental: `npm_config_cache=/mnt/buildcache/npm` (machine env
   pin) on a 100% full mount; override `npm_config_cache=/tmp/npm-cache`
   fixes it. Documented in AGENTS.md.
6. **T07 — Mechanics:** `.buildflow.yml` eslint-fix skip rationale now
   states the oxlint reality; AGENTS.md rewritten facts (working set
   re-verified on rc.13, lock policy flipped to "tracked", canary layout,
   pin history + rc.13 contents, TS 7 supersession, suite baseline, npm
   cache gotcha).
7. **T16 — CI (#337):** build job gains `yarn tsc -b packages/*/tests`
   (verified green locally first); installs move `yarn --no-immutable` →
   `yarn` (immutable now that the lock is tracked — catches lock drift).
8. **T08–T10 — #242 in all three ports:**
   - Go (`d95516a`): `removed atomic.Bool` tombstone set in
     `removeHookLocked` before splice (atomic because dispatch reads are
     lock-free; `-race` clean); all five modes skip; waterfall advances
     past removed hooks; 8 tests incl. snapshot-deferral and the
     mechanism itself.
   - Rust (`b6c0e48`): `removed: AtomicBool` (works behind Rc and Arc —
     matches existing `once` precedent, no cfg); single splice path sets
     it; all modes + waterfall live-iterator skip; 7 parity + 2
     thread-safe tests; clippy pedantic+nursery clean both variants;
     red-check confirmed tests fail without the fix.
   - Zig (`6edba3b`): hooks now arena-allocated `*Hook` pointers (value
     copies could never observe a tombstone); removal sets flag before
     identity-splice; both dispatch loops + waterfall skip;
     `error.OutOfMemory` threaded (panic allowlist unchanged at 17).
9. **T11/T12 (Go + Zig) — Parity battery GREEN:** #56 (internal/update
   never accumulates — Go by construction, test pins it), #133 (pending
   dispose settles DISPOSED), #209 (isolation boundary stops the injected
   view), #191 (re-provide after isolated provider unload succeeds),
   #196 (provider resources released only after dependent teardown — Go
   already had it via provide-disposer settle, pinned by ordering test;
   **Zig was genuinely RED**: dependents settled only in the outer drain
   loop → fixed by extracting `Core.settlePending` and calling it inside
   the provide cleanup, mirroring Go).
10. **T13 — #134 (Go):** new `EventConfig = "internal/config"` waterfall;
    `fiber.load` resolves config through it on EVERY load (start, update,
    dep return); zero-listener path returns raw config (golden traces
    untouched — verified by full suite); `internal/commit` deliberately
    not ported (Go loader has its own EntryChange commit model) —
    documented divergence.
11. **T14 (partial) — #205:** Go `Parallel` wraps `errors.Join` with
    `one or more listeners of event %q failed: %w` (golden decoupled,
    existing assertions substring-based). Rust label edit written (see b).
    #136 (emit mislabeled 'parallel') verified already-correct in Go by
    construction. #296 (DisposableList.unshift) = N/A for all ports
    (no such structure; Prepend exists) — PORTS.md note pending.
12. **T15 — event-name stability:** upstream kept `'internal/plugin'`
    through the #307 component rename; all four trees use it verbatim.
    Cross-language contract intact.

## b) PARTIALLY DONE

1. **Rust parity battery (T11/T12 Rust):** 6 tests written into
   `rust/tests/parity.rs` (#56, #133, #209, #191, #196, #205) + the #205
   label in `src/events.rs` — **currently does NOT compile**: two `E0308`
   (plugin/inject bodies return `Result<Disposer>` where `Result<()>` is
   expected — missing `?` after `attach`/`provide_named`) and two `E0373`
   (the `probe` closure borrows itself into `plugin(...)` — needs a fn or
   restructure). Uncommitted. Not yet run on either feature variant, no
   clippy, no thread-safe pass.
2. **T17 full gate sweep:** only per-language test runs so far. Missing:
   `nix run .#test` ×3 via flake, `nix flake check`, `nix develop -c
   buildflow`, golden byte-identity re-verification, guards re-run on the
   final tree.
3. **T22 harvest:** ROADMAP D1–D5 landed, but TODO_LIST errata fixes
   (stale upstream-tracking line, deny.toml line) and the plan/report
   harvest are NOT done — TODO_LIST.md is concurrently being edited by
   the other session.
4. **Battery coverage by language:** Go 6/6 scenarios (+ #134). Zig 5/5
   (+ #196 fix). Rust 0/6 committed.

## c) NOT STARTED

- T18: hmr rework deep-read (#244/#269/#282/#298), divergence memo, D3
  confirmation, #264 default re-check.
- T19: `registry.replace` (#266) evaluation + Go prototype/rejection memo.
- T20: loader parity review (#329/#271/#267/#239/#210/#246/#241/#179) +
  reconciliation with the concurrent session's `0c42ee9` loader work
  (entry_options/entry_lifecycle) + watch-golden re-run.
- T21: docs wave (DOMAIN_LANGUAGE rename entries, ROADMAP parity matrix
  rows, PORTS.md notes, README table).
- T23–T27: backlog batches (several items may already be done by the
  concurrent session's `0c42ee9`: greeter examples, golden #5/#6, miri
  CI — needs verification, then harvest).
- Push of the 10 local commits (policy: only on explicit request).
- Rust/Zig #134 equivalents (deliberate open question, see g).

## d) TOTALLY FUCKED UP (and what came of it)

1. **`f68cce2` shipped broken Zig code.** My gate run raced the still-live
   first Zig subagent: I tested a complete working-tree state, the agent
   then overwrote the dispatch loops with `// stripped` placeholders, and
   my `git add` captured THAT. Its own #242 tests failed against the
   commit. Caught via the second Zig agent's report; confirmed by
   `git show`; fixed by rewriting the local-only commit into `6edba3b`
   (complete code + battery + #196 fix, honest message). No broken state
   remains in history; nothing was pushed.
2. **Subagent orchestration half-failed:** two agents died on provider
   rate limits (`chuck-zig-242`, `carlos-rust-battery` — retry-exhausted,
   no output). I compensated by direct verification (Zig) and by writing
   the Rust battery myself — but compensation for Zig came AFTER the bad
   commit (see 1).
3. **My first Rust battery pass doesn't compile** (details in b) — wrote
   six tests without a compile loop in between; the errors are mechanical
   (`?` on `attach`/`provide_named`, closure borrow), not conceptual.
4. Minor: `git status --porcelain --cached` typo in a preview command
   (harmless, no effect on the commit — verified). A truncated-and-
   rewritten test block in `go/events_test.go` left draft cruft once
   (cleaned before commit).

## e) WHAT TO IMPROVE

1. **Commit-time verification protocol (the real lesson):** gates must
   run against what is STAGED/COMMITTED, not the worktree: after
   `git add`, spot-check `git diff --cached` (or re-run the gate
   post-commit, as I did for the sync commit and should have done for
   f68cce2). Never commit a file scope a live subagent still owns.
2. **Compile/write loop discipline:** build after every few lines of
   generated Rust, not after 200.
3. **Rate-limit resilience:** when an agent dies on rate limits, either
   take the work in-session immediately (as with Rust) or retry with
   backoff; don't assume its scope is finished.
4. **Record the race lesson in AGENTS.md** (worktree ownership for
   subagents + commit-time gate re-run) — pending in the docs wave.
5. Free space on `/mnt/buildcache` (system-level, 208G/220G) or repin
   `npm_config_cache` — it will keep breaking package-manager tests.

## f) NEXT (up to 50, priority order)

| # | Item |
| --- | --- |
| 1 | ~~Fix Rust battery compile errors (`?` on attach/provide bodies; restructure `probe` closure)~~ done at `bdf294f` |
| 2 | ~~`cargo test` + `--features thread-safe`, clippy both variants → green~~ done at `bdf294f` |
| 3 | ~~Commit Rust battery + #205 label~~ done at `bdf294f` (with the #196 settle_pending fix it uncovered) |
| 4 | ~~T17: `nix run .#test-go` / `.#test-rust` / `.#test-zig`~~ done, all green 2026-11-15 |
| 5 | ~~T17: `nix flake check`~~ done, all checks passed 2026-11-15 |
| 6 | ~~T17: `nix develop -c buildflow`~~ done — 99/100 green; vitest-test skipped with rationale (yarn-test owner), docs-gates cleared at session close |
| 7 | ~~T17: golden byte-identity ×3 ports~~ done (six scenarios, all three suites green) |
| 8 | ~~Re-run all four parity guards on the final tree~~ done (manifest, canary, byte, semantic all green) |
| 9 | ~~Verify `yarn lint` (oxlint) passes~~ done at `ca59fcd` (legacy `.oxlintrc.json` deleted; upstream config owns) |
| 10 | ~~Verify example smoke (`node examples/typescript/greeter.mjs`) still green~~ done |
| 11 | ~~T18: read #244/#269 diffs, write hmr divergence memo, confirm D3~~ done at `49d9b46` (memo: hmr-rc13-divergence) |
| 12 | ~~T18: re-check #264 defaults vs Go hmr~~ done — no Go surface (no fs watcher); watch-golden untouched and green |
| 13 | ~~T19: `registry.replace` read + Go prototype/rejection memo~~ done at `49d9b46` (rejected, memo) |
| 14 | ~~T20: map 8 loader fixes onto go/loader; reconcile with 0c42ee9~~ done at `49d9b46` (loader-parity-review) |
| 15 | ~~T20: re-run loader watch-golden~~ done (green in the sweep) |
| 16 | ~~T21: DOMAIN_LANGUAGE: component rename + Plugin-kept divergence entries~~ done at `b06de59` |
| 17 | ~~T21: ROADMAP parity matrix rows~~ done at `b06de59` (+ #242/#196 rows) |
| 18 | T21: PORTS.md notes (#205, #296 N/A, #134 no internal/commit, Zig #205 N/A) — still blocked on the concurrent session's uncommitted PORTS.md edits; divergence content lives in ROADMAP matrix + memos meanwhile |
| 19 | ~~T21: README port table refresh~~ done at `b06de59` (six golden scenarios, rc.13 pin) |
| 20 | T22: harvest plan + 16-31 report + this report into TODO_LIST/ROADMAP — partially done: ROADMAP updated (matrix, watch-branches, resolved open item 1); TODO_LIST rows are the concurrent session's uncommitted file, left to them |
| 22 | ~~T22: fix TODO_LIST errata (upstream-tracking, deny.toml)~~ done — already resolved in the concurrent session's working tree (deny.toml committed f6bf182; job-count verdict recorded) |
| 23 | ~~T22: coverage re-baseline into AGENTS.md after sweep~~ done — Go 90.3% total / 88.3–91.0 per pkg; Rust 85.7% lines / 84.1% regions (llvm_23 recipe pinned) |
| 24 | ~~T22: ROADMAP: upstream watch-branches note~~ done (`perf/traceable-underscore`, `refactor/migrate-server`) |
| 25 | ~~AGENTS.md: commit-time-gate + subagent-scope lessons (e.1/e.3)~~ done (Program-execution lessons bullet in Repo hygiene) |
| 26 | ~~Verify 0c42ee9's greeter/golden-#5#6/miri items~~ done (greeter smoke green; golden #5/#6 in all three suites; miri job present in ports.yml, CI will execute) |
| 27 | ~~#206 intercept metadata merge: verify + pin test (Go)~~ done at `cb6d144` (implemented: layered merge + 3 tests, race-clean) |
| 28 | #296/#136/#205 divergences documented per port in PORTS.md — same blocker as item 18; content in ROADMAP matrix + loader memo meanwhile |
| 29 | ~~Decide + implement (or reject) Rust/Zig #134 internal/config hook~~ decided: rejected for now (no loader consumer in Rust/Zig; rationale in ROADMAP matrix note) |
| 30 | ~~T23: panic-free follow-ups~~ done — closed 2026-11-14 by the concurrent session (TODO_LIST documents the evidence) |
| 31 | ~~T24: verify deny.toml devShell story; workflow job-count review~~ done (cargo-deny green in buildflow; verdicts in TODO_LIST) |
| 32 | ~~T25: `scripts/vulnix-audit.sh direct` re-run~~ done 2026-11-15: 23 derivations, 3 flagged, all known false-positive classes (MediaWiki-Cargo collision spot-checked against NVD; ShellCheck/Go version-range errors); nothing actionable, vulnix stays out of CI |
| 33 | ~~T25: `yarn npm audit` on the adopted lockfile~~ done 2026-11-15: 5 advisories, all dev-only toolchain (vite 7.3.6 + launch-editor, Windows dev-server surface) in upstream-adopted lock bytes; accept + revisit at next upstream sync |
| 34 | T27: upstream issue — clean single-step build root-cause — drafted (docs/planning/2026-11-15_upstream-issue-drafts.md, draft 1); filing is user-gated |
| 35 | T27: TS 7 minimal reproducer issue draft — drafted (same file, draft 2); filing is user-gated |
| 36 | Lock hygiene: `yarn dedupe --check` — decided NO: the lockfile is byte-parity-adopted upstream bytes; dedupe belongs upstream (would fork the lock) |
| 37 | devShell `npm_config_cache` override — decided NO for the shared flake: machine-specific workaround; fix the machine (repin the full /mnt/buildcache npm cache) instead |
| 38 | Push the local commits (user gate) — 16 commits now awaiting the push decision |
| 39 | CI green verification on push (F62) — blocked on item 38 |
| 40 | One-session-per-worktree guard tooling — not built; the discipline is recorded in AGENTS.md lessons instead |

## g) QUESTIONS (cannot be answered from the repo)

1. **Push timing:** origin/main is 10 commits behind local (e19481b →
   ecdd1e7, plus the pending Rust battery commit). Push now, or after
   T17's full gate sweep? (I would sweep first — but pushing is your
   call.)
2. **yarn.lock delta policy:** the deliberate `@types/node ^26.6.6`
   divergence permanently forks our lock away from upstream's bytes
   (re-resolution churn on every sync). Keep the delta (current state,
   D4 as ratified), or drop it to gain byte-identical lock syncs with
   upstream?
3. **Concurrent session's uncommitted docs:** `PORTS.md`/`TODO_LIST.md`
   edits + the 07-22 report are theirs and in flight. Should my T21/T22
   docs wave fold those files in (after re-reading their final state), or
   strictly leave them to that session and only touch DOMAIN_LANGUAGE/
   ROADMAP/README for now?

---

*Snapshot; living state moves to TODO_LIST.md/ROADMAP.md at T22.*
