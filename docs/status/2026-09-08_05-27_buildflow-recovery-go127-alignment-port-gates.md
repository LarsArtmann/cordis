# Session Status — Buildflow Recovery, Go 1.27 Alignment, Port Gates

- **When:** 2026-09-08, 05:27 CEST
- **Where:** `~/forks/cordis`, branch `main` (39 local-only / 31 origin-only commits — post-rebase divergence)
- **Session scope:** recovery of the interrupted 2026-09-08 session (Go 1.27 adoption patch, rebase
  completion, buildflow fallout), full verification of all port gates, lint suite repair, and root-cause
  work on every buildflow failure.

## TL;DR

All three port test suites and `nix flake check` are green. Buildflow went from **10 failed steps → 3**,
and the remaining 3 are one root cause that is a **buildflow tool limitation**, not a repo defect. Every
repo-side failure is fixed with a real fix or a documented, justified suppression. All work is staged
(37 files). The push to origin needs `--force-with-lease` (verified safe, awaiting approval). Two
self-inflicted dead-ends (a `go.work` attempt, a `builtins.currentSystem` filter) were caught and
reverted — noted honestly below.

## a) FULLY DONE

1. **Git state triage** — divergence 39/31 proven to be the rebase (origin's 31 commits are 1:1
   pre-rebase twins; no unique remote work). `README.md` typechange understood (HEAD = upstream
   symlink, worktree = fork-owned real file).
2. **README layout per user demand** — root `README.md` is the fork sales page (real file);
   `packages/core/README.md` restored **byte-identical to upstream `caab04e`** (verified by diff
   against `git show caab04e:...`, 534 bytes).
3. **Upstream tracking restored** — reverted dprint quote-churn in the four
   `packages/hmr/tests/*.yml` fixtures; `dprint.json` now excludes `packages/**` so formatters can
   never rewrite upstream files again.
4. **Go port green on Go 1.27** — build, vet, `-race` tests, golden parity; gofmt clean.
5. **Rust port green** — clippy (`pedantic`+`nursery` deny) + full test suite including doctests.
6. **Zig port green** — 29 tests across 3 modules, leak-checked.
7. **`nix flake check` passes** — go/rust/zig checks build and pass on x86_64-linux (incompatible
   systems skipped with warning, as designed).
8. **Root Go module stub** (`go.mod` + `doc.go`, no buildable code) — repo-root Go tooling resolves;
   fixes buildflow's `go-generate`, `govalid-generate`, `test-race` which run at root with `./...`
   (buildflow has no per-step workdir knob — verified via `buildflow config init`).
9. **erraudit remediation (8 findings, all fixed)** — errors wrapped with `%w` context in
   `go/accessor.go` and `go/loader/tree.go` (Create/Move, 4 CRITICAL context_loss); watcher `Close`
   errors now logged (`watch.go`); hmr rollback registration-restore failure now captured and
   reported (`hmr/hmr.go`); two deliberate discards (`logger.go` export, `tree.go` Await) suppressed
   with `//nolint:erraudit // reason` (directive verified working). Suite back to "No violations".
10. **Go 1.27-built tool binaries** — `govalid` and `erraudit` rebuilt with Go 1.27 into `~/go/bin`
    (PATH shadows the Go 1.26-built system copies whose x/tools skew broke analysis).
11. **Flake hardening** — `formatter` output added (nixfmt wrapped to accept directories, which
    `nix fmt` passes); `gcc` added to devShell (`-race` needs cgo); devShell `GOCACHE` moved from a
    `builtins.getEnv "HOME"` attr (empty under pure eval — real bug) to a `shellHook`; `go_1_27`
    pins in checks/devShell/apps.
12. **Lint suite fully clean** — markdownlint: 0 findings (config + `.markdownlintignore` covering
    frozen `docs/status|planning`, upstream-frozen `packages/core/README.md`, `node_modules/`);
    codespell clean (`appliable`, `childen` fixed); lychee clean (`PORTS.md` self-link fixed,
    packages/core README restored to upstream kills its 18 broken fork links).
13. **AGENTS.md truth restored and extended** — re-added five facts the interrupted session dropped
    (yarn `--no-immutable` install method, upstream hmr test flake, thread-safe borrow rule, Rust
    timeout rule, package.json diff rule) and documented all new facts (root module stub, devShell
    gcc/GOCACHE, buildflow knobs and limitation, erraudit/nolint convention).
