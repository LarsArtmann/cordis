# Session Status — Backlog Execution, CI Guards, Racing-Recovery

_Point-in-time snapshot (2026-09-08 21:18 CEST). Will go stale. HARVEST (f) into TODO_LIST/ROADMAP before it does._

## TL;DR

All nine harvested Repo TODO items were executed and verified end-to-end: the
flake gate now runs in CI (`flake` job), `packages/**` is guarded against
upstream drift (`upstream-parity` job: byte + prettier-normalized semantic +
dprint guards), the loader JSON config layer has a fuzz target, the local race
canary matches CI (`-race -count=3`), CONTRIBUTING.md reflects reality, the
TS workspace was proven install-from-scratch green (248/248), and both GitHub
workflows are green on the current HEAD (`3da7d0f`).

Execution happened under **active concurrent-session interference**: another
agent (and the auto-commit daemon with amend cycles) rewrote `packages/**`,
`flake.nix`, `TODO_LIST.md`, `ports.yml` and one hmr fixture _while_ this
session worked. Three CI runs failed on racing half-states before the final
green pair. Two serious regressions shipped by the racing session were
diagnosed and repaired here: a reintroduction of the fatal `typescript
^7.0.2` pin, and hmr test/fixture style incoherence that timed out 17 tests.

## a) FULLY DONE

1. **CI `flake` job** (`ports.yml`): `DeterminateSystems/nix-installer-action`
   pinned to the v22 commit SHA (`ef8a1480…`), runs `nix flake check` — the
   flake gate (go vet + `-race -count=3`, cargo clippy + both feature test
   suites, zig leak-checked tests) is now enforced remotely. Green in CI
   three times (e.g. run 34266167600, 1m06s).
2. **Race-canary alignment** (TODO item 9): `flake.nix` checks and both go
   test apps moved from `-count=1` to `-race -count=3`, matching
   `ports.yml`. Local `nix flake check` verified: all three checks passed.
3. **`upstream-parity` CI job** (TODO item 3, re-scoped): the literal
   "packages/** stays byte-identical to upstream" premise was false — the
   fork carried deliberate prettier-100 formatting mixed with upstream-blob
   files, so no single upstream commit matched byte-for-byte. Implemented the
   honest version against pin `caab04e`:
   - byte guard: every non-TS/non-JS/non-manifest file under `packages/**`
     must be byte-identical to the pin;
   - semantic guard: TS/JS from both trees normalized with pinned prettier
     (`npx prettier@3.9.6`, same version nixpkgs shipped) must diff EMPTY —
     fork TS may differ from upstream only by formatting;
   - `dprint.json` excludes guard: `jq -e '.excludes | index("packages/**")'`.
     Green in CI (12s).
4. **Loader JSON config fuzz** (TODO item 8):
   `go/loader/config_fuzz_test.go` — property: any input decodes without
   panicking; every successful decode re-encodes; encode∘decode is
   byte-idempotent. 9 hand seeds + 45s live fuzzing: 19,084,861 execs, 513
   corpus inputs, PASS. Seeds run as ordinary unit tests in every gate.
5. **TS install-from-scratch verification** (TODO item 6): trashed
   `node_modules` (all workspaces) and `yarn.lock`, then
   `yarn install --no-immutable && yarn build && yarn test` → **248/248
   tests, 25 files, green** (also re-proven after every manifest/fixture
   repair).
6. **CONTRIBUTING.md rewritten** (TODO item 7): flake app quickstart
   (`test`, `test-go/-rust/-zig`, `nix flake check`), TS workspace commands,
   the upstream sync policy including `UPSTREAM_PIN` bump discipline, and
   upstream-issues routing. dprint-clean.
7. **`tmp-*` gitignored** (TODO item 5).
8. **CI green on final state** (TODO item 4): run set for `3da7d0f` —
   Build ✅ 2m33s, Ports ✅ 1m13s (go ✓, rust ✓ incl. thread-safe clippy,
   zig ✓, flake ✓, upstream-parity ✓).
