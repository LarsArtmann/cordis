# Status Report — Docs-Health Audit: All 12 Historical Docs Annotated, Living Docs Rebuilt, Gate Fully Green

**Date:** 2026-09-08 16:47 CEST
**Session scope:** single directive — "View ALL \*\*/2026-0\* files, execute the
docs-health skill, make the six living docs superb, archive fully-done
reports." Full AUDIT mode (VERIFY + HARVEST + BUILD + ANNOTATE) over all 11
status reports and 1 planning doc, plus every living doc.
**HEAD at write time:** `b4650df` (clean port gates; 20 files modified by this
session, listed under (a)).
**Format note:** written as `.md` per explicit user instruction; the
status-report skill's canonical format is a styled HTML dashboard — override
honored and flagged per skill spec.

---

## TL;DR

The fork's documentation was in the predicted post-rebase state: eleven
timestamped reports had accreted with **zero harvested next-task lists and
zero inline resolution markers**, the living docs had drifted four sessions
behind the code (CHANGELOG was still a fake `[0.1.0] - 2026-01-01` stub,
FEATURES stopped at phase 2, README advertised Go 1.26 and the string-keyed
API), and two "known truths" were quietly false — the TS7/yarn install
blocker had resolved itself upstream, and "~85% coverage" was folklore.

