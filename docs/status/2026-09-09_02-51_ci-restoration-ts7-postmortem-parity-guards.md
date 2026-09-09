# Session Status — CI Restoration, TS 7 Postmortem, Parity Guards

_Point-in-time snapshot (2026-09-09 02:51 CEST). Will go stale. HARVEST (f)
into TODO_LIST/ROADMAP before it does. Scope: this session only (started
~01:30 CEST); no new research beyond what the run itself surfaced._

## TL;DR

The TODO's first item ("validate the manifest divergence through CI")
uncovered the real state: **CI had been red on every push since `2ac1be1`
(~4.5 h, three commits)** — Build failing on TypeScript 7's broken dts
build, Ports failing the parity semantic guard. Both are fixed, all four
actionable TODO items are executed, two new CI guards are live and verified
in CI, and `4420234` is green end-to-end (Build 8/8 jobs, Ports all jobs,
run IDs 34295490325 / 34295490246, 2026-09-09T00:33Z).

The session's biggest lesson is a process failure, not a code one: I spent
five diagnostic cycles theorizing about dependency drift, node versions and
races before running the ONE decisive experiment — replicating CI's exact
`yarn build core` → `yarn build` sequence. The "unexplained local failure"
was an invocation artifact all along.

## a) FULLY DONE

1. **CI red root-caused and fixed** (`4420234`, pushed 00:33Z, both
   workflows green):
   - Build failure = `typescript ^7.0.2`: the dts build dies with `TS2665:
     Module 'cordis' resolves to an untyped module at lib/index.js` — the
     workspace's cross-package `declare module 'cordis'` augmentation does
     not resolve under TS 7's rewritten resolver. Tests stayed green the
     whole time because vitest never typechecks. Three CI runs
     (34275726430/67, 34292854557/667, 34292916731/706) were red on it.
   - Ports failure = two prettier-surviving formatting diffs vs the pin:
     a stray blank line in `core/src/context.ts` and a re-braced
     single-statement `if` in `core/src/registry.ts` — both restored to
     pin bytes.
   - Manifests reverted to the upstream pin's exact set (yarn 4.14.1,
     TS ^5.9.3, vitest ^4.1.5, esbuild ^0.28.0, eslint ^8.57.1); the only
     fork delta is again root `@types/node ^26.5.0`. `.yarnrc.yml` is
     upstream's two lines again.
2. **Upstream sync decision resolved**: `f8ea3cd`'s entire delta vs
   `caab04e` is the eight workspace manifests (rc.10 version set) —
   adopted byte-for-byte, `git diff f8ea3cd -- 'packages/*/package.json'`
   is empty. The guarded tree needed no changes.
3. **`UPSTREAM_PIN` → tracked file** (TODO item): `.github/UPSTREAM_PIN`
   (one line, `f8ea3cd…`), `ports.yml` reads it into `GITHUB_ENV` — pin
   bumps now review as one-line diffs. Verified live in CI logs.
4. **`package.json` exclusion decided, documented AND machine-guarded**
   (TODO item): `scripts/manifest-parity.mjs` fails CI on any manifest
   difference against the pin outside an explicit allowlist (root
   `@types/node` only). Would have caught all three bump-everything
   incidents. CI-verified: "all manifests match pin f8ea3cd…".
5. **hmr fixture/spec replace-literal canary** (TODO item):
   `scripts/hmr-fixture-canary.mjs` asserts every spec `.replace()` literal
   exists in a fixture (28 literals / 26 fixtures, milliseconds). Validated
   positively AND negatively (simulated mass fixture reformat → 13
   file:line failures). CI-verified. Known limit documented: any-fixture
   matching cannot pin a literal to one exact fixture when several share
   it; single-file breaks of a shared literal pass.
6. **Forensic exclusions proven** (each killed a wrong theory): no
   dependency published in the drift window (390-package registry scan);
   node 26.8.1 identical to the green CI run; workspace symlinks correct;
   `@types/node` 26.5.0 same as CI; deterministic across repeat runs and
   4-core emulation. Plus the decisive one: the **single-step clean
   `yarn build` failure is a yakumo-tsc project-reference artifact** —
   ~35 dts errors deterministically, on the last-green commit itself
   (proven in a clean worktree), while CI's two-step sequence is green.
7. **Full local verification of the final state**: from-scratch install,
   two-step build (0 dts errors), `yarn lint` clean, 248/248 tests, both
   parity guards clean, `nix flake check` all checks passed, markdownlint
   green.
8. **Docs reconciled**: AGENTS.md (toolchain truth rewritten — the old
   "VERIFIED WORKING SET" bullet was wrong; new two-step-build gotcha;
   TS 7 saga RESOLVED bullet; pin-file and guard references), TODO_LIST
   (all items closed), CHANGELOG (Added/Changed/Fixed), ROADMAP (sync
   section, TS toolchain stance resolved-in-practice), FEATURES +
   CONTRIBUTING pin references.

