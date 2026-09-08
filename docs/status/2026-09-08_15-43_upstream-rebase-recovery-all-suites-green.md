# Status Report — Upstream Rebase Recovery, All Four Test Suites Green

**Date:** 2026-09-08 15:43 CEST
**Session scope:** Recovery of the interrupted `git sync` rebase (upstream
`caab04e`) reported in this session's paste, plus everything needed to get
every test suite back to green. Based solely on this session's run.
**Verdict:** Rebase fallout fully repaired. TS 248/248, Go (race) green,
Rust green, Zig 29/29. 24 files remain uncommitted. One process-level
failure: I raced a concurrent crush session for the whole session.

---

## Session timeline (what actually happened)

1. Session started mid-rebase: commit `5dfa504` (formatting/lint hardening)
   conflicted on 12 TS files across `hmr/`, `include/`, `loader/`.
2. I derived the correct resolution policy: fork-side changes to those files
   were **formatting-only** (verified via commit history + token-normalized
   diffs), so the right merge is **upstream semantics + fork formatting**,
   and proved fork style == `prettier --print-width 100` (byte-idempotent on
   49 formatted files).
3. While I was resolving, a **concurrent crush session** finished the rebase
   itself (I detected it via appearing/disappearing index state). Its
   resolution was partially wrong; it also committed its own dep-downgrade
   fix overlapping mine, plus status-report commits (`51cddf2`, `61ec9f9`,
   `4383ef8`, `eb831a7`).
4. I then repaired the fallout independently:
   - reverted broken/hallucinated dependency bumps in 6 `package.json` files,
   - restored upstream semantics in the 12 misresolved files (+ formatting),
   - rebuilt stale `lib/` bundles (`yarn build`),
   - root-caused the hmr timeouts (fixture string-coupling, see below),
   - restored hmr test fixtures to byte-identical upstream versions.
5. All suites green; AGENTS.md updated with the discovered gotchas.

---

## a) FULLY DONE

- **Rebase conflict resolution policy derived and validated.** Fork TS
  changes since the merge base are formatting-only → correct result is
  upstream semantics + fork formatting. Proven with history analysis
  (`git log 2ceea23..1c38a41` shows only the two style commits touched
  these files) and token-normalized diffing.
- **Fork TS style reverse-engineered exactly**: `prettier --print-width 100`
  (all other defaults). Verified byte-idempotent across the 49
  already-formatted TS files. Recorded in AGENTS.md.
- **TS toolchain repaired.** `yarn install` was broken repo-wide (root +
  5 packages had re-bumped deps: `typescript ^7.0.2` kills yarn-berry's
  builtin compat patch — `lib/_tsc.js` missing; plus non-existent versions
  like `js-yaml ^5.4.1`). Reverted to upstream's known-good pins
  (`typescript ^5.9.3`, `vitest ^4.1.5`, `eslint ^8.57.1`, `vite ^7.3.2`,
  `esbuild ^0.28.0`, `chokidar ^4.0.3`, `js-yaml ^4.1.x`, etc.). Install now
  succeeds (verified against a pure-upstream worktree baseline: same failure
  existed at pre-rebase tip `1c38a41`, i.e. pre-existing, not rebase
  fallout).