9. **Regression repair — TS manifest** (found via item 6): commit `0542b6d`
   ("sync TS workspace with upstream") had reintroduced
   `typescript: ^7.0.2` — the exact pin AGENTS.md documents as crashing
   every `yarn install` on yarn's `lib/_tsc.js` compat lstat — plus kept
   fork-era bumps upstream never adopted (`js-yaml ^5.4.1` which broke
   `yaml.Type` at build time, `chokidar ^5`, `supports-color ^11`,
   `esbuild ^0.28.2` in hmr, and more). Root `package.json` and all six
   diverging workspace manifests realigned to upstream main bytes; the
   fork's only manifest delta is again the deliberate `@types/node ^26.5.0`.
10. **Regression repair — hmr fixture coupling** (found via item 6): the
    fork-styled (double-quote) fixture set no longer matched the
    upstream-style spec replace literals — 17 hmr tests timed out
    ("waitFor timed out", the exact AGENTS-documented failure mode).
    All 18 `packages/hmr/tests/` files restored to upstream-main bytes;
    suite green. ROADMAP's open "hmr fixture style" question resolved in
    place: fixtures stay byte-identical to upstream, now CI-enforced.
11. **Regression repair — core formatting churn**: `bin.js` (quote/semi
    churn), an extra blank line in `context.ts`, and a re-braced guard in
    `registry.ts` reverted to upstream bytes; `include` plugin fixtures and
    `base.yml` restored (the yml churn the 05-27 report predicted).
12. **Go lint findings** (Ports red on `fa45896`): dead `uint64 < 0`
    comparison in the hmr storm test (SA4003), unchecked `ReplaceType`
    return (errcheck, now asserted `found=false` + zero registration),
    unchecked `ctx.Parallel` in the golden dispatch runner (now fatal with
    context). golangci-lint: 0 issues; golden traces still byte-identical.
13. **Docs reconciled**: AGENTS.md (style policy rewritten: upstream style
    is CI-enforced; prettier is not a packages gate — 47/64 files fail
    `--check` under every plausible config; fixture coupling widened;
    prettier 3.9.6 lockstep note), CHANGELOG (Added: CI jobs + fuzz;
    Fixed: manifests, fixtures, core churn), ROADMAP (fixture-style
    question closed), TODO_LIST (all nine items resolved; list empty).

## b) PARTIALLY DONE

1. **TODO item 2 (prettier)**: resolved as _not-do_ with evidence (no
   prettier-stable style exists to enforce; `yarn build` already precedes
   publish; no fork-owned TS exists), but that means **no machine gate
   covers fork-authored TS style** — acceptable only while no fork TS
   exists.
2. **Upstream sync state**: the fork's TS content base is the `caab04e` pin
   plus selectively adopted newer upstream tests; upstream main (`f8ea3cd`)
   has moved further (rc.10 version set, hmr src evolution, 47+ files).
   A real sync + pin bump is _pending_, deliberately not started here.
3. **Concurrent session's claims**: `75fb408`/`25ff5fb`/`3da7d0f` claim
   Rust interception events, root-fiber coverage, thread-safe clippy gate,
   llvm-cov baseline and benches. Spot-verified: `EVENT_PLUGIN`/`EVENT_UPDATE`
   in `rust/src/fiber.rs`, parity test at `rust/tests/parity.rs:756`,
   clippy thread-safe step in `ports.yml`. The llvm-cov/bench artifacts
   were **not** verified by this session.
4. **Fuzzing depth**: 45 seconds of live fuzzing (seeds run permanently);
   no long-running or CI-scheduled fuzz campaign yet.
5. **`docs/status/2026-09-08_21-11_rust-todo-sweep….md`** (concurrent
   session) is still **untracked** — invisible to `nix flake check` per the
   AGENTS gotcha; nobody has `git add`ed it yet.

## c) NOT STARTED

1. Real upstream sync of `packages/**` to current upstream main (rc.10 set,
   hmr src), with `UPSTREAM_PIN` bump in the same commit.
2. Oxlint policy decision for upstream TS (the fork lint config still
   reports ~26 errors / 190 warnings on `packages/core` alone — pure noise
   today).
3. Nightly/periodic fuzz campaign and `--all-systems` darwin flake check.
4. This session's own status-report HARVEST into TODO_LIST (this report's
   section f; TODO_LIST is currently empty by design).