14. **Vitest crash root-caused** — re-verified live: `yarn install` fails on the upstream
    `typescript ^7.0.2` dep (yarn 4.14.1 builtin compat patch lstat `lib/_tsc.js` → ENOENT); the
    partial stale `node_modules` has no working vitest. Upstream-side blocker; fork gates on Ports.
15. **All work staged** — 37 files (5 from the earlier session + this session's fixes).

## b) PARTIALLY DONE

1. **Buildflow gate: 50/57 with 3 failures, one root cause** — `nix-build` + `nix-hash-fix` cascade
   try to build `.#checks.aarch64-darwin.*` on x86_64-linux ("platform mismatch"). Repo-side
   workarounds are impossible: pure eval forbids `builtins.currentSystem` (Nix removed it), and
   emptying `checks` would gut `nix flake check`. The fix belongs in buildflow (filter enumerated
   checks to the running system, or skip platform mismatch). Documented in AGENTS.md with the full
   argument.
2. **TS suite** — root cause fully verified and documented, but the suite itself stays dead until
   upstream fixes its toolchain (or we deliberately diverge). Not fixable in-fork per the
   packages-track-upstream rule.
3. **Push** — verified safe (no unique remote content), command chosen (`--force-with-lease`), not
   executed: needs approval, and the staged work must be committed first.
4. **This status report** — written (it overrides the status-report skill's HTML default to `.md`,
   per your explicit filename request). `TODO_LIST.md` HARVEST from section (f) not yet run.

## c) NOT STARTED

1. The push itself (blocked on approval + commit).
2. Buildflow upstream fix (nix-checker system filter / platform-mismatch skip).
3. `TODO_LIST.md` harvest of this report's section (f).
4. Darwin/aarch64 verification of the flake checks (nothing on this machine can build them; CI
   `ports.yml` runs go/cargo/zig directly, so flake checks have never been exercised on darwin).
5. Cleanup of the partial `packages/core/node_modules` + `packages/hmr/node_modules` stale installs.
6. LSP wiring for Go 1.27 (gopls/golangci-lint LSP still run the system Go 1.26 with
   `GOTOOLCHAIN=local` and error on every go file — cosmetic, CLI is green).
7. Tool-binary provisioning strategy (govalid/erraudit currently hand-rebuilt in `~/go/bin`; not
   reproducible via flake/home-manager yet).

## d) TOTALLY FUCKED UP

Nothing unrecoverable, and no data was lost — but honest callouts:

1. **The interrupted session had silently deleted five hard-won AGENTS.md facts** during its
   reorganization (yarn install method, hmr flake, thread-safe borrow rule, timeout rule,
   package.json rule). I caught and restored them this session. Knowledge loss in the memory file
   is the most expensive class of fuckup in this repo.
2. **My `go.work` detour** — I added it before reasoning through workspace-mode pattern semantics;
   root `./...` still doesn't resolve outside the used modules. One wasted build+verify cycle, then
   replaced by the correct root-module-stub pattern.
3. **My `builtins.currentSystem` flake filter** — implemented, then hit the pure-eval ban (Nix
   removed it), reverted. I should have verified pure-eval compatibility before building on it.
   Worse, my verification pipeline (`nix flake check | tail -4 && echo FLAKE-OK`) printed a green
   banner through a failing run — the exact pipeline-masking trap my own memory warns about. The
   eval error was only caught when cross-reading buildflow logs.
4. **Pre-existing, upstream-side:** the TS toolchain (`typescript ^7.0.2` + yarn patch crash) keeps
   the entire TS test suite dead and caused the original vitest "worker crash" red herring.

## e) WHAT WE SHOULD IMPROVE

1. **Run new tools manually before trusting the gate's red/green** — erraudit's 8 real findings hid
   behind "detector analysis failed" for two buildflow rounds; a manual `erraudit ./...` in `go/`
   exposed them instantly. Same for govalid.
2. **Pipeline hygiene** — never `cmd | tail && echo OK`; use `PIPESTATUS`/`set -o pipefail` or read
   the log. (Memory rule exists; I violated it once this session.)
3. **Verify platform/language constraints before building on a mechanism** (`builtins.currentSystem`,
   workspace-mode pattern semantics) — 30 seconds of doc-checking beats a build cycle.
4. **Kill dev/prod environment skew at the flake level** — devShell now has gcc and a working
   GOCACHE; next step is provisioning govalid/erraudit/go-1.27-built tools the same way instead of
   hand-rebuilding into `~/go/bin`.
5. **Silence report-only noise that trains us to ignore warnings** — vulnix (153 env CVE findings),
   oxlint (6.9k upstream-TS style findings), pnpm-audit in a yarn repo: either scope them to what
   the fork owns or configure them off.
6. **Write status/memory docs during the session**, not after a demand.

## f) NEXT THINGS (brainstorm, impact-ordered tiers — HARVEST fuel, not commitments)

**Now / this session:**

1. Commit the staged 37-file batch; then `git push --force-with-lease origin main` (on approval).
2. Fix buildflow upstream: filter enumerated `.#checks.<system>.*` to the running system (or skip
   platform-mismatch errors) — turns the gate fully green.
3. ~~HARVEST this report's (f) into `TODO_LIST.md` / `ROADMAP.md` via docs-health.~~ done (docs-health pass this pass routed the harvest into TODO_LIST/ROADMAP)
4. ~~Trash the stale partial `packages/*/node_modules` installs so nothing pretends to be a TS suite.~~ **Won't implement — superseded — the TS suite is green since 69b9fd6; the installs are live, not stale debris.**
5. Watch Ports CI (`ports.yml`) after push and record the run links.

**Buildflow / tooling (Lars-tool side):**
6. buildflow config knob for per-step workdir (go-generate/test-race would not need the root stub).
7. buildflow: degrade `nix flake show` enumeration failures to warnings consistently (it already
does when the flake fails to eval — make foreign-system builds take the same path).
8. Ship go-1.27-built govalid/erraudit via nixpkgs/home-manager so PATH tools never skew again.
9. Add vulnix/cargo-audit/cargo-deny/lychee/codespell to the flake devShell (or mark N/A) to clear
the "9 tools unavailable" health-check noise.
10. Scope oxlint to fork-owned files only (exclude `packages/**` — upstream owns TS style).
11. Make pnpm-audit step language-detect yarn repos and skip (or config knob).
12. Reduce vulnix noise: scope to the flake's own derivations, not the whole store closure.

**Repo guards (regression prevention):**
13. CI check that `packages/**` stays byte-identical to upstream (diff against a pinned upstream
remote) — would have caught the yml churn and the core-README fork-ification automatically.
14. CI check that `dprint.json` excludes keep covering `packages/**`.
15. Add tests for the new error paths: hmr rollback `Replace` failure, `watch.go` close-failure
logging, accessor/tree error message shapes.
16. ~~Re-verify Go coverage (~85% claim) after the erraudit changes; record the number in AGENTS.md.~~ done (docs-health pass measured 2026-09-08 and recorded in AGENTS.md (core 91.7%, loader 74.6%))
17. ~~Exercise `nix run .#test` (aggregate app) end-to-end once, post-formatter changes.~~ done (ran green today during the docs-health verify pass)
18. Verify the flake checks on aarch64-darwin (CI runner or remote builder) — currently only
linux-executed.
19. Pin the flake's zig version explicitly if nixpkgs drifts from 0.16 (build.zig API risk).
20. Consider `nixConfig` or CI guard asserting the formatter attr exists (the nix-fmt step failure
mode that started this mess).
21. Suppress/justify remaining report-only TODO-check finding (`packages/core/src/fiber.ts:49`,
upstream file) or exclude packages/ from todo-check.
22. Sweep `go/` for other bare `_ =` discards erraudit doesn't flag; align style proactively.

**Upstream tracking:**
23. ~~Raise the TS7/yarn blocker with upstream cordiverse (or track their fix) — unblocks the whole~~ done (moot — upstream reverted to typescript ^5.9.3)
~~TS suite, vitest included.~~
24. ~~Track upstream's `3-stage-hmr` branch for the 11 flaky hmr tests; adopt when merged.~~ done (replayed onto the fork (b4650df))
25. Check upstream for `RequireNoResidue`/issue-#2 progress (Kernovia feedback loop) and update
ROADMAP's "Bidirectional feedback" section.

**Docs / knowledge:**
26. ~~Mirror the root-go.mod-stub explanation into `PORTS.md` (currently only in AGENTS.md).~~ **Won't implement — kept single-homed in AGENTS.md — PORTS.md is the user-facing architecture doc.**
27. ~~Annotate the two 2026-09-08 status reports with this session's outcome (docs-health ANNOTATE).~~ done (docs-health pass this pass annotates the 09-08 reports)
28. Record the buildflow nix-checker bug as an upstream issue reference in AGENTS.md once filed.
29. ~~Document the `//nolint:erraudit // reason` convention in PORTS.md's port-architecture section.~~ **Won't implement — kept single-homed in AGENTS.md repo-hygiene facts.**
30. Update ROADMAP's "Go 1.27 adoption" section with the erraudit remediation notes.

**Port quality (Go):**
31. Add a regression test asserting the new wrapped error messages (accessor/tree context).
32. Consider returning joined rollback errors from hmr `rollback` with `%w` chains preserved
(already done — add a golden-scenario op if cheap).
33. Review whether `Tree.Await`'s discard of `f.Await()` should instead surface the fiber error via
the loader's error sink (design question, currently deliberate).
34. ~~Keep `testing/synctest` bubble pattern for any new timing tests (already policy; add lint?).~~ done (AGENTS.md build section records the synctest pattern policy)
35. Re-check `-race -count=3` parity with CI (`ports.yml` uses count=3; local runs used count=1).

**Port quality (Rust/Zig):**
36. ~~Run the `thread-safe` Rust feature build again post-toolchain (clippy gated off it by config).~~ done (thread-safe suite green on 2026-09-08)
37. ~~Verify Zig golden embeds stay byte-identical after any `golden/` edits (none this session).~~ done (suites green today; embeds verified by zig build test)

**Environment / machine:**
38. Fix the dangling `~/.cache/go-build` home-manager symlink (root cause of the GOCACHE dance).
39. Fix or mask the broken `/mnt/buildcache/go-build` default on this machine.
40. Consider a repo-level `.envrc`/flake app that exports `GOCACHE=/tmp/gocache` for bare-shell use.
41. Wire gopls/golangci-lint LSP to Go 1.27 (kills the 8 recurring diagnostic errors in every edit).
42. Clean `/tmp/buildflow*.log`, `/tmp/cfgtest`, `/tmp/upstream-core-readme.md` scratch files.

**Strategy / hygiene:**
43. Decide the oxlint policy for upstream TS once and encode it (report-only today).
44. ~~Decide whether `FEATURES.md`/`docs/DOMAIN_LANGUAGE.md` need a freshness pass after the loader/~~ done (docs-health pass this pass refreshed both files)
~~hmr/hmr-rollback work (not touched this session).~~
45. Add the buildflow round-4 log excerpt to the buildflow upstream issue (repro evidence).
46. Evaluate `--all-systems` flake check in CI on a darwin runner to keep darwin checks honest.
47. Keep an eye on Nix deprecations (formatter naming, eval cache) — two warnings observed.
48. After upstream TS fix: full vitest pass, then re-baseline the "gate on Ports" policy.
49. ~~Confirm the auto-commit daemon produced clean, well-scoped commits for the staged batch.~~ done (recovery batch landed as well-scoped 51cddf2 + b4650df)
50. Schedule the next flake-lock refresh + go/rust/zig toolchain bump cadence (buildflow update
steps exist; verify they ran green this round).

## g) QUESTIONS I CANNOT ANSWER MYSELF

1. **Push approval:** May I run `git push --force-with-lease origin main` once the staged batch is
   committed? (Rewrites origin/main; verified nothing unique is lost — but it is your remote.)
2. **Buildflow fix path:** Do you want to fix the nix-checker system-filter bug in buildflow
   yourself, or should I prepare the patch/issue for it?
3. ~~**TS suite policy:** Keep gating on Ports only until upstream fixes `typescript ^7.0.2` (current~~ done (superseded — upstream reverted to ^5.9.3 and the fork matched; gate-on-Ports stands until CI confirms)
   ~~policy), or pin a TS downgrade in the fork despite permanent divergence cost in root/package~~
   ~~manifests?~~

_Point-in-time snapshot — will go stale. HARVEST (f) into TODO_LIST before it does._

---

## Resolution (docs-health pass, 2026-09-08)

14 §f items and §g Q3 resolved inline above. §f 1 is half-done: the staged
batch committed cleanly as `51cddf2` (+ `b4650df`), the push remains
user-gated (§g Q1, ROADMAP Open decisions). Still open: buildflow
platform-mismatch fix (2, §g Q2 — tool-side), post-push CI watch (5),
buildflow knobs and tool provisioning (6–12), CI byte-parity and dprint
guards (13–14), new-error-path tests (15), darwin checks (18), zig pin
(19), formatter guard (20), todo-check suppression (21), discard sweep
(22), Kernovia progress (25), ROADMAP erraudit notes (30), rollback golden
op (32), `Tree.Await` question (33), `-count=3` parity (35), GOCACHE and
LSP machine fixes (38–39, 41), scratch cleanup (42), oxlint policy (43),
buildflow issue evidence (45), darwin flake CI (46), nix deprecations
(47), post-fix vitest re-baseline (48), flake-lock cadence (50).