## b) PARTIALLY DONE

1. **Manifest guard negative path**: only its PASS path ran in CI and
   locally. The fail path was never exercised (no synthetic-bad-manifest
   test). A guard whose failure mode is untested is half a guard — the
   canary got a proper negative test, `manifest-parity.mjs` did not.
2. **Single-step build quirk characterized, not root-caused**: pinned as a
   reproducible artifact (documented in AGENTS with the workaround), but I
   did not isolate WHICH component (atsc / dtsc / tsconfig-utils
   interplay) mis-caches the loader augmentation in one-shot runs.
3. **Local gate parity for the new guards**: both scripts run in CI but
   are not wired into the flake (`nix run .#test-*` apps), so local gates
   and CI gates diverge slightly.
4. **AGENTS/CONTRIBUTING coverage of the two-step sequence**: documented
   in AGENTS (agent-facing) but CONTRIBUTING (human-facing quickstart)
   still shows the naive build command; the load-bearing CI sequence is
   not there.

## c) NOT STARTED

1. Upstream issue drafts (deferred deliberately, per verify-before-filing):
   the TS 7.0.2 augmentation regression (needs a minimal reproducer first)
   and the yakumo single-step project-reference quirk.
2. Extending the fixture canary to `packages/include` specs (their
   fixtures are also string-coupled; not surveyed this session).
3. oxlint policy for fork-owned scripts: `scripts/*.mjs` emit ~50 stylistic
   warnings under `.oxlintrc.json` (not a gate today; becomes debt the day
   oxlint is gated). I knowingly left them and did not document the
   decision anywhere.
4. Re-evaluating the Build workflow's `yakumo publish` step for the fork:
   it no-ops while fork versions equal upstream's published set (rc.10
   exists on npm), but any fork-local version bump would make the fork
   attempt to publish `@cordis/*`. Empirically safe today; policy unexamined.
5. Prettier single-sourcing (CI `npx prettier@3.9.6` vs nixpkgs) — still
   open from the 2026-09-08 report, untouched.
6. Nightly fuzz campaign, darwin `--all-systems` flake check — untouched.

## d) TOTALLY FUCKED UP

Nothing is left broken or red. The failures were process, and they were
mine:

1. **Five wasted diagnostic cycles from not replicating CI exactly.** I had
   `build.yml` in hand early; the decisive experiment (`yarn build core` →
   `yarn build`) ran last, after fresh installs, node 26, `taskset`, a
   390-package registry scan and a worktree time-machine. When reproducing
   CI, replicate the CI commands EXACTLY before theorizing about the
   environment. This is the session's top lesson, written into AGENTS as
   the two-step-build gotcha.
2. **Wrong intermediate theory stated as fact**: after reverting TS to
   5.9.3 and still seeing 35 errors locally, I concluded the remaining
   toolchain bumps (yarn 4.18 / vitest 5 / esbuild) broke the build and
   reverted the whole set on that basis. The revert was correct policy
   (upstream pins) but the reasoning was wrong — the failures were the
   invocation artifact, and only TS 7 was a real breaker. Right fix,
   wrong proof, for several cycles.
3. **Self-defeating negative test**: my first canary breaker rewrote the
   spec's own literals together with the fixtures, so the canary "passed"
   for a corrupted-input reason and I nearly accepted it. A passing
   negative test proves nothing until you've seen the guard fire for the
   RIGHT reason. (Manifest guard: see b1 — same class of gap, still open.)
4. **`gh` without `--repo` in a two-remote repo**: my first `gh run list`
   silently showed cordiverse/cordis's runs; I caught it only because the
   commit titles didn't match the fork. Always pass `--repo` here.
5. Small self-inflicted round trips: `yarn --no-immutable install` syntax
   fumble, a sed quoting explosion, a grep-in-nested-quotes miscount
   (`/tmp/main2.log:0`). Each trivial; together they burned real cycles in
   a long chain. Temp files beat deep quoting.

## e) WHAT WE SHOULD IMPROVE

1. **CI-exact replication before environmental theorizing** (see d1) —
   cheap, decisive, and it would have collapsed ~40 minutes of forensics.
2. **Every new guard ships with a negative-path test** — canary got one
   (after a stumble); manifest guard still owes it.
3. **gh in multi-remote repos: always `--repo`.**
4. **Red-main detection**: CI was red ~4.5 h over three pushes unnoticed.
   Branch protection or a notification hook would have flagged `2ac1be1`
   immediately.
5. **Stamp verifications with the commit they ran against** (the 21-18
   report asked for this; I did it implicitly — commit IDs in every claim
   above — but it should be an explicit convention).
6. **Consider a committed yarn.lock** — the lock-free float cost this
   session an entire drift-hunt chapter that a lockfile would have
   eliminated in seconds. Still a user-gated ROADMAP decision; the new
   manifest guard covers the divergence surface but not resolution
   reproducibility.