5. Session-lock convention — proposed repeatedly in past reports, still
   nothing encoded anywhere.

## d) TOTALLY FUCKED UP

1. **Racing sessions + amend-happy daemon**: a second session rewrote the
   same trees concurrently; the auto-commit daemon amend-cycled commits
   (HEAD changed identity under running verification twice; one commit's
   file content differed between two `git show` calls minutes apart). My
   fixture restores were overwritten in-flight; one `upstream-parity` CI
   failure was purely a racing half-state. Three consecutive push pairs
   were red before the final green pair. This is the third report in a row
   naming the missing coordination convention as the top structural risk.
2. **`0542b6d` regression**: a commit titled "sync TS workspace with
   upstream" reintroduced `typescript ^7.0.2` (fatal to every install, no
   lockfile to save it) and preserved fork-era bumps upstream never had —
   **second occurrence** of the js-yaml ^5.4.1 hazard AGENTS.md explicitly
   documents. Build was red on two pushes; only caught because this
   session ran the install-from-scratch verification the same evening.
3. **My own missteps (honesty section)**:
   - I initially designed guards against a contradictory premise (byte
     parity AND fork formatting) and only discovered the contradiction by
     running `prettier --check` late; the premise should have been tested
     first, before any design.
   - I misread a `git diff A B` direction once (upstream-vs-fork) and
     briefly concluded the fork was ahead of upstream when it was behind —
     cost a full re-diagnosis round.
   - I let three background verifications queue while the tree kept moving;
     two had to be re-run against newer HEADs. In a racing repo,
     verification results must be stamped with the exact commit verified.

## e) WHAT WE SHOULD IMPROVE

1. **Coordination**: encode a session-lock or worktree-per-session rule
   (ROADMAP open item); the daemon needs a "no amend after push" rule.
2. **Manifest safety net**: decide yarn.lock policy once — lock-free bit us
   twice (TS7 twice, js-yaml twice). Either commit a fork-only lockfile or
   extend `upstream-parity` to cover `package.json` deps (currently
   excluded).
3. **Fast-fail fixture canary**: a tiny test asserting every spec
   `.replace()` literal exists in its fixture would catch the fixture-style
   hazard in milliseconds instead of 190 seconds of waitFor timeouts.
4. **Pin ergonomics**: move `UPSTREAM_PIN` from workflow YAML into a
   tracked one-line file read by the workflow; bumping becomes a diff anyone
   can review.
5. **Verify-stamping**: every verification result should record the exact
   commit hash it ran against (this session's confusion was mostly
   un-stamped state).
6. **Prettier version single-source**: CI pins `3.9.6` via npx, local uses
   nixpkgs — derive one from the other automatically.
7. **Claim verification**: concurrent sessions' "done" claims (llvm-cov,
   benches) should be re-verified by docs-health VERIFY before trusted.
8. **Premise-testing first**: before building any guard, run the cheapest
   experiment that could kill its premise (would have saved an hour here).

## f) 50 things we should get done next

_Brainstorm, impact-ordered tiers — HARVEST fuel, not commitments. Most
items below tier 3 are ROADMAP material._

**Now / coordination:**

1. Decide and encode the concurrent-session convention (lock, worktrees, or
   serialized handoff) — blocked-on-user decision, third report raising it.
2. Sync `packages/**` to current upstream main (`f8ea3cd`), bump
   `UPSTREAM_PIN` in the same commit, run all gates.
3. ~~`git add` the untracked `2026-09-08_21-11` status report (flake
   invisibility gotcha).~~ done (second docs-health pass staged it)
4. ~~HARVEST this report's (f) into TODO_LIST/ROADMAP via docs-health.~~ done (second docs-health pass, 2026-09-08)
5. Add the fixture/spec replace-literal canary test to `packages/hmr`.
6. Move `UPSTREAM_PIN` into a tracked file (e.g. `UPSTREAM_PIN`) the
   workflow reads.
7. Decide yarn.lock policy (fork-only committed lockfile vs lock-free +
   manifest guard) — blocked-on-user tradeoff.