This session ran the full docs-health AUDIT: every one of the 12 historical
files now carries **318 inline verdicts** (strikethrough + commit hashes /
verified evidence / won't-implement), 24 verified open items were harvested
into a rebuilt `TODO_LIST.md`, the user-gated decisions were consolidated
into a new ROADMAP "Open decisions" section, and all six living docs were
rewritten against the actual tree. The full verification battery ran green
in-session — including `nix flake check`, `golangci-lint` under Go 1.27,
`cargo clippy --all-targets`, the thread-safe Rust suite, a `-race -count=5`
sweep, a fresh Zig run, dprint and markdownlint — and the real per-package
coverage was measured for the first time since M09 (core 91.7%, loader
74.6% is the honest gap).

**Archived: none.** No historical file is fully resolved — each retains
genuinely open items, which the absence of `done at` markers now signals
correctly. That is the honest outcome of the archive rule, not a shortcut.

---

## a) FULLY DONE

### Discovery / research (all verified against the repo, not reports)

1. **The environment snapshot at session start was stale.** It showed
   pre-rebase HEAD `1c38a41` with 20 modified files; the actual tree is
   clean at `b4650df` (post-rebase, repair batch committed). Every verdict
   in this session was re-based on fresh `git status`/`git log`.
2. **The TS7 install blocker is moot.** Upstream `main` itself pins
   `typescript ^5.9.3` again; the fork's only manifest delta vs upstream is
   the deliberate `@types/node ^26.5.0` bump (tsc-verified). The 248/248
   local TS run from the `69b9fd6` session stands on matching pins.
3. **`3-stage-hmr` was already replayed** onto the fork (`b4650df`:
   commit-based loader entry changes, atomic include writes, `hmr.watch()`).
   AGENTS.md's "gate on Ports, do not debug Build" gotcha was
   inline-corrected to the new state (CI confirmation pending next push).
4. **Batch API contradiction in ROADMAP resolved by code:** `Context::batch`
   exists (`rust/src/context.rs:208`); the stale "add Context::batch" prose
   item is gone.

### Verification battery (every command run this session, all green)

5. `nix run .#test` (Go + Rust + Zig composite) — green.
6. `nix flake check` — **all checks passed** (go vet+race, cargo
   clippy+test, zig; foreign systems skipped as designed).
7. `golangci-lint run` under Go 1.27 — **0 issues**.
8. `cargo clippy --all-targets` — clean; `cargo test --features
   thread-safe` — green (first explicit post-toolchain run).
9. `go test -race -count=5 ./hmr/... ./loader/...` — green (the flake sweep
   the 15-43 report asked for).
10. Fresh `zig build test` (`nix run nixpkgs#zig`, outside the flake cache)
    — green.
11. Go benchmarks execute post-rebase (`-benchtime=1x` sanity, 6 benchmarks
    across the core package).
12. `dprint fmt` + `dprint check` clean; `markdownlint` (repo config) clean
    on all living docs.

### BUILD / living docs rebuilt (all claims grep- or run-verified)

13. **TODO_LIST.md** — rebuilt to open-only: 24 bounded, code-verified items
    with per-item citations (`file:line` + source report). Completed work
    deleted (lives in CHANGELOG now), no trophy sections. Includes the
    measured loader-coverage gap (74.6%).
14. **FEATURES.md** — rebuilt with the canonical status vocabulary
    (FULLY_FUNCTIONAL / PARTIALLY_FUNCTIONAL / PLANNED): Zig rows corrected
    (batch, effects introspection, dispatch modes — all landed), a new Go
    ecosystem table (timer/group/loader/hmr/accessor/callable/polish/
    benchmarks), golden count 1→3, honest TS-suite row, "Planned" trimmed
    to genuinely code-less items.
15. **CHANGELOG.md** — the lying `[0.1.0] - 2026-01-01` stub replaced with
    the real arc: ports, native APIs, ecosystem packages, Rust thread-safe/
    snapshots/status, Zig modes, goldens, Go 1.27, the 2026-09-08 upstream
    sync + `3-stage-hmr` replay, the three real fixes (deadlock,
    PollWatcher race, fixture coupling), and the two pushed tags.
16. **README.md** — Go 1.27+ badge (was 1.26), honest per-language status
    table (Go ecosystem, Rust thread-safe/snapshots, Zig modes — no more
    "Foundation"), the 30-second example rewritten around the **typed API**
    (mirrors the compile-proven `go/example_test.go`), golden guarantee in
    Shared architecture, Status section updated.
17. **ROADMAP.md** — split brains fixed (Rust batch, Zig disposers,
    effects-introspection cell, first-green-CI); new **Upstream sync
    (2026-09-08)** subsection; new **Open decisions (user-gated)** section
    consolidating seven standing questions; phase-3 generic-methods idea
    recorded with the research-verified interface limitation.
18. **PORTS.md** — Zig row updated to core-complete reality; "one golden
    scenario" → three.
19. **docs/DOMAIN_LANGUAGE.md** — added the missing ubiquitous-language
    terms flagged open since pass 4: Accessor, Mixin, Callable service,
    Tracker, Status event, Swap, Generation, Accept set, Entry/EntryTree/
    commit, Resolver, Snapshot/restore.
20. **AGENTS.md** — TS7 blocker paragraph inline-corrected to RESOLVED
    (with the `@types/node ^26.5.0` delta fact); layout bullets updated
    (`go/` subpackages, `rust/` thread-safe feature); **real coverage
    numbers replace "~85%"**; BorrowExt gotcha added; `3-stage-hmr` replay
    recorded in Upstream facts.

### ANNOTATE / HARVEST / ARCHIVE

21. **All 12 historical files annotated inline** — 318 verdicts total,
    applied with the skill's `annotate-rows.py`/`annotate-prose.py`
    (dry-run first on every new file shape, shape-checked writes):
    08-22 (45 items + headline), 15-34 (42 table rows + §g), 19-09 (21 +
    the layout question), 20-52 (33 + §g), 22-48 (36 + §g 2/3), 03-03 (24),
    04-04 (12 P0 items, extending its existing appendix), 04-32 (22 + §g
    Q3), 05-27 (14 + §g Q3), 07-46 (19 + §g 2/3), 15-43 (19), plan (all 27
    M-rows + F-tier resolution appendix with 5 named open exceptions).
22. **HARVEST executed forward:** 24 bounded items into `TODO_LIST.md` with
    dual citations; ideas and user decisions routed to ROADMAP; duplicates
    dropped; questions never turned into tasks.
23. **ARCHIVE rule applied honestly:** zero files qualified — every file
    retains open items (e.g. 08-22 still has miri-CI, the greeter example,
    the perf note; the plan still has F26.7/F27.1/F27.3/F27.6/F27.8-half).
24. **Cross-file consistency sweep clean:** stale-phrase grep over all
    living docs ("~85%", "Foundation:", "pending push", "1.26") returns
    only the legitimate govalid gotcha line; TODO↔CHANGELOG and
    TODO↔ROADMAP duplication removed (flake-check CI job, push
    verification and the logger golden each have exactly one home).

## b) PARTIALLY DONE

1. **eslint/oxlint re-run (15-43 §f7).** eslint hits the documented
   per-package ignore-pattern invocation mismatch; oxlint is not installed
   on this machine. Left open in TODO_LIST's source report only.
2. **TS behavioral claims.** The 248/248 suite run is accepted from the
   `69b9fd6` session's report (same pins, same tree), not re-run here; the
   install-from-scratch verification remains a TODO_LIST item.
3. **ROADMAP parity reassessment (15-43 §f13).** The reassessment is now an
   explicit, well-scoped ROADMAP task naming #111/#121/#123/#128 and the
   replay — but the actual Go/Rust/Zig gap analysis has not been performed.
4. **The plan's 150 fine-grained F-rows.** M01–M27 carry inline markers;
   the F-tier is resolved via the resolution appendix (decomposition
   argument + 5 named open exceptions) rather than 150 hand-struck rows.
   Defensible, but it is the one place where the "every numbered item"
   rule was satisfied by argument instead of per-row markers.
5. **Report commit.** This report is not committed (no commit was
   requested; the auto-daemon is expected to pick it up, as with prior
   reports).
6. **`packages/*/node_modules` state.** Verified present and legitimate
   (live install behind the green TS suite) — the "trash stale installs"
   item was marked Won't-implement in 05-27, but a fresh-install-from-
   nothing proof is still outstanding.

## c) NOT STARTED

- **Push / `git sync`:** local `main` still diverged from `origin/main`
  (force-with-lease needed; user-gated). Post-push CI verification of
  build.yml (incl. the replay) and ports.yml.
- **Port-level parity work** for the inherited upstream features
  (#111/#121/#123/#128 + replay) into `go/loader`/`go/hmr` or documented
  divergence.
- **CI hardening:** `nix flake check` job; `.prettierrc` + `prettier
  --check` + `yarn build` before tests in build.yml; `packages/**`
  byte-parity and dprint-excludes guards; `-count=3` parity in the local
  flake gate.
- **Golden scenario #4** (dispatch-mode parity) and a logger golden.
- **Zig:** snapshot/restore + status events, accessor/mixin, `zig build
  -femit-docs`, AGENTS std-gotchas section.
- **Rust:** `internal/plugin`+`internal/update` events, root-fiber status
  emission check, `significant_drop` cleanup → gate thread-safe clippy,
  cargo-llvm-cov baseline, `cargo bench` for the "30% faster" claim.
- **Go:** `go fix ./...` sweep, IntervalFunc goroutine lifetime, hmr
  rollback error detail, `Resolver.ReplaceType[C]`, storm test, loader
  config fuzz, timer property test, loader coverage push.
- **Docs/process:** CONTRIBUTING review + rebase-policy documentation,
  GitHub Release pages, release-cadence execution.
- **User-gated decisions** (parked in ROADMAP): push approval, yarn.lock
  policy, TS toolchain stance, hmr fixture style, one-session-per-worktree,
  oxlint policy, generic-method deprecation timeline.
- **Machine-level (out of repo scope):** dangling `~/.cache/go-build`
  symlink, buildflow platform-mismatch fix, gopls on Go 1.27, govalid/
  erraudit provisioning.

## d) TOTALLY FUCKED UP (honest ledger, this session)

Nothing shipped broken; all gates green at session end. Honest callouts:

1. **Trusted the stale session-start context snapshot at first.** It showed
   pre-rebase HEAD and phantom modified files; acting on it would have
   misdirected the AGENTS.md and annotation work. Caught on the first fresh
   `git status` — the exact "independently verify tool output" rule this
   repo's memory already records. The near-miss is the finding.
2. **Three edit collisions from unbatched sequencing.** The annotate
   scripts modify files after I read them; three subsequent hand edits
   failed with "file modified since read" and cost a re-read round trip
   each. Should have re-read each target immediately before its hand edit.
3. **Script scope edge case hit twice.** `annotate-prose.py` assumed §g
   items are numbered (19-09's "THE ONE QUESTION" is prose) and
   `annotate-rows`/prose section scoping stops at lines starting `##`,
   which lets a following `### Self-review questions` numbered list leak
   into scope (04-32 §g Q3 matched twice). Both fell back to correct hand
   edits; the skill asset has a genuine bug worth fixing upstream.
4. **`mcp_qmd_multi_get` failed twice on absolute paths** at skill-load
   ("File not found" for files that exist) — wasted two round trips before
   falling back to `view`.
5. **Formatted before finishing appends.** `dprint fmt` ran mid-pass (it
   correctly re-padded the annotated tables), which forced one extra
   fmt/check cycle after the final appends. Appends should have been
   batched before formatting.
6. **Coverage folklore propagated for weeks** — "~85%" sat in AGENTS.md
   across at least four sessions while the real numbers were one
   `go test -cover` away (and loader was actually 74.6%). I fixed it this
   session; the failure was everyone-before-me _and_ that I almost
   annotated the number instead of measuring it.

## e) WHAT WE SHOULD IMPROVE

1. **First command of every session: fresh `git status` + `git log`.**
   Context-block snapshots in this repo are stale by construction (rebase
   recovery, daemon commits). This session nearly built on one.
2. **Measure, don't annotate folklore.** Any concrete number in a living
   doc (coverage, perf, counts) gets re-derived in the audit pass, not
   verified "as documented".
3. **Harvest after every status report.** Eleven reports accreted unharvested
   next-task lists; pass 3 was the last session that actually updated
   TODO_LIST. The docs-health skill mandates it — make it a hard post-report
   step, not a later cleanup.
4. **Batch annotate writes + hand edits per file, format once at the end.**
   The annotate tools own their files; interleaving hand edits guarantees
   stale-read collisions.
5. **Fix the docs-health annotate scripts upstream (to the skill):** section
   scoping must stop at `###` too, and prose items need a non-numbered
   variant. Both gaps cost fallback hand edits this session.
6. **The `p` (docs-health pass) verdict kind is the right answer to
   "annotate needs a hash but the daemon owns commits."** Use it by default
   instead of deferring annotations until a hash exists.
7. **Keep single-homing facts.** Two reports asked to mirror AGENTS facts
   into PORTS.md; both marked Won't-implement this session (AGENTS is the
   machine-facing home, PORTS the user-facing one). Duplication is how the
   TS7 blocker went stale in the first place.

## f) Up to 50 things to get done next

_Impact-ordered. Items already carried in TODO_LIST.md/ROADMAP.md are
marked with their home — this list is the audit's prioritized view, not a
second backlog._

**Now / unblock everything:**

1. ~~Decide + execute the push (`--force-with-lease`; rebase rewrote 32
   commits) — ROADMAP Open decisions.~~ done (pushed; `main` == `origin/main` at `3da7d0f`) _(user-gated)_
2. ~~After push: watch build.yml (incl. the `3-stage-hmr` replay) and
   ports.yml; record green runs.~~ done (Build 34267336684 + Ports 34267336671 green on `3da7d0f`)
3. ~~Reconcile ROADMAP/FEATURES one week out: if CI confirms the replay,
   soften the "gate on Ports" gotcha in AGENTS.md.~~ done (CI confirmed 2026-09-08; AGENTS gotcha updated by the second docs-health pass)

**Parity (the fork's real work):**

4. Port-or-document `go/loader`+`go/hmr` decisions for #111 (three-stage
   reload), #121 (include journal), #123 (bare specifiers), #128
   (`hmr.watch()`) and the replayed commit-based `EntryTree.commit()`
   semantics. _(ROADMAP Go §1)_
5. ~~Golden scenario #4: bail/serial/waterfall dispatch parity across all
   three runners.~~ done at `72e1505`
6. Logger golden scenario. _(ROADMAP Go §3)_
7. ~~Golden scenario candidate: loader watch/reload trace.~~ done at `72e1505` (Go-only watch transcript, `go/loader/testdata/watch-golden.txt`)

**Go quality:**

8. ~~Push `go/loader` coverage from 74.6% toward the ~90% bar.~~ done at `72e1505` (90.8%, recorded in AGENTS.md)
9. ~~`go fix ./...` modernizer sweep under Go 1.27.~~ done at `72e1505`
10. ~~hmr: surface `Fiber.Err()` detail in rollback errors.~~ done at `72e1505`
11. ~~hmr: concurrency storm test (`Swap` vs `Tree.Create/Remove`).~~ done at `72e1505`
12. ~~loader: `Resolver.ReplaceType[C]` sugar.~~ done at `72e1505`
13. ~~`IntervalFunc` pump-goroutine lifetime: fix or document.~~ done at `72e1505`
14. ~~Timer property test: debounce/throttle fire counts.~~ done at `72e1505`
15. ~~Regression tests for the wrapped error messages (accessor/tree).~~ done at `72e1505`
16. ~~`Tree.Await`: surface the fiber error or document the discard as final.~~ done at `72e1505` (routes failures into the entry error sink)

**Rust quality:**

17. ~~`internal/plugin` + `internal/update` interception events.~~ done at `75fb408`, `25ff5fb`
18. ~~Root-fiber status emission: cover or document (`FiberData::new_root`,
    `rust/src/fiber.rs:82`).~~ done at `25ff5fb`
19. ~~`significant_drop` cleanup, then gate `cargo clippy --features
    thread-safe` in Ports.~~ done at `25ff5fb`
20. ~~`cargo-llvm-cov` baseline next to Go's numbers.~~ done at `25ff5fb` (86.4% lines)
21. ~~`cargo bench` to substantiate or hedge the "30% faster small
    allocations" ROADMAP claim.~~ done at `25ff5fb`

**Zig quality:**

22. ~~Registry `has`/`delete` by `TypedPlugin` identity.~~ done at `75fb408`
23. ~~`zig build -femit-docs` doc-comment pass.~~ done at `75fb408` (`zig build docs` gate)
24. ~~AGENTS.md Zig 0.16 std-gotchas section.~~ done at `75fb408`

**CI hardening:**

25. ~~CI job running `nix flake check`.~~ done at `fa45896`
26. ~~`.prettierrc` (printWidth 100) + `prettier --check` + `yarn build`
    before tests in build.yml.~~ **Won't implement — no prettier-stable style exists to pin; upstream style is CI-enforced by the `upstream-parity` semantic guard (`fa45896`).**
27. ~~CI guards: `packages/**` byte-parity vs upstream; dprint excludes cover
    `packages/**`.~~ done at `fa45896`
28. ~~Align the flake gate's `-race -count=1` with ports.yml's `-count=3`.~~ done at `fa45896`
29. ~~Gitignore `tmp-*` debris.~~ done at `fa45896`

**TS / upstream:**

30. ~~Install-from-scratch TS verification (`rm -rf node_modules && yarn
    install && yarn build && yarn test`).~~ done at `fa45896` (248/248)
31. eslint/oxlint: fix the invocation mismatch, drive findings to ~0.
    _(source: 04-32 §f4–5)_
32. Watch upstream `3-stage-hmr` merge; drop the fork's replay when
    upstream lands it. _(source: 15-43 §f23)_

**Docs / process:**

33. ~~Review CONTRIBUTING.md; document the "upstream semantics + fork
    formatting" rebase policy; add the flake app quickstart.~~ done at `fa45896`
34. ~~Loader: fuzz the JSON config layer.~~ done at `fa45896`
35. Cut GitHub Release pages for `go/v0.1.0` / `rust/v0.2.0`. _(source:
    03-03 §f3)_
36. Fix the docs-health annotate scripts: `###`-aware section scoping;
    non-numbered prose items. _(this session's finding)_
37. Check upstream `RequireNoResidue`/issue-#2 progress; update ROADMAP's
    Kernovia section. _(source: 05-27 §f25)_
38. Kernovia `cordisparity` re-run against the rebased oracle after the
    next tag. _(source: 04-32 §f49)_

**User-gated decisions (ROADMAP Open decisions):**

39. yarn.lock policy (commit generated lockfile vs lock-free).
40. TS toolchain stance (track upstream exactly vs fork-pinned).
41. ~~hmr fixture style (byte-identical vs style-agnostic specs).~~ done (resolved 2026-09-08: fixtures stay byte-identical to upstream, CI-enforced; ROADMAP open decision closed)
42. One-session-per-worktree convention for concurrent agents.
43. Oxlint policy for upstream TS (report-only today).
44. Generic-method API deprecation timeline (Go phase 3).
45. Release surface: tags only vs GitHub Release pages.

**Machine (outside the repo, listed for completeness):**

46. Fix the dangling `~/.cache/go-build` home-manager symlink.
47. Fix/mask the `/mnt/buildcache/go-build` default.
48. buildflow: filter enumerated nix checks to the running system.
49. Wire gopls/golangci-lint LSP to Go 1.27.
50. Provision govalid/erraudit via nixpkgs/home-manager instead of
    hand-rebuilt `~/go/bin` copies.

## g) Questions I cannot figure out myself

1. **Push:** everything is green (ports gate, flake check, lint battery)
   and the docs are reconciled — do you want the `git push
   --force-with-lease origin main` now? The rebase rewrote 32 commits;
   origin has nothing unique (verified by the recovery session, re-checked
   by me: no upstream-only work beyond what the rebase already carries).
2. **yarn.lock:** commit a generated lockfile to the fork (reproducible
   CI/installs, small permanent divergence, sync noise) or stay lock-free
   tracking upstream (this exact class of breakage has now bitten twice)?
   This also decides whether a renovate/dependabot-style TS dep watcher is
   even possible.
3. **Port parity for the upstream sync:** should `go/loader`/`go/hmr` port
   the replayed `3-stage-hmr` semantics (commit-based `EntryTree.commit()`,
   include journal analogues, `hmr.watch()` embedder API) as the next big
   work block — or do we declare the Go watcher layer "embedder-owned by
   design" and document the divergence in ROADMAP?

---

_Point-in-time snapshot. The (f) list overlaps TODO_LIST/ROADMAP by design —
those files are the backlog of record; this report is the audit's view of
it. Not committed (no commit requested); the auto-daemon is expected to
pick it up._
