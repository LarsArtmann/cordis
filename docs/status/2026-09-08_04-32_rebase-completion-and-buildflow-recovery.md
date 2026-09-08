# Status Report — Upstream Rebase Completion, Buildflow Recovery, Go 1.27 Convergence

- **Generated:** 2026-09-08 04:32 CEST (CLI `date`)
- **HEAD at write time:** `1fe5e3e` (rebase tip: "chore: refresh Nix nixpkgs, relax Go toolchain pin, bump @types/node")
- **Session scope:** user pasted a failing `buildflow` run (10 failed steps, 103 steps total) and asked for root-cause → fix → verify until everything works.
- **Format note:** user explicitly requested `.md` at `docs/status/`; the status-report skill's canonical format is styled HTML — override honored, flagged per skill spec.
- **No commit:** the user did not request one; per standing rule nothing was committed this session. All work sits in the worktree for review.

---

## TL;DR

The buildflow failures were **symptoms, not the disease**. The repo was mid-flight in an
**interrupted interactive rebase** (11 of 32 fork commits replayed onto new
`upstream/main` `caab04e`, 11 files sitting in raw conflict markers), while **two other
writers** touched the same tree: a concurrent session doing a Go 1.27 adoption (its
uncommitted work was in the worktree, later confirmed by its own status report at
`docs/status/2026-09-08_04-04_go-127-toolchain-adoption-and-session-state.md`) and
buildflow's auto-fixers, which had sprayed formatting/modernization edits over the
half-merged tree at 23:29 the previous evening.

This session: finished the rebase (all 32 picks), reconciled the Go 1.27 work and the
user-demand README restructure on top, fixed 65 rust clippy-deny lints that blocked the
flake's Rust check under rustc 1.97, added the missing flake `formatter`, and cleared
every doc/lint finding from the original buildflow log that lives in this repo.
`nix flake check` (Go + Rust + Zig) is fully green; `go test -race -count=3` (CI parity)
is green; markdownlint/lychee/codespell/dprint are clean.

**The one thing that is still broken:** `yarn install` — upstream's `typescript:
^7.0.2` ships only `bin/tsc` (no `lib/_tsc.js`) and yarn 4.14.1's builtin
`compat/typescript` patch hard-crashes on it. Diagnosed to the tarball level, documented,
not fixed (fix = TS pin or yarn bump — upstream toolchain decisions). Consequence:
**the TS test suite has NOT run against the rebased tree**; TS conflict resolutions are
verified structurally (no markers, byte-parity logic), not behaviorally.

---

## a) FULLY DONE

1. **Root-cause investigation.** Identified the interrupted rebase (`rebase-merge` state,
   11 `UU` files with live conflict markers) as the true cause behind vitest worker
   crashes, lint noise, and "cannot find main module" style failures — not any single
   tool config.
2. **Rebase conflict analysis.** Proved via identifier-set diffing that upstream's delta
   to all 11 conflicted TS files (loader/hmr/include trees) was format-only
   (double-quote + semicolon + prettier wrapping), zero new/removed identifiers — so the
   fork's pick-12 blobs were the correct resolutions. Confirmed `hmr.watch()` (#128) was
   already in the fork's base.
3. **Rebase completed.** All 32 picks replayed; two conflicts resolved
   (pick-12 batch; `build.yml` merged upstream's OS-matrix `runs-on` with the fork's
   `timeout-minutes: 20` caps). `main` now = fork history on upstream `caab04e`.
4. **User-demand doc restructure restored.** Root `README.md` is a real fork-owned file;
   `packages/core/README.md` byte-identical to upstream (verified with `diff` against
   `caab04e`'s blob); the AGENTS "Upstream facts" note re-applied. First attempt (a
   `git stash pop`) was wrong and fully recovered — see d) and the self-review.
5. **Go 1.27 adoption preserved and landed.** The concurrent session's uncommitted work
   (module `go 1.27`, flake `go_1_27`, `strings.CutLast`, `testing/synctest` timer tests)
   was saved to a patch, the tree restored to rebase state, then re-applied after the
   rebase with 3-way merge; three content conflicts resolved in favor of both intents
   (Go 1.27 + later-pick test rewrites). Verified: `ports.yml` already carries
   `go-version: stable`, so the ROADMAP claim is true.
6. **flake.nix fixed.** Added the missing `formatter` output (this was the exact
   `nix-fmt` failure in the buildflow log and it cascaded to `nix-build-verify`).
7. **65 rust clippy-deny lints fixed** under nixpkgs rustc 1.97 (pedantic+nursery are
   `deny` in Cargo.toml): `# Errors`/`# Panics` doc sections across events/fiber/plugin/
   service, first-paragraph doc splits, let-else rewrites, `map_or_else`/`unwrap_or_else`,
   `split_first` instead of slicing, `props: HashMap<String, ()>` → `HashSet<String>`,
   `wrapping_add`/`saturating_sub` for counters and depth, borrow-walk parent-chain lookups
   replacing `clone_from` (which I first broke, then fixed properly), scoped
   documented `#[allow]`s only where a real invariant justifies it (fiber arena,
   builder-misuse expect, typed-event panic contract).
