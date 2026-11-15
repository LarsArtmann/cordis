# BuildFlow skip audit, retest, and config repair — status report

**Date:** 2026-11-15 11:18 CET
**Repo:** `/home/lars/forks/cordis` (branch `main`, worktree shared with a concurrent session)
**BuildFlow binary measured:** `/home/lars/.local/bin/buildflow`, `version 1d474a0`
**Trigger:** "worth reporting bugs upstream via GitHub Issues?" → "maybe just reenable
them and see if they are still justifiedly disabled first?" → "fix it and clean up
`/home/lars/forks/cordis-retest` if not needed anymore!"

## Scope line

**Verified here (fresh output this session):**

- `date` → `2026-11-15 11:18 CET`.
- Original state, `buildflow --dry-run --verbose` in the repo: exit 69, summary
  `89/108`, `63 findings`, `8 skipped via config`, `45 not applicable`; the five
  config skips were `go-auto-upgrade`, `eslint-fix`, `type-check`,
  `file-size-check`, `vitest-test`.
- Five single-step runs in a throw-away copy with `BUILDFLOW_NO_RESULT_CACHE=1`:
  - `type-check`: exit 0, 590 ms first run / 378 ms verbose run, run id
    `20261115-095931-6898e05b`; exactly **one** `tsc --noEmit` invocation, in
    `packages/cli-cordis`.
  - `vitest-test`: exit 69 (tool exit 1), run id `20261115-100006-e13f3092`,
    34 s, `3 failed | 56 passed | 1 skipped` files and
    `13 failed | 880 passed | 46 skipped (939)` tests.
  - `file-size-check`: 12 findings at the 350-line cap (all Go files,
    `go/fiber.go` 760 lines max); clean at `max_file_size: 800`.
  - `oxlint`: 14 `eslint(curly)` warnings; a direct
    `node_modules/.bin/oxlint` from the repo root reproduces the same warning
    class, so BuildFlow is not inventing them.
  - `go-auto-upgrade`: 12 findings, rules `import-rewrite`,
    `third-party-json-semantics`, `suggest-lo-filter`, `suggest-lo-fromptr`,
    `suggest-lo-map`, `suggest-lo-reduce`.
- Source reads in `/home/lars/projects/BuildFlow` at `1d474a0`:
  `tools/providers/tsconfig_check.go:25` (one tsconfig dir),
  `tools/providers/npm_audit.go:273` (`firstMatchDir`),
  `tools/providers/js_providers.go:71-87,106-117` (oxlint config vocabulary,
  bare `vitest run`), `execution/filtered_tools.go:246` (language-mismatch
  fallback), `execution/pipeline_builder_1.go:176-193` (fan-out is Go-module
  only), `execution/module_fanout.go:29-60`,
  `internal/cli/skip_audit_cmd.go` (`skip-audit` exists).
- `gh repo view LarsArtmann/BuildFlow` → private, issues enabled;
  `gh search issues` over open and closed for `type-check`, `tsconfig`,
  `vitest`, `workspace`, `yarn`, `skip_steps`, `oxlint`, `monorepo`: no
  duplicates for the two defects below; related closed issues are #22, #85
  (tsconfig), #66 (`max_file_size` dead), #42 (cargo at repo root), #37/#43
  (filter/skip attribution), #9 (pnpm-audit at root); #103 is open
  (no provider path for repo-local check scripts).
- After the fix: `buildflow config lint` → "config clean";
  `buildflow -s file-size-check` in the real repo → `✔ 1ms`;
  `buildflow --dry-run --verbose` lists only three skips
  (`type-check`, `vitest-test`, `go-auto-upgrade`) and runs `file-size-check`.
- `dprint check AGENTS.md TODO_LIST.md .buildflow.yml` → exit 0.

**NOT verified here:**

- The devShell's BuildFlow (`nix develop -c buildflow --version` was never run).
  Every number in this report is for the local `1d474a0` binary.
- A fresh `yarn test` baseline. The "926 passed / 13 skipped / 939" figure is
  inherited from AGENTS.md / CI, not re-run this session.
- The full gate after the fix (`nix develop -c buildflow`). Only dry-run and
  targeted steps ran.