8. Extend or explicitly document the `package.json` exclusion in
   `upstream-parity` (it is the one unguarded drift surface left).
9. ~~Verify the concurrent session's llvm-cov/bench claims and artifacts;
   annotate their 21-11 report with evidence links.~~ done (second docs-health pass: `25ff5fb` contents verified — `rust/benches/core.rs`, clippy gates, AGENTS/ROADMAP records; 21-11 annotated)
10. ~~Add "re-run fresh `yarn install` after any manifest change" checklist
    line to AGENTS.md + CONTRIBUTING PR checklist.~~ done (AGENTS toolchain-pins + yarn-build gotchas and the CONTRIBUTING sync policy cover the discipline)

**CI hardening:**
11. Add a nightly fuzz job (`go test -fuzz=FuzzConfigRoundtrip -fuzztime=10m`)
with corpus cache artifact.
12. Cache the nix store in CI (magic-nix-cache) — the flake job spends most
of its 1m15s downloading toolchains.
13. Pin the golangci-lint version used by `eab03a8`'s Ports step to the same
version the devShell ships (version skew already caused one
local-green/CI-red split).
14. Branch protection requiring both Build and Ports green on main.
15. Bump/replace `mlugg/setup-zig` when its Node 20 deprecation is fixed.
16. Add `--all-systems` flake check on a darwin runner (or remote builder).
17. Matrix-extend the go job with `-count=5` hmr/loader sweep as a weekly
canary (15-43 f36).
18. upload `go test -cover` artifacts to CI for trend tracking.

**Upstream tracking:**
19. Track upstream's merge of `3-stage-hmr`; when merged, collapse the fork
replay divergence and re-pin.
20. Upstream goodwill PRs: style-agnostic hmr spec replaces; include-test
`tmp-*` → `os.tmpdir`; afterEach cleanup on crash paths.
21. Track `RequireNoResidue` / cordis issue #2 (Kernovia feedback loop).
22. Re-check `packages/hmr/README.md` drift once upstream's README settles
(f30 of 15-43, currently upstream-identical).
23. Decide whether `internal/` event namespaces need Zig parity
(`75fb408` closed Rust; Zig has typed registry but interception events
unverified this session).

**Port quality (Go flagship):**
24. ~~Loader coverage push: 74.6% is the weakest number; add tests from the
fuzz corpus's interesting inputs.~~ done at `72e1505` (90.8%, recorded in AGENTS.md)
25. Regression table tests derived from fuzz corpus entries (pin the
byte-idempotence property as plain unit tests for the top 50 inputs).
26. Re-audit bare `_ =` discards across `go/` after the last week's commits.
27. Sweep for remaining unchecked returns in test files (errcheck caught
three this week; assume more exist).
28. `Fiber.Err` / cancellable-await golden coverage beyond dispatch scenario.
29. ~~Consider surfacing `Tree.Await`'s discarded fiber error via the loader
error sink (deliberate today — re-justify or fix; 05-27 f33).~~ done at `72e1505`
30. Timer: periodic `-count=5` synctest bubble run (flakiness sentinel).

**Rust / Zig:**
31. Verify llvm-cov numbers landed and are visible in CI artifacts.
32. ~~Bench numbers for the "up to 30% faster" ROADMAP claim, or hedge the
claim.~~ done at `25ff5fb`
33. Zig interception events parity check (M13 set).
34. Zig `zig build docs` output review pass (doc comment quality).
35. Re-run thread-safe clippy gate after next rustc bump (nursery lints
historically reappear; 05-27 f36 pattern).

**Docs / knowledge:**
36. Record today's racing incident + resolution in AGENTS.md daemon notes
(amend-after-push hazard observed live).
37. Mirror the `UPSTREAM_PIN` sync runbook into PORTS.md's upstream section.
38. ~~ROADMAP: add "prettier is not a TS style gate" as a settled decision
(currently only in AGENTS.md).~~ **NOT-DO/DUPLICATE — AGENTS.md is the single home for the style policy; duplicating it in ROADMAP recreates the drift the rule prevents.**
39. ~~FEATURES.md freshness pass after today's CI/fixture changes (a staged
edit from the other session may already cover it — verify).~~ done (second docs-health pass verified and corrected: golden-runner matrix, inherited-features + CI matrix note)
40. ~~Annotate the 15-43 and 05-27 reports' newly-completed items
(docs-health ANNOTATE pass).~~ done (second docs-health pass, 2026-09-08)