8. **Rust tests green on both feature variants.** Default (50 tests) and `thread-safe`
   (53 tests) — including the golden byte-identical trace tests.
9. **`nix flake check` fully green** (Go vet+race tests on go_1_27, Rust
   clippy `--all-targets` + tests, Zig tests) — the exact gate that failed in the
   original buildflow run.
10. **`go test -race -count=3 ./...` green** (CI-parity invocation, closing a gap the
    concurrent session's self-review flagged).
11. **Doc lint debt cleared (the repo-owned parts of the buildflow log):**
    markdownlint 506 findings → 0 on living docs (new `.markdownlint.jsonc` +
    `.markdownlintignore`; CHANGELOG reformatted from its broken stub);
    lychee 19 broken links → 0 (PORTS.md `../ROADMAP.md` escape fixed); codespell
    findings fixed (`appliable`, `childen` ×4); ROADMAP `#2 (` ATX-heading trap fixed;
    three bare action-run URLs in TODO_LIST wrapped.
12. **Tool config aligned with the polyglot layout:** `.oxlintrc.json` now ignores
    generated `lib/`/`dist/` (was 6544 findings on build artifacts); `.buildflow.yml`
    added with correct excludes; AGENTS.md documents the mandatory
    `nix develop -c buildflow` invocation.
13. **Memory updated.** AGENTS.md gained "Upstream facts" (rebase outcome, upstream
    style switch, conflict-resolution doctrine) and "Repo hygiene facts" (lint config
    locations, buildflow invocation rule, rust nursery-drift expectation, TS blocker).
14. **dprint compliance.** `dprint fmt` run with the repo's own config (6 files);
    markdownlint re-verified clean after.

## b) PARTIALLY DONE

1. **TS verification.** The rebase's TS conflict resolutions are verified structurally,
   not behaviorally — `yarn install` is blocked by the TS7/yarn-patch clash, so no
   vitest run has executed against the rebased tree. The earlier vitest crash was
   almost certainly the conflict markers (workers died at spawn, 86 ms, 20 errors),
   but "almost certainly" is not a test run.
2. **Oxlint re-check.** `ignorePatterns` for `lib/`/`dist/` added (was 6544 findings on
   build artifacts) but oxlint was never re-run to confirm the drop.
3. **Thread-safe Rust clippy.** ~10 `significant_drop` nursery findings remain —
   pre-existing (clippy was only ever gated on default features), tests pass, documented
   honestly in AGENTS.md, but not fixed.
4. **eslint warnings.** The 71 "File ignored because of a matching ignore pattern"
   warnings from the log were not addressed (`.eslintrc.yml`/`.eslintignore` are
   upstream-owned; the warning is an invocation mismatch).
5. **Buildflow tooling gaps.** `cargo-audit`, `cargo-deny` are not installed on this
   machine (their buildflow steps fail with "no such command"); `pnpm-audit` runs
   against a yarn workspace. These are machine/tool-choice issues, documented in
   `.buildflow.yml` comments only partially.
6. **TODO_LIST harvest.** Per the status-report skill, section (f) below belongs in
   `TODO_LIST.md` via docs-health HARVEST. Not done yet.
7. **Stash/tmp hygiene.** `stash@{0}` (pre-resolution backup) is kept intentionally but
   is now superseded; `/tmp/go127-adoption.patch` and the concurrent session's
   `/tmp/cordis-session-backup-20260908/` are not cleaned up or archived.

## c) NOT STARTED

1. TS pipeline repair decision (TS pin vs yarn bump vs drop to upstream-exact deps).
2. Filing the yarn/TS7 builtin-patch bug upstream (premise verified at tarball level;
   `verify-before-filing` deep-dive not done).
3. `go fix ./...` modernizer sweep under Go 1.27 (concurrent session's flag).
4. Benchmark run to substantiate the "up to 30% faster small allocations" claim the
   concurrent session encoded into ROADMAP/AGENTS (verified-in-source, not
   verified-in-context).
5. First green `ports.yml` CI run on GitHub (long-standing TODO_LIST item; everything is
   locally green, nothing pushed).
6. Go release/tag (`go/v0.1.x`) including the 1.27 bump; Kernovia oracle re-pin.
7. Content refresh of the fork sales page (root README status column predates several
   shifts), FEATURES.md refresh, DOMAIN_LANGUAGE.md post-rebase check.

## d) TOTALLY FUCKED UP

1. **The state I inherited was genuinely fucked:** an interrupted rebase with 11 raw
   conflict-marker files, two concurrent writers, and buildflow auto-fixers that had
   mutated a half-merged tree (including a failed `go-auto-upgrade` migration that
   "restored 3 files from pre-migration backups"). Nothing in the pasted log could be
   taken at face value.
2. **My `git stash pop` botch (worst in-session mistake).** I stashed three doc files
   with `git stash push -- <paths>` without realizing stash snapshots the _full tree_;
   popping it tried to drag pick-12-era content across 20 picks and littered conflicts
   across 15+ files. Recovery was clean (full restore to HEAD, surgical re-apply of the
   three files), but it confused the concurrent session — its status report describes
   "unmerged conflicts wrapped around exactly the three files this session edited",
   which was my mess. Preventable: `git show stash@{0}:<path>` from the start.
3. **I broke `Fiber::update` with my own edit.** A multiedit old_string swallowed the
   function body and the impl-closing brace; caught on the next view, repaired, but it
   was careless string surgery under momentum.
4. **I broke compilation applying a lint suggestion blindly.** `data.clone_from(&d.parent)`
   inside a `while let Some(d) = data` loop that _moves_ the variable — compile error
   E0382. The correct fix (borrow-walk, no clone at all) was obvious once I thought about
   ownership; I pattern-matched the lint message instead of the code.
5. **I wrote a literal `\n` into a doc comment** (double-escaped string in an edit
   payload), then needed three attempts to remove it because my own cleanup patterns were
   wrong twice. Embarrassing, harmless, noisy.
6. **I removed `BorrowExt` imports from three test files based on the default-features
   build alone**, breaking the `thread-safe` build (`Rc`/`RefCell` alias to
   `Arc`/`Mutex` needs the trait). The repo's own AGENTS.md documents the thread-safe
   axis; I ignored the feature matrix until the failing variant test caught it.
7. **The identifier-diff proof has a known hole.** "Zero new/removed identifiers" cannot
   detect behavior changes that reuse the same names (changed defaults, swapped argument
   order, altered string literals). The format-only conclusion is very likely right
   (byte-level diff review of hunks agreed) but it is not proof, and the TS suite that
   would close the gap can't run (see b1).

## e) WHAT WE SHOULD IMPROVE

1. **One-writer-at-a-time discipline.** Three writers (rebase session, Go 1.27 session,
   buildflow) on one worktree is how we got here. Adopt a convention: a session claims
   the repo (even a `.git/SESSION_LOCK` note file), others wait or read-only.
2. **Feature-matrix reflex.** Every "unused/dead" judgment in this repo must check
   `{default, thread-safe} × {lib, tests}` before acting — the BorrowExt incident is the
   template case.
3. **Verify behavioral claims with the real gate, not proxies.** "No conflict markers +
   lint clean" ≠ "tests pass". The TS blocker should push us to get the TS pipeline
   (or an alternative install path) working precisely because it is the only behavioral
   check for `packages/`.
4. **Clippy nursery + `deny` is a recurring tax.** Every toolchain bump re-breaks the
   build (this session: 65 findings). Either budget a pay-down after each bump (we did it
   twice now) or demote nursery to warn with documented rationale. The repo currently
   pays the tax silently on the `thread-safe` variant (never gated).
5. **Doc claims need receipts.** ROADMAP now asserts perf numbers and CI settings; I
   verified `go-version: stable` but not the benchmark claim. Claims without a
   command + output attached rot into lies.
6. **CHANGELOG is a lying stub.** "[0.1.0] - 2026-01-01 — Initial release" describes a
   release that never happened in that shape. Populate from real history or empty it
   until the first real release.
7. **Point-in-time snapshots of git state should be re-verified before acting** (global
   rule already says this; the env snapshot at session start showed a stale `main`
   state). First command of every session: `git status` + `ls .git | grep -i rebase`.
8. **The dual TS linter stack** (upstream `.eslintrc.yml` + fork `.oxlintrc.json`) will
   drift; document which rules each owns or converge on one.

## f) Up to 50 things to get done next

_Brainstorm, impact-sorted within tiers — ROADMAP/TODO_LIST fuel, not commitments
(docs-health HARVEST should route these)._

**Blocked-critical (unblocks verification):**

1. ~~Decide + implement TS install fix (pin `typescript` below 7.x in a fork-controlled~~ done at `eb831a7`
   ~~resolutions block, bump yarn in a test branch, or drop deps to upstream-exact) and~~
   ~~get `yarn install && yarn build` green.~~
2. ~~Run loader/include/hmr vitest suites against the rebased tree (closes the only~~ done (TS 248/248 green (69b9fd6 session))
   ~~behavioral gap in the rebase).~~
3. ~~Run full `yarn test` + `type-check` and reconcile with the known upstream hmr flake~~ done (yarn install && yarn build && yarn test green — 248/248)
   ~~(gate on Ports per AGENTS).~~
4. Re-run oxlint after `ignorePatterns`; drive remaining findings to ~0.
5. Fix the 71 eslint "ignored pattern" warnings (align invocation with `.eslintignore`).
6. Run `nix develop -c buildflow` end-to-end and chase a green run.
7. Install `cargo-audit`/`cargo-deny` (or disable those steps in `.buildflow.yml`).
8. Resolve pnpm-audit vs yarn in buildflow config.

**Commit/ship (small, high certainty):**
9. ~~Commit Go 1.27 adoption as one coherent commit (module, flake, synctest, CutLast,~~ done at `51cddf2`
~~ROADMAP/AGENTS).~~
10. ~~Commit rust clippy pay-down as its own commit.~~ done at `51cddf2`
11. ~~Commit doc-lint configs + fixes as their own commit.~~ done at `51cddf2`
12. ~~Commit README/AGENTS user-demand restructure.~~ done at `51cddf2`
13. Review + drop `stash@{0}`; archive or delete `/tmp/go127-adoption.patch` and
`/tmp/cordis-session-backup-20260908/`.
14. Push `main` (force-with-lease, user approval required — rebase rewrote 32 commits).
15. Trigger/watch the first green `ports.yml` run on GitHub.

**Upstream/outbound:**
16. ~~File the yarn builtin-typescript-patch vs TS7 layout bug (repro verified).~~ done (moot — upstream reverted to typescript ^5.9.3 themselves)
17. ~~Ask upstream cordis how CI installs with `typescript: ^7.0.2` (their lockfile-free~~ done (moot — upstream main pins ^5.9.3)
~~flow should hit the same crash) — or discover they already pin something.~~
18. Consider contributing the `build.yml` timeout caps upstream.

**Rust quality:**
19. Fix or explicitly allowlist the ~10 `significant_drop` thread-safe findings with
lock-scope rationale comments.
20. Gate `cargo clippy --features thread-safe` in Ports once clean (stop silent rot).
21. Decide on a rust toolchain pin (`rust-toolchain.toml`) vs paying nursery drift per
nixpkgs bump.
22. Run `cargo-llvm-cov` and record a coverage baseline next to Go's ~85-90%.
23. `cargo bench` for the "30% faster small allocations" claim; attach numbers to
ROADMAP or hedge the text.
24. Review `snapshot.rs` `start_base(&base, ...)` call for ownership clarity (my signature
change rippled there; it compiles and passes, a second pair of eyes is cheap).

**Go quality:**
25. `go fix ./...` sweep under 1.27 (embedlit/unsafefuncs hits).
26. ~~Re-run coverage, update the "~85-90%" doc claims with fresh numbers.~~ done (docs-health pass measured 2026-09-08 — core 91.7%, group 90.6%, hmr 90.0%, timer 88.9%, loader 74.6%)
27. Extend golden scenarios (#4+: events, logger — ROADMAP already lists this).
28. ~~Verify `golangci-lint` in ports.yml actually runs clean under Go 1.27.~~ done (golangci-lint 0 issues under Go 1.27 (04-04 session))
29. Run the zig suite standalone (`zig build test`) once outside the flake for parity.
30. Tag `go/v0.1.x` including the 1.27 bump; re-pin Kernovia's oracle (ADR-004 flow).

**Docs:**
31. ~~Harvest this report's (f) into `TODO_LIST.md` (docs-health HARVEST).~~ done (docs-health pass this pass routed §f into TODO_LIST/ROADMAP)
32. ~~Fix or empty the lying CHANGELOG stub (fake `[0.1.0] - 2026-01-01`).~~ done (docs-health pass CHANGELOG.md rebuilt from real history by this pass)
33. ~~Refresh root README status table (coverage, port statuses post-rebase).~~ done (docs-health pass README status table/example refreshed by this pass)
34. ~~Refresh FEATURES.md against the post-rebase tree.~~ done (docs-health pass FEATURES.md rewritten with canonical statuses by this pass)
35. ~~docs-health VERIFY pass over `docs/status/` history + ANNOTATE the 04-04 report~~ done (docs-health pass this pass verified and annotated the 04-04 report)
~~(its "unmerged index hazard" items are now resolved).~~
36. ~~Check `docs/DOMAIN_LANGUAGE.md` against current terms after the upstream merge.~~ done (docs-health pass terms verified/added by this pass)
37. Revisit MD013=off policy: consider a higher line-length limit instead of fully off.
38. Read `CONTRIBUTING.md` (added by pick 12) for accuracy — never reviewed.

**Repo hygiene / infra:**
39. ~~`.gitignore`: ensure `.buildflow-traces/`, `result*` (nix) are covered.~~ done (result*/.buildflow artifacts covered by the buildflow-managed gitignore block)
40. Add a CI job running `nix flake check` so the flake gate is enforced remotely, not
only locally.
41. Session-lock convention for concurrent agents (see e1) — even a convention line in
AGENTS.md.
42. Investigate/configure the auto-daemon to no-op during rebases (it staged files
mid-rebase this session).
43. Reconcile `.eslintrc.yml` (upstream) vs `.oxlintrc.json` (fork) ownership in AGENTS.
44. ~~`builtins.getEnv "HOME"` in the devShell `GOCACHE` breaks under pure eval — make it~~ done at `51cddf2`
~~robust.~~
45. Look at flake `checks` `meta` warnings (apps lack `meta` attr — cosmetic).
46. ~~Confirm `.yarn/` (install-state) is properly ignored; no yarn.lock must ever be~~ done (.gitignore covers yarn.lock and .yarn/*)
~~committed (upstream policy).~~

**Strategic:**
47. Decide the fork's TS toolchain stance: track upstream exactly (their versions, their
breakage) vs fork-modernized (current state) — this decision unblocks 1-3.
48. ~~Evaluate adopting the `3-stage-hmr` upstream branch for the known hmr flake.~~ done (replayed onto the fork (b4650df))
49. Kernovia convergence: run their `cordisparity` scenarios against the rebased oracle
after the release tag.
50. ~~Schedule a full docs-health audit after the commit wave (a-g reports exist for three~~ done (docs-health pass this pass is that audit)
~~consecutive sessions now; the living docs need one consolidation pass).~~

## g) Questions I cannot figure out myself

1. **Concurrency/ownership:** Was the 04:04 Go-1.27 session an intentional parallel
   agent of yours, and do you want a session-lock convention (or a "one writer per
   worktree" rule) recorded in AGENTS.md — or is the current "whoever survives, merges"
   mode acceptable to you?
2. **Commit/push policy for this session's work:** I have five clean logical layers in
   the worktree (Go 1.27 adoption; rust clippy pay-down; doc-lint configs+fixes;
   README/AGENTS restructure; `.buildflow.yml`/oxlint config). Commit them separately,
   let your auto-daemon do it, or leave uncommitted? And after committing — push
   `main` with `--force-with-lease` given the rebase rewrote 32 commits?
3. ~~**TS toolchain stance:** Should the fork keep its ahead-of-upstream devDeps~~ done (superseded — upstream reverted to ^5.9.3 and the fork matched it, except a deliberate @types/node ^26.5.0 bump; the stance question is parked in ROADMAP Open decisions)
   ~~(typescript ^7.0.2, vitest ^5, eslint ^10) and work around the yarn/TS7 crash~~
   ~~locally, or drop `packages/` deps back to upstream-exact (`typescript ^5.9.3` etc.)~~
   ~~until upstream moves — knowing the former keeps a broken install until resolved and~~
   ~~the latter contradicts the fork's own recent "dep bumps" commits?~~

---

### Self-review questions (brutal-self-review checklist, answered)

1. **Forgot?** To re-check git state after each phase (the UU-after-apply state sat
   unnoticed through several verifications because flakes/tests read the worktree, not
   the index); to run the thread-safe feature variant before declaring imports dead; to
   re-run oxlint after adding ignores.
2. **Stupid thing we do anyway?** Denying unstable nursery lints on a floating rustc and
   never gating the thread-safe variant — we pay either in surprise breakage or silent
   rot. Also: no session lock with multiple concurrent agents.
3. **Done better?** The stash (used `pop` instead of surgical `git show` extraction);
   the `Fiber::update` edit (should have replaced the whole function, not hand-matched);
   string-escape hygiene in edit payloads.
4. **Still improve?** TS behavioral verification (the big one); thread-safe clippy;
   oxlint/eslint residual noise; doc claims with receipts.
5. **Lied?** No. The one claim I'd soften: "clippy clean" means default features on
   rustc 1.97; thread-safe nursery findings exist and were disclosed.
6. **Less stupid?** See e) items 1-8.
7. **Ghost systems?** `docs/reviews/` referenced by the concurrent session's report was
   folded into its status file (no orphan); `.eslintignore` + `.oxlintrc.json` overlap is
   the only near-ghost (e8). The stash and /tmp artifacts are scheduled for cleanup (f13).
8. **Scope creep?** One contested call: 65 clippy fixes were beyond "fix buildflow", but
   they blocked `nix flake check` — the repo's actual gate — so they were in-scope by
   consequence. Doc lint configs were direct findings from the pasted log. No fishing.
9. **Removed something useful?** The `BorrowExt` imports (wrongly — restored); nothing
   else removed. `drain(..)` → `into_iter()` and `HashMap<(), ()>` → HashSet changed
   types, not behavior (golden + tests confirm).
10. **Split brains?** ROADMAP's Go-1.27 section vs CI: verified true (`go-version: stable`
    present). CHANGELOG's fake release date is a live split brain (f32). AGENTS env
    gotcha vs hygiene section have mild overlap (f49).
11. **Tests?** Ports fully green including race and count=3 CI parity; golden traces
    byte-identical across all three ports. Gap: TS suite unrun (blocked), thread-safe
    clippy ungated, rust coverage unmeasured.

---

## Resolution addendum (docs-health pass, 2026-09-08)

22 §f items and §g Q3 resolved inline above. Note two items overtook
events: the TS7 blocker is moot (upstream reverted to `^5.9.3`; the fork
installs and tests 248/248), and `3-stage-hmr` was replayed onto the fork
(`b4650df`) rather than awaited. Still open: oxlint/eslint re-runs (4–5),
buildflow platform-mismatch fix (6), cargo-audit/cargo-deny/pnpm-audit
tooling (7–8), stash//tmp cleanup (13), the user-gated push (14) and
post-push CI verification (15), timeout-caps contribution upstream (18),
thread-safe clippy work (19–20), toolchain pin (21), coverage baseline
(22), bench claim (23), snapshot.rs review (24), `go fix` sweep (25),
golden scenarios #4+ (27), zig standalone run (29), post-1.27 tag +
Kernovia re-pin (30), MD013 policy (37), CONTRIBUTING review (38),
`nix flake check` CI job (40), session-lock convention (41), daemon
rebase behavior (42), linter ownership (43), checks meta warnings (45),
TS stance decision (47), Kernovia re-run (49).