- Whether `buildflow --fix` / `buildflow format` rewrites upstream-owned
  `packages/**` via the oxfmt lane (flagged as a hazard below, untested).
- The two failing `cargo-audit` / `cargo-deny` steps (pre-existing, not
  diagnosed).
- `buildflow skip-audit` against the fleet (source read only).
- `buildflow -s markdown-lint` (on-demand gate, not run).
- Whether the hardlink scratch copy wrote through to the real worktree. `git
  status` was checked only at the end; no leak was detected, but the method was
  unsafe by construction.
- Anything about the concurrent session's changes (`go/*.go`,
  `.github/dependabot.yml`, commit `4f7bca0`); they are not this session's work.

## What did you forget?

1. **`buildflow skip-audit` was never run.** I read the command's source and
   quoted its purpose, then did not execute it on `~/projects` or even this
   repo. It is the purpose-built classifier (fixed-upstream / ghost /
   recognized) and would have been the cheapest first evidence.
2. **No fresh `yarn test`.** I inherited the green baseline instead of proving
   it in the scratch copy. That is exactly the "inherited claim is a claim, not
   a fact" failure mode.
3. **No positive proof of the type-check false green.** I inferred from the
   single `tsc` invocation and `firstMatchDir` that a type error in, say,
   `packages/core` would be missed. Injecting one error and re-running
   `type-check` in the copy would have converted inference to demonstration.
4. **The authoritative BuildFlow artifact was never established.** The repo's
   documented gate is `nix develop -c buildflow`; I measured
   `~/.local/bin/buildflow 1d474a0`. Version parity is unverified.
5. **The full pipeline was never run after the fix.** A green `config lint`
   plus a green `file-size-check` does not make the gate green (it is not).
6. **`file-size-check` language scope was never checked.** All 12 findings were
   Go; if the check only counts Go, `max_file_size: 800` does not bound Rust,
   Zig, or TS files at all.
7. **`markdown-lint` was never run,** although `AGENTS.md` and `TODO_LIST.md`
   were edited. dprint is not markdownlint.
8. **The `cargo-audit` / `cargo-deny` failures were left untouched.** They make
   the gate red today; the report should not be read as "the gate is fixed".
9. **No `git status` immediately after each scratch run,** so the hardlink
   write-through risk was never positively excluded, only not observed.
10. **The `oxlint.config.ts` recognition gap was noticed but not tested.** The
    provider's known names (`js_providers.go:71-87`) omit the file upstream
    rc.13 actually ships.
11. **BuildFlow's contribution/issue conventions were not read** before
    recommending issue filing.
12. **The AGENTS.md "deliberate ~376-line AGENTS.md" claim was left stale**
    while BuildFlow's own preflight reports 448 lines (max 220).

## What could you have done better?

- **Use an isolated experiment rig, not a hardlink copy.** `git worktree add`
  (plus a symlinked or copied `node_modules`) avoids the write-through class
  entirely. The first `cp -al` into `/tmp` also failed cross-device and
  produced thousands of error lines; a two-second `df` check would have
  prevented it.
- **Capture evidence in a durable notes file before cleanup.** Run ids, raw
  finding lists, and the exact failure lines were in terminal scrollback and
  `/tmp` logs, which were then trashed. This report reconstructs them; a
  future session cannot.
- **Prove, don't infer, the monorepo coverage claim.** Two minutes of editing a
  copy would have made the issue body irrefutable.
- **Separate "config is clean" from "the gate is green" in the first answer.**
  The original gate was already red; I only said that late.
- **Run the skip audit and the full gate in the same pass** instead of stopping
  at the targeted steps.
- **File the two verified BuildFlow issues** (or ask explicitly whether to)
  instead of parking them in `TODO_LIST.md`. The evidence was ready.
- **Check whether the oxfmt lane is reachable in this repo's normal workflow
  before calling the config fixed.** It is the largest unaddressed risk found
  today.
- **Verify the devShell binary version** so the retest describes the gate
  users actually execute.
- **Compare `buildflow config lint --fix` output with the hand-edit** to catch
  anything the rewrite missed (the hand-edit kept the comments, which is
  better, but the diff would be evidence).
- **Run `buildflow config validate`** in addition to `config lint` for an
  explicit parse check.