7. **Intermediate results go to files, not nested shell quoting.**

## f) Things we should get done next

_Brainstorm, impact-ordered tiers — HARVEST fuel, not commitments. Most
tier-3+ items are ROADMAP material; several are pre-existing ROADMAP items
noticed, not researched, this session._

**Now (guards + follow-through):**

1. Negative-path self-test for `scripts/manifest-parity.mjs` (synthetic
   bad manifest must fail; run it in CI next to the real check).
2. Wire both new guards into the flake (`nix run .#test-guards` or fold
   into existing apps) so local gates match CI.
3. Add the CI two-step build sequence (`yarn build core && yarn build`) to
   CONTRIBUTING's quickstart and as an explicit `build:ci` package.json
   script.
4. Fix the stale AGENTS heading "TS workspace gotchas (learned in the
   2026-09-07 upstream rebase)" — drop the parenthetical; the section has
   grown past that date.
5. Extend the fixture canary to `packages/include` specs after surveying
   their replace usage (runtime-string replaces must be excluded).
6. Decide the oxlint policy for `scripts/*.mjs` (conform vs ignore) and
   document it; otherwise the debt is invisible.
7. `git status` hygiene: confirm `tsconfig.temp.json` (generated by
   yakumo-tsc at the root) is ignored; trash `/tmp/canary-test` leftovers.

**Short (upstream + CI posture):**

8. Root-cause the single-step clean `yarn build` failure (bisect
   atsc/dtsc/tsconfig-utils in a sandbox) and file it upstream.
9. Draft the TS 7 TS265/TS2665 augmentation issue upstream — after
   building a minimal reproducer (verify-before-filing).
10. Decide the fork's Publish-step policy in `build.yml` (gate it off, or
    consciously keep riding upstream's published version set).
11. Prettier single-source: derive CI's 3.9.6 pin and the nixpkgs version
    from one place.
12. Branch protection on main (or a red-run notification) — see e4; needs
    a call on how the auto-commit daemon interacts with required checks.
13. TS 7 re-evaluation checkpoint: when upstream adopts TS 7, replay the
    dts build against their tree before ever re-diverging.
14. Annotate the 2026-09-08 21-18 report's §b2 claim ("upstream moved
    further, 47+ files") — the f8ea3cd delta was eight manifests; future
    sessions should not re-chase that.

**Mid (ports, from ROADMAP as observed):**

15. Go parity reassessment: 3-stage reload (#111), include journal (#121),
    bare-specifier resolution (#123), `hmr.watch()` (#128) vs
    `go/loader`/`go/hmr` — port or document divergence.
16. Logger golden scenario for Go.
17. Rust: effect introspection expansion, logger service,
    `internal/get|set|listener|dispatch` interception events.
18. Zig: snapshot/restore + status events, accessor/mixin services,
    logger, loader/hmr module-layout decision.
19. Coverage/bench gate policy (floor vs record-only) — user-gated.
20. yarn.lock policy — user-gated (see e6).
21. One-session-per-worktree convention — user-gated.
22. Generic-method deprecation timeline — user-gated.
23. Nightly/periodic fuzz campaign; `--all-systems` darwin flake check.
24. Verify whether the root `workspaces: ["external/*"]` glob and the
    root tsconfig `paths` for it still serve any purpose on the fork
    (`external/` does not exist here; possibly dead config).

## g) Questions I cannot figure out myself

1. **Publish policy**: Build CI on the fork runs upstream's
   `yakumo publish` step on every main push. It no-ops today only because
   the fork's versions equal upstream's published rc.10 set. Do you want
   the fork to publish at all — and if not, should I gate the Publish step
   off in the fork's `build.yml`? (Intent question; the repo cannot
   answer it.)
2. **yarn.lock**: this session's drift-hunt would have been a one-second
   `git diff` with a committed lockfile. Stay lock-free tracking upstream,
   or commit a fork-only lock now that the manifest guard pins the
   divergence surface? (Standing user-gated ROADMAP decision.)
3. **Red-main enforcement**: required checks on `main` would have caught
   the 4.5-hour red window — but they would also block the auto-commit
   daemon's pushes when red. Branch protection, a notification hook, or
   leave it manual? (Process choice that changes the daemon's workflow.)

## Verification stamps

- Local (tree of `4420234`): install/build-two-step/lint 248/248 tests,
  parity guards, `nix flake check`, markdownlint — all green, 02:07–02:45
  CEST 2026-09-09.
- CI `4420234`: Build 34295490325 (8/8 jobs success), Ports 34295490246
  (go/rust/zig/flake/upstream-parity success; manifest guard and canary
  step logs confirmed), 2026-09-09T00:33Z.
- Prior red set: `2ac1be1`, `1984665`, `3d2ad15` — Build + Ports failure
  each (run IDs in a1).