- **12 misresolved files repaired.** `loader/src/index.ts`,
  `loader/src/config/{entry,group,isolate,tree}.ts`, `include/src/index.ts`,
  `hmr/src/index.ts` + 5 test files + `loader/tests/{group.spec,utils}.ts`
  now carry upstream semantics (#111 three-stage reload, #121 include
  journal, #123 bare-specifier resolve, #128 `hmr.watch()`) in fork
  formatting.
- **Stale-build hazard cleared.** Cross-package imports resolve through each
  package's `lib/index.js`; the pre-fix bundles were still being executed by
  the tests. `yarn build` regenerated everything; gotcha recorded in
  AGENTS.md.
- **hmr timeout root cause found and fixed.** The specs mutate fixture
  sources with literal `content.replace("value = 'initial'", ...)`; the
  style pass had converted fixtures to double quotes, so the replaces
  no-oped and every reload test waited 8.5 s and timed out. All hmr test
  fixtures (`*.yml`, `plugin*.ts`, `dep*.ts`, `tree-dep.ts`) restored
  byte-identical to upstream. Verified by bisection in a passing
  pure-upstream worktree (our src + upstream fixtures = 43/43; our old
  fixtures = 12 failures).
- **All test suites green** (this session's final runs):
  - TS: **248/248 passed, 25 files** (`yarn test`, ~61 s)
  - Go: **all packages ok with `-race`** via `nix run .#test-go` (go 1.27
    from flake): root, group, hmr, loader, timer
  - Rust: **44 tests + 6 doctests ok** (`cargo test`)
  - Zig: **29/29** via `nix run .#test-zig`
- **Memory updated.** Project AGENTS.md gained a "TS workspace gotchas"
  section: toolchain pins are load-bearing; fixtures are string-coupled to
  specs; prettier-100 = fork style; stale `lib/` shadows `src/`.
- **Hygiene:** upstream worktree removed, tmp-* test debris and /tmp scratch
  dirs cleaned.

## b) PARTIALLY DONE

- **Rebase completion itself.** Finished by the concurrent agent, not me; I
  verified and repaired its output. Net history: 32+ fork commits on
  `caab04e`, local `main` diverged from `origin/main` (39 vs 31) — rebase
  not yet pushed.
- **The 24-file repair batch is uncommitted** (12 semantic restorations +
  hmr fixture byte-restores + AGENTS.md). The auto-commit daemon or another
  session may pick them up; no clean, well-described commit exists yet.
- **AGENTS.md memory update** written but also uncommitted (same batch).
- **Verification of the concurrent agent's four commits** (`51cddf2`,
  `61ec9f9`, `4383ef8`, `eb831a7`): I only verified that the end state
  tests green; I did not review their diffs line-by-line (e.g. the
  "port gates"/"Go 1.27 alignment" changes inside them).

## c) NOT STARTED

- Push / `git sync` completion (needs force-with-lease on diverged main —
  never done without explicit approval).
- Lint/format gates after my changes: `eslint`, `oxlint`, `dprint check`,
  `golangci-lint`, explicit `cargo clippy --all-targets`, `nix flake check`
  (the flake checks also run clippy+vet; I ran the flake *apps*, not the
  checks).
- Fork docs not updated for the newly-inherited upstream features:
  FEATURES.md (hmr.watch, include journal, loader bare specifiers,
  Windows/macOS CI), PORTS.md/ROADMAP.md parity matrix (Go/Rust/Zig parity
  gaps grew), CHANGELOG.md entry for the upstream sync + recovery.
- CI verification on GitHub (workflows changed during the rebase; only
  locally validated).
- Repo-hardening follow-ups (fixture byte-pin guard, prettier config file,
  yarn.lock decision) — see (f).

## d) TOTALLY FUCKED UP!

- **I raced a concurrent crush session for the entire session.** Multiple
  crush processes were live on this repo; the rebase finished underneath
  me, unmerged index entries appeared/vanished between my commands,
  conflict markers got committed and later removed by someone else, and my
  dep-fix landed in a commit the other agent authored (`eb831a7`). I
  detected it, adapted, and verified the end state — but I kept working in
  the same tree instead of stopping to coordinate. Real risk of lost or
  duplicated work; two of my diagnostic snapshots were garbage because the
  tree changed under me.
- **Two botched diagnostic setups.** My "semantic diff" harness was wrong
  twice (wrong `cp` layout → false "no differences"; `git show` outside a
  repo → false "yml differs"), and I once diffed unnormalized ours vs
  normalized theirs and briefly chased a style artifact (`Awaitable`
  import) as a semantic divergence. Each cost a diagnostic cycle and could
  have misdirected the fix.
- **Wasteful test runs.** I kicked off the full 150 s TS suite repeatedly
  (three overlapping background runs) before narrowing to the failing
  files; the hmr suite alone burns ~155 s per run and I ran it ~6 times.
  Should have isolated failing files + used `-t` filters from the start.
- **Left the tree dirty at session end** (24 files) while an auto-commit
  daemon and another active session exist — the exact racing hazard again.

## e) WHAT WE SHOULD IMPROVE!

- **One agent per working tree.** Enforce via git worktrees per session, or
  a session lock. This session's chaos was 100% self-inflicted concurrency.
- **Make the fixture coupling un-breakable.** Either a CI hash-pin that hmr
  fixtures stay byte-identical to upstream, or (better, upstreamable) make
  the specs' replaces style-agnostic (regex over quote styles). Silent
  no-op replaces producing 8.5 s timeouts is the worst possible failure
  mode.
- **Commit a `yarn.lock`.** Upstream has none, so every install re-resolves
  and a single bad bump breaks everyone instantly (this exact failure
  happened twice in fork history). Trade-off: lockfile noise during upstream
  syncs; worth deciding deliberately.
- **Check formatting in CI.** The fork's style exists only in tribal
  knowledge + AGENTS.md; a `.prettierrc` (printWidth 100) plus a
  `prettier --check` CI step would have caught the fixture regression
  before it landed.
- **`yarn build` before `yarn test` in CI/local docs.** Stale `lib/`
  shadowing cost a full diagnostic round and is non-obvious.
- **Diagnostic discipline.** Normalize *both* sides before diffing; verify a
  diff harness with a known-different file before trusting "no output";
  isolate failing specs before any full-suite run.

## f) Up to 50 things to get done next

**Immediate (this repair's loose ends):**
1. Commit the 24-file repair batch as one clean, well-described commit.
2. Decide/coordinate concurrent sessions before any further work (see Q1).
3. Review the concurrent agent's 4 commits (`51cddf2`…`eb831a7`) for
   correctness beyond "tests green".
4. Run `nix flake check` (runs go vet+race, cargo clippy+test, zig).
5. Run `golangci-lint` on `go/`.
6. Run explicit `cargo clippy --all-targets` (deny list).
7. Run `eslint` + `oxlint` over `packages/`.
8. Run `dprint check` (json/yaml/md).
9. Run `nix run .#test` (composite all-ports app) once, end to end.
10. Decide push: `git sync` / force-with-lease on diverged main (needs user
    approval — see Q2).
11. Verify CI green on GitHub after push (build.yml + ports.yml).

**Docs parity (fork docs are now stale vs the new TS baseline):**
12. FEATURES.md: add `hmr.watch()`, include journal reconciliation,
    loader bare-specifier resolution, Windows/macOS CI matrix.
13. ROADMAP.md parity matrix: re-assess Go/Rust/Zig gaps for #111/#121/
    #123/#128 features.
14. CHANGELOG.md: entry for the upstream sync + recovery batch.
15. PORTS.md: note the hmr three-stage reload semantics the ports must
    match (if not already covered).
16. Check the other agents' status reports (from `61ec9f9`, `4383ef8`,
    `eb831a7`) for claims that this session contradicts or supersedes;
    reconcile via docs-health ANNOTATE.

**Parity work triggered by the new upstream features:**
17. Go `hmr` package: evaluate `watch()` API parity.
18. Go: evaluate include/journal-equivalent behavior (or document the
    divergence in ROADMAP.md).
19. Go loader: bare-specifier project resolution parity (`resolve.mjs`
    analog).
20. Rust/Zig: same three assessments (probably ROADMAP entries only).
21. Golden scenarios: consider a scenario covering watch/reload semantics
    if ports implement them.

**Hardening (never again):**
22. CI hash-pin: hmr fixtures byte-identical to upstream (cheap guard).
23. Upstreamable PR: make hmr spec replaces style-agnostic regexes.
24. Add `.prettierrc` (printWidth 100) to the repo.
25. Add `prettier --check` + `yarn build` to build.yml before tests.
26. Decide yarn.lock policy (commit one vs stay lock-free like upstream).
27. Move include test tmp-* files to os.tmpdir (upstreamable; they litter
    the fixtures dir on failure).
28. Teach include tests to clean up tmp fixtures even on failure (afterEach
    already tries; the litter came from crashed runs).

**Small consistency items noticed:**
29. `packages/hmr/tsconfig.json`: trailing-newline diff vs upstream.
30. `packages/hmr/README.md`: fork dropped a stale line; re-check against
    upstream's new README content for further drift.
31. Verify `@types/node ^26.5.0` + TS 5.9.3 is a sound combo (tsc build
    passed, but a deliberate note in AGENTS.md would help).
32. Ensure `lib/`, `tsconfig.tsbuildinfo`, `tmp-*` are fully gitignored
    (they appeared as untracked/copyable debris during diagnosis).
33. Restore or formally drop `packages/hmr/node_modules` (I removed it to
    rule out esbuild shadowing; a fresh `yarn install` recreates it).
34. Check whether `.oxlintrc.json` rules pass on the newly adopted
    upstream-semantics files (they were never linted in fork config).
35. Confirm the `esbuild ^0.27.3` (hmr) vs `^0.28.0` (root) split is
    intentional upstream state, not an accident we preserved.

**Verification depth (quality bar):**
36. Go: `-count=5` flake sweep on hmr/loader packages (race-sensitive).
37. Go: re-run coverage report (~85% claim needs re-measuring after all
    changes).
38. Rust: run the thread-safe feature build/tests explicitly
    (`--features` variant as defined in Cargo.toml).
39. Zig: clean-cache hermetic test run once.
40. TS: one full `yarn test` from a fresh clone-equivalent (rm -rf
    node_modules && install && build && test) to prove install-from-scratch.

**Process:**
41. Adopt "one session per worktree" rule in AGENTS.md + CONTRIBUTING.md.
42. Document the rebase resolution policy ("upstream semantics + fork
    formatting, prettier-100") in CONTRIBUTING.md for humans too.
43. Kill or park the idle crush processes from this box (user-level).
44. Consider `git town` sync config to skip rather than auto-rebase huge
    stacks when conflicts are expected (this rebase had 12-file conflicts
    mid-stack).
45. Add a `just`-free task doc: flake app list (`test`, `test-go/-rust/
    -zig`) in CONTRIBUTING quickstart.

**Bigger bets (ROADMAP fuel, not commitments):**
46. Port-parity sprint: close the largest ROADMAP gaps flagged in (13).
47. Upstream relationship: our fixture-coupling and tmp-file fixes as
    goodwill PRs to cordiverse/cordis.
48. Evaluate CI job-count vs runtime after workflow changes (15–20 min caps
    exist; re-validate).
49. Benchmark suite (benchmarks landed earlier in the fork) — run once
    post-rebase to confirm no TS-side regressions skew parity numbers.
50. Docs-health HARVEST pass to route this report's (f) list into
    TODO_LIST.md / ROADMAP.md properly (deliberately NOT done now per this
    session's "report only" instruction).

## g) Questions I cannot answer myself

1. **Concurrency:** several crush sessions are running on this machine and
   at least one other was active in this repo during mine. How do you want
   this coordinated — one-agent-per-repo rule, per-session git worktrees,
   or should I just proceed and accept the racing? Right now the 24-file
   repair batch sits uncommitted while the auto-commit daemon may fire.
2. **Push policy:** local `main` has diverged from `origin/main` (39 vs 31
   commits) after the rebase. Do you want me to complete the sync
   (force-push with lease via your `git sync`) once the repair is
   committed, or hold for your review first?
3. **Fixture style policy:** I chose to keep hmr test fixtures
   byte-identical to upstream (their specs string-replace into them). The
   alternative is fork-styling the fixtures AND rewriting the specs'
   replace patterns. Upstream-sync safety says my choice; repo-wide style
   consistency says the other. Which do you want long-term?

---

*Report format: user-requested Markdown override of the skill's HTML
default (flagged per skill instructions). HARVEST into TODO_LIST/ROADMAP
intentionally deferred pending instructions.*