## What could you still improve?

- Add a recurring "re-test every skip" ritual: `buildflow skip-audit` plus a
  `policy verified YYYY-MM-DD` date in every rationale. The config now carries
  one such date; the procedure is not written down anywhere.
- Put `buildflow config lint` into CI or the pre-commit hook as a ratchet so
  the config cannot drift back into restated defaults or dead entries.
- Establish which BuildFlow binary is canonical (flake devShell vs local
  install) and record its version in AGENTS.md.
- Re-baseline `yarn test` after commit `4f7bca0` ("reformat the whole tree
  with the repo formatters"); the tree changed under the documented numbers.
- Decide the `packages/**` formatter ownership question once: either BuildFlow's
  oxfmt lane excludes upstream-owned trees, or the fork accepts its output as
  the new parity-normalizable style. Today two formatter configs disagree.
- Make `type-check` a real per-module check (upstream issue or a repo script
  that loops the 16 tsconfigs) instead of a skipped, single-package step.
- Keep the issue evidence and the source citations in one place (the TODO
  entries now hold them; an issue body would hold them better).
- Prefer precise language in reports: "checked", "ran", "read" are different
  verbs, and this session mixed them in the first answer.

## Brutal self-review answers (remaining questions)

- **Did you lie?** No statement was knowingly false. Two claims were inherited
  (the `yarn test` baseline) or inferred (which tsconfig `firstMatchDir` picks
  and why) and are flagged above. The "buildflow has no oxlint step" line was
  the repo's lie, not mine; it is now corrected.
- **Ghost systems?** One candidate: `oxlint-auto-configure` is registered,
  has a dependency from `oxlint`, and still reports "language mismatch" in this
  repo even with no skips. Either its trigger is wrong or its config vocabulary
  is stale. Unconfirmed.
- **Split brains?** Yes, several, listed in section (d): formatter ownership
  (`dprint.json` vs `.oxfmtrc.json`/BuildFlow), skip policy restated in
  `.buildflow.yml` + AGENTS.md + TODO_LIST.md (now aligned, but three copies),
  and "BuildFlow owns type checking" vs "yarn build owns type checking".
- **Removed something useful?** Not this session. Removing the `eslint-fix`
  skip freed nothing (the tool never applies); removing the `file-size-check`
  skip restored a check that passes at 800.
- **Tests?** No new tests were needed for a YAML change; the config lint and
  targeted step are the verification. But no automated guard prevents
  regression.
- **Legacy?** The two skipped steps and the oxfmt hazard are the legacy surface;
  both are now documented.

## a) FULLY DONE

1. **Skip-by-skip re-test.** All five skips were re-enabled in a scratch copy
   (skips removed from `.buildflow.yml`, everything else identical) and run
   individually with `BUILDFLOW_NO_RESULT_CACHE=1`. Evidence: the runs listed
   in the scope line. Scope: `/home/lars/forks/cordis-retest` (trashed) and
   BuildFlow source reads.
2. **`.buildflow.yml` rewritten and verified.** Removed the `eslint-fix` skip
   (dead: no eslint config, BuildFlow already reports missing prerequisites)
   and the `file-size-check` skip (the `max_file_size` knob works; set to 800);
   kept `go-auto-upgrade`, `type-check`, `vitest-test` with rationales that now
   match observed behavior and cite dates. Removed restated defaults
   (`build_mode: full`, `.git`, `node_modules`, `vendor`, `.turbo`).
   Evidence: `buildflow config lint` → clean; `buildflow -s file-size-check` →
   `✔`; dry-run lists three skips.
3. **Documentation synced.** `AGENTS.md` Buildflow bullet now states the
   verified policy, the false-green type-check, the 13 vitest failures, the
   `max_file_size: 800` decision, and the oxlint reality. `TODO_LIST.md`
   Cross-repo gained the two upstream defects plus the low-priority reporting
   bug, each with source citations. Evidence: `dprint check` exit 0; diff stat
   below.
4. **Scratch artifacts removed.** `/home/lars/forks/cordis-retest` (1.7 GB
   apparent hardlink copy) and the five `/tmp/*.log` evidence logs are trashed.
   No tracked file in the real repo was edited by the scratch runs (checked via
   `git status`; the only unrelated change is the concurrent session's
   `dependabot.yml`).
5. **Answer to the original question produced and acted on.** Verdict:
   3 of 5 skips were stale or dead config, not upstream bugs; 2 real BuildFlow
   defects verified (`type-check` one-tsconfig coverage, `vitest-test`
   no-script/package-manager path); 1 optional reporting bug. Two of those are
   now tracked; filing them is still open.

## b) PARTIALLY DONE

1. **The two verified BuildFlow issues are evidenced but not filed.**
   Evidence is complete (source citations, run ids, failure classes).
   Remaining: write the issue bodies (github-voice) and file. Blocker: user
   has not asked for the GitHub write, and the repo is private. Effort: S.
2. **The oxfmt / `packages/**` hazard is noticed but unverified.** Dry runs
   list 150+ upstream-owned files for oxfmt while `dprint.json` excludes
   `packages/**` and `upstream-parity` requires byte identity. Remaining: run
   `buildflow` in fix mode on a copy with `+oxfmt` only and diff. Blocker:
   risk of proving it by rewriting the tree; use a worktree. Effort: S/M.
3. **The type-check skip is honest but leaves a coverage hole.** The step now
   demonstrably checks only `packages/cli-cordis`; `yarn build` remains the
   owner. Remaining: upstream fan-out fix or a repo-local loop. Blocker:
   upstream change. Effort: M.
4. **The vitest skip is confirmed but the workaround is only documentation.**
   Remaining: upstream test-script capability, or a repo step that runs
   `yarn test` through BuildFlow. Blocker: upstream feature. Effort: M/L.
5. **The repo gate is red for pre-existing reasons.** `cargo-audit` and
   `cargo-deny` fail to produce parseable JSON; 41 findings across `art-dupl`,
   `branching-flow`, `flake-meta-checker`, `go-structure-linter`, `todo-check`.
   Remaining: diagnose both cargo failures. Blocker: none, just unstarted.
   Effort: M.
6. **Baseline honesty.** The `yarn test` number is inherited. Remaining: run it
   once in a clean checkout after `4f7bca0`. Blocker: corepack/yarn setup time.
   Effort: M.
7. **`file-size-check` scope is unknown.** Remaining: determine whether it
   counts only Go files and, if so, decide about Rust/Zig/TS. Effort: S.
8. **`buildflow skip-audit` is unrun.** Remaining: run it on `~/projects`, read
   the fixed-upstream/ghost buckets. Effort: S.

## c) NOT STARTED

1. Filing the two BuildFlow issues (plus the optional reporting bug).
2. Testing/fixing the `packages/**` oxfmt hazard and adding an ownership
   exclusion.
3. Diagnosing `cargo-audit` / `cargo-deny` parse failures.
4. Running `buildflow skip-audit` against the fleet.
5. Running the full gate (`nix develop -c buildflow`) after the fix.
6. Verifying the devShell BuildFlow version.
7. Re-baselining `yarn test` after `4f7bca0`.
8. `buildflow -s markdown-lint` on the edited docs.
9. Adding `buildflow config lint` to CI/pre-commit as a ratchet.
10. Per-language file-size decision (Go-only cap vs all languages).
11. Upstream `oxlint.config.ts` recognition (known config names + cache key).
12. Adding a CHANGELOG entry for the config/policy change.
13. Writing the skip re-test procedure into the docs.
14. Fixing the stale "~376-line AGENTS.md" claim (now 448 lines).
15. Deciding whether `go/fiber.go` (760 lines) should be split under the new
    cap or stay as the reference state machine.

## d) TOTALLY FUCKED UP!

1. **BuildFlow's oxfmt lane scans upstream-owned `packages/**` (high).**
   The dry run lists `oxfmt --check ... packages/cli-cordis/src/index.ts ...`
   (150+ files) while `dprint.json` excludes `packages/**` and the
   `upstream-parity` CI job requires byte identity (only
   prettier-normalizable differences allowed). If `buildflow --fix` is ever
   run here, oxfmt may rewrite upstream-owned files and break parity. Root
   cause: BuildFlow's oxfmt lane is not aware of dprint's excludes;
   `.oxfmtrc.json` only ignores `.markdownlint.jsonc`. Mitigation: add
   `packages/**` to `.oxfmtrc.json` `ignorePatterns` or to the BuildFlow
   `exclude` list, then re-run the dry run; do not run fix mode until verified.
   Status: found today, NOT fixed.

2. **The repo's BuildFlow gate is red today for unrelated reasons
   (medium-high).** `cargo-audit` and `cargo-deny` refuse to parse their output
   JSON; the summary also carries 41 findings and an `io-history-ratchet`
   warning (190 files hashed vs a median of 12). A "config clean" claim must not
   be read as "gate green". Root cause: uninvestigated. Mitigation: none yet;
   run `buildflow -s cargo-audit -v` / `-s cargo-deny -v`.

3. **The skip rationale lied for an unknown period (medium).** The comment
   "buildflow has no oxlint step to enable" was false; oxlint runs in every
   mode and reports the same 14 warnings the repo's own oxlint does. Root cause:
   skip rationales were trusted as enduring policy and never re-tested; there is
   no cadence or date requirement. Fixed in `.buildflow.yml` (comment removed
   with the dead `eslint-fix` entry), but the process defect remains.

4. **The scratch rig was unsafe (low, no damage detected).** A hardlink copy of
   a 1.7 GB tree shares inodes: any formatter, generator, or auto-configure step
   run in the copy could have rewritten the real worktree. Only read-only steps
   were run, and no leak was observed, but the first attempt in `/tmp` also
   failed cross-device with thousands of error lines. Use `git worktree` next
   time.

5. **Skip reporting is self-contradictory upstream (low).** For each remaining
   skip, the summary prints both "skipped via skip_steps config" and
   "language mismatch (project: go, javascript, nix, rust)", even though
   javascript is present. Root cause: `nonMatchReason` falls back to a
   language-mismatch label when no trigger condition failed
   (`execution/filtered_tools.go:246`), so user intent is misreported as a
   language fact. Same class as closed #37/#43.

6. **Dry-run output cannot be used as a findings verdict (low).** The scratch
   dry run printed `oxlint ... detect pass (no findings)` while the real run
   reported 14 warnings; dry run prints "Would execute" and does not run the
   formatter detect. I initially treated the dry-run pass as signal. Not fixed;
   use real targeted runs for findings.

## e) WHAT WE SHOULD IMPROVE!

1. **Re-test every skip before trusting it, on a schedule.** Use
   `buildflow skip-audit` plus a `policy verified YYYY-MM-DD` comment on every
   entry. Today only `go-auto-upgrade` carries a date. Impact: prevents another
   year-long false rationale. Fix: add the ritual to AGENTS.md and run it at
   each BuildFlow upgrade.
2. **One canonical BuildFlow artifact.** Decide flake devShell vs local
   install, record the version in AGENTS.md, and re-run measurements when it
   changes. Impact: today's evidence may describe a binary the gate does not
   use.
3. **Experiment in a worktree, never a hardlink copy.** Impact: removes the
   write-through class. Fix: `git worktree add /tmp/cordis-retest HEAD`, build
   or symlink dependencies, run, `git worktree remove`.
4. **Separate config health from gate health in every claim.** "Config lint is
   clean" and "the gate is green" are different sentences. Impact: avoids
   false confidence. Fix: always name the exact command and exit code.
5. **Make `buildflow config lint` a ratchet.** Wire it into CI/pre-commit so
   restated defaults, dead skips, and stale excludes cannot come back. Impact:
   keeps the config honest for free.
6. **One owner per formatter lane.** Resolve dprint vs oxfmt ownership for
   `packages/**` once. Impact: prevents silent upstream-parity breakage. Fix:
   exclusion in `.oxfmtrc.json` (or BuildFlow `exclude`), verified by a
   dry-run diff.
7. **Track the two upstream defects as issues, not just TODO rows.** Impact:
   they survive across repos and can be linked from the skip comments. Fix:
   write and file them, then cite the numbers in `.buildflow.yml`.
8. **Require a date on every policy skip.** Impact: makes rot visible. Fix:
   extend the existing `policy verified` convention; have `skip-audit` warn when
   absent (it already parses the date and flags staleness).
9. **Capture evidence durably.** Keep run ids and raw output until the report
   is written, or better, put them in the report immediately. Impact: avoids
   reconstructing from memory.
10. **Prefer proven over inferred claims in issue bodies.** Impact: issue
    quality; `verify-before-filing` exists for exactly this.

## f) Top next tasks (up to 50, prioritized)

Legend: Impact C/H/M/L; Effort S/M/L; Category.

### Critical / High

1. Verify whether BuildFlow's oxfmt lane rewrites `packages/**` in fix mode
   (worktree copy, `-s oxfmt` with `--fix`) and add an exclusion either way.
   C / S–M / Bug
2. File the `type-check` single-tsconfig issue with the 16-package evidence and
   the source citation. H / S / Bug
3. File the `vitest-test` bare-invocation issue with the 13-failure breakdown
   and the corepack/tsconfig-paths explanation. H / S / Bug
4. Diagnose the `cargo-audit` and `cargo-deny` JSON parse failures; the gate is
   red because of them. H / M / Bug
5. Run `buildflow skip-audit --root ~/projects` and reconcile this repo's three
   skips against the fixed-upstream buckets. H / S / Quality
6. Confirm the devShell's BuildFlow version and whether today's measurements
   apply to it; record the answer. H / S / Documentation
7. Re-run `yarn test` in a clean checkout after `4f7bca0` and update the
   baseline in AGENTS.md if it moved. H / M / Testing
8. Run the full gate (`nix develop -c buildflow`) in the fixed tree and report
   the actual remaining failures. H / M / Quality

### Medium

9. Determine whether `file-size-check` counts only Go files; document it and
   decide about Rust/Zig/TS caps. M / S / Quality
10. Verify the `type-check` false green by injecting a type error into a
    non-`cli-cordis` package in a worktree. M / S / Testing
11. Add `packages/**` to `.oxfmtrc.json` `ignorePatterns` if task 1 confirms the
    hazard. M / S / Bug
12. Add the `oxlint.config.ts` filename to BuildFlow's oxlint config detection
    and cache key (`js_providers.go:71-87`) or file it. M / S / Feature
13. Investigate why `oxlint-auto-configure` reports language mismatch in this
    repo even with no skips. M / S / Bug
14. Wire `buildflow config lint` into CI or the pre-commit hook as a ratchet.
    M / S / Quality
15. Add `policy verified YYYY-MM-DD` to the remaining two skip rationales and
    make the ritual explicit in AGENTS.md. M / S / Documentation
16. Run `buildflow -s markdown-lint` on the edited docs (or confirm the flake
    markdown gate covers them). M / S / Quality
17. Add a CHANGELOG entry for the BuildFlow policy/config change. M / S /
    Documentation
18. File the low-priority issue for skip entries being reported as "language
    mismatch" (`execution/filtered_tools.go:246`). M / S / Bug
19. Decide whether to keep `max_file_size: 800` or split `go/fiber.go`; record
    the decision. M / M / Quality
20. Add a repo-local loop that type-checks all 16 tsconfigs if the upstream
    fan-out is not fixed soon. M / M / Feature
21. Verify that removing the four default excludes changed no scan counts
    (before/after `-v` dry-run counts). M / S / Quality
22. Check whether the `io-history-ratchet` warning (190 vs median 12 files) is
    caused by `4f7bca0` or by the concurrent session. M / S / Quality
23. Confirm the `go-structure-linter`/`art-dupl`/`branching-flow` findings are
    pre-existing and not introduced by the reformat commit. M / M / Quality
24. Link the filed issue numbers from `.buildflow.yml` skip comments once tasks
    2-3 land. M / S / Documentation
25. Add a config-guard test that asserts `skip_steps` equals the reviewed set.
    M / S / Testing
26. Verify the scratch-run procedure can be replaced by `git worktree` with
    working node_modules resolution, and document it. M / S / Documentation
27. Fix the stale "~376-line AGENTS.md" claim (actual 448) and the 220-line
    preflight warning decision. M / S / Documentation
28. Check `buildflow config validate` explicitly (not only `lint`). M / S /
    Quality
29. Run `buildflow skip-audit --json` and archive the output for the next
    session. M / S / Documentation
30. Re-check `.buildflow.yml` after the next nixpkgs/buildflow bump; the
    binary version is part of the evidence. M / S / Quality
31. Add the retest commands (scratch copy recipe, cache-disable env var) to the
    project docs. M / S / Documentation
32. Verify the `oxlint` 14 warnings match `yarn lint` exactly (not just the
    rule class) so the record is precise. M / S / Quality
33. Confirm `go-auto-upgrade`'s 12 findings are still the same set after the
    reformat commit. M / S / Quality
34. Decide whether the `file-size-check` cap should live per language in the
    upstream tool. M / M / Feature
35. Replace the `type-check` skip with a documented `yarn build`-only rule once
    upstream fan-out exists. M / S / Documentation

### Low

36. Capture run ids in a durable audit log for this repo's future reports.
    L / S / Documentation
37. Add a note that dry-run formatter "pass" is not a findings verdict. L / S /
    Documentation
38. Clean up the `/tmp` evidence policy: document where scratch logs live
    before deletion. L / S / Documentation
39. Confirm no tracked file changed via the hardlink copy (repeat `git status`
    immediately after such runs in future). L / S / Quality
40. Check the concurrent `.github/dependabot.yml` diff for conflicts with
    BuildFlow's `dependabot-auto-configure`. L / S / Quality
41. Decide whether `buildflow` should support `--config`/env override so
    scratch configs never require editing the repo file. L / M / Feature
42. Consider a `--force-step`/`--no-skip-steps` flag for A/B skip testing.
    L / M / Feature
43. Improve `nonMatchReason` to say "not matched (reason unknown)" instead of
    "language mismatch" when no condition failed. L / S / Bug
44. Deduplicate the not-applicable and skipped lists in the summary. L / M /
    Quality
45. Document BuildFlow's JS workspace CWD/config resolution for consumers.
    L / S / Documentation
46. Add `buildflow config lint` to the pre-commit example in the BuildFlow docs.
    L / S / Documentation
47. Consider a per-language `max_file_size` option upstream. L / M / Feature
48. Add an explicit "issue filed" checklist to the repo docs so verified
    defects do not stop at TODO rows. L / S / Documentation
49. Re-run the skip retest at the next BuildFlow release, not only when
    something looks wrong. L / S / Quality
50. Keep `.buildflow.yml` comments free of tool-claim rot by citing the
    command/version that verified them. L / S / Documentation

**HARVEST note:** items 2, 3 and 18 are already rows in `TODO_LIST.md`
(Cross-repo). The rest of this list is ROADMAP/TODO_LIST fuel and needs the
`docs-health` HARVEST pass with routing rigor; most of the Low bucket is
ROADMAP, not TODO.

## g) Questions I cannot answer myself (3)

1. **Which BuildFlow is authoritative for this repo?** The documented gate is
   `nix develop -c buildflow`, but every number here came from
   `~/.local/bin/buildflow 1d474a0`. I did not run the devShell binary, and only
   you can say whether the flake pin, the local install, or the
   `~/projects/BuildFlow` worktree is the one that should define "the gate".
2. **Is `buildflow --fix` / `buildflow format` part of this repo's normal
   workflow?** The oxfmt lane lists 150+ files under upstream-owned
   `packages/**`. If you never run fix mode here, the hazard is latent; if you
   do, it should be fixed now by excluding `packages/**`. I cannot infer the
   workflow rule from the repo alone.
3. **Do you want the two verified BuildFlow issues filed now?** They are
   private-repo issues with complete evidence, and the `type-check`/
   `vitest-test` skips reference them. If you prefer, I can keep the findings in
   `TODO_LIST.md` only, or implement the repo-local workarounds instead.

## Appendix: files changed this session (uncommitted)

```
 .buildflow.yml  | 58 ++++++++++++++++++++------------------------------
 AGENTS.md       | 23 ++++++++++++++-------
 TODO_LIST.md    | 17 +++++++++++++++++
```

Not mine, left untouched: `.github/dependabot.yml` (concurrent session),
`go/*.go` (concurrent session), commit `4f7bca0` "style: reformat the whole
tree with the repo formatters" (landed 10:56:50, before this session's first
dry run). Per the harness contract no commit was made; the auto-commit daemon
may pick these files up.