**Tooling / environment:**
41. buildflow upstream fix: filter `nix flake show` checks to the running
system (still open, blocks fully green buildflow).
42. buildflow per-step workdir knob (would obsolete the root go.mod stub).
43. `nix run .#test-ts` flake app wrapping the documented yarn incantation.
44. ~~vulnix/cargo-audit/cargo-deny/lychee/codespell availability in devShell
(05-27 f9 noise).~~ see 05-27 §f9 (still open, tool-side)
45. Pin the flake's zig version explicitly (nixpkgs drift risk, 05-27 f19).

**Strategy:**
46. ~~Decide whether fork packages/** dep bumps are ever acceptable without an
upstream commit — encode the "never" explicitly.~~ done (encoded in AGENTS.md: "TS dep bumps remain upstream decisions")
47. Evaluate git-town / sync tooling to make upstream syncs one-command
(15-43 f44).
48. Port-parity sprint: ROADMAP's flagged gaps (hmr watch parity, loader
bare-specifier resolution).
49. Job-count/runtime review of both workflows after this week's additions
(15-43 f48).
50. Schedule flake lock refresh + toolchain bump cadence (05-27 f50).

## g) Questions I cannot answer myself

1. **Coordination intent:** the concurrent session overwrote parts of my
   in-flight fixture restores mid-verification, and the daemon amend-cycled
   already-pushed-looking commits (HEAD identity changed under two
   verifications). Was that session authorized to touch `packages/**` and
   `flake.nix` while mine was running — and do you want
   one-agent-per-repo, per-session worktrees, or explicit handoff? I can
   only observe the racing; I cannot know the intended ownership.
2. **Manifest risk tradeoff:** after the second TS7/js-yaml incident: do
   you accept a fork-only committed `yarn.lock` (diverges from upstream's
   lock-free stance, instantly reproducible installs) or should we stay
   lock-free and instead extend `upstream-parity` to guard manifest dep
   ranges against the pin? The right choice depends on how much install
   breakage you are willing to trade for upstream-diff cleanliness — I
   cannot weigh that for you.
3. **Fixture authority:** is upstream-main byte-parity for
   `packages/**/tests/**` a hard invariant, or may the fork carry
   upstream-divergent test content when a port (e.g. Go hmr) implements
   matching behavior? The racing session's "plugin-error fixture change"
   suggests it considered divergence intentional; the final state (and the
   passing suite, and the new CI guard) says upstream-identical. Which is
   the long-term contract?

---

_Report format: user-requested Markdown override of the skill's HTML default
(flagged per skill instructions). HARVEST (f) into TODO_LIST/ROADMAP before
it goes stale._

---

## Resolution (annotated 2026-09-08, second docs-health pass)

§f items 3, 4, 9, 10, 24, 29, 32, 38–40 and 46 carry inline verdicts
above; §a claims 1–13 were independently re-verified by this pass (CI
green on `3da7d0f` — Build 34267336684 / Ports 34267336671 — including
the flake and upstream-parity jobs). Still open, routed: the upstream
sync + pin bump (§b2, §c1, §f2 → TODO_LIST), the fixture/spec canary
(§f5 → TODO_LIST), the UPSTREAM_PIN tracked file (§f6 → TODO_LIST), the
package.json exclusion decision (§f8 → TODO_LIST), nightly fuzz + darwin
checks (§c3, §f11, §f16), CI hardening (§f12–18), upstream tracking
(§f19–23), Go/Rust/Zig quality items (§f25–35), daemon/coordination
notes (§f36, §c5 → ROADMAP), PORTS pin runbook (§f37), buildflow and
toolchain items (§f41–45), git-town and job-count reviews (§f47, §f49),
flake-lock cadence (§f50). §g questions remain user-gated (ROADMAP Open
decisions carries the standing ones).
