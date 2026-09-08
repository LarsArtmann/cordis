# Status Report — Go 1.27 Toolchain Adoption & Concurrent-Session Hazard

- **Generated:** 2026-09-08 04:04 CEST (CLI `date`)
- **Session scope:** single task — "How could the Go version benefit from Go 1.27?" (research → implement → verify)
- **HEAD at write time:** `1fe5e3e` (chore: refresh Nix nixpkgs, relax Go toolchain pin, bump @types/node)
- **Session-file backups:** `/tmp/cordis-session-backup-20260908/` (go.mod, flake.nix, timer_test.go, plugin.go, ROADMAP.md, AGENTS.md)
- **⚠ Headline:** this repo is being mutated by at least one other concurrent session/daemon. Mid-session it went from "clean" → staged bulk diff → new commits → **unmerged conflicts wrapped around exactly the three files this session edited**. Nothing is lost (worktrees verified intact, backups exist), but the index is a hazard and needs an ownership decision.

---

## Self-Review (brutal, per request)

### What did I forget?

1. **To re-verify the foreign merge state before finishing.** I detected the UU conflicts mid-session, reported "stale conflict state, no MERGE_HEAD, confined to packages/*" — and never re-checked. At report time the conflicts had MOVED onto my own files (`go/go.mod`, `ROADMAP.md`, `go/timer/timer_test.go`). My final claim was stale within the hour. My own rule: independently re-verify before resting on an observation.
2. **TODO_LIST.md harvest.** The follow-up work items I discovered went only into ROADMAP/AGENTS. Per docs-health flow, section (f) of a status report belongs in TODO_LIST.md — not entombed in a timestamped file. Not done.
3. **dprint.** The other session introduced `dprint.json` (markdown/yaml/json plugins). I edited two markdown files and flake.nix without running `dprint check` — my edits may violate the formatter the repo just adopted.
4. **Local substantiation of the perf claim.** I encoded "up to 30% faster small allocations" (release-note claim, verified in source) into ROADMAP/AGENTS without running `bench_test.go` before/after locally. Verified-in-source, unverified-in-context. Should have been hedged or measured.
5. **Full CI-parity test invocation.** CI runs `go test -race -count=3 ./...`; I ran `-race -count=3` only for the timer package and `-count=1` for the full suite (flake check). Cheap to close.
6. **`go fix ./...`** — I grepped for modernizer targets instead of just running it under 1.27 (embedlit/unsafefuncs could still have hits my greps didn't model).

### What could I have done better?

- **Three failed edits** ("file modified since read") burned round trips because I edited files the foreign pass had touched without re-reading each one first. After the first mismatch I should have re-read all target files in one batch.
- **One stray tool call** (`read_mcp_resource` with placeholder arguments) — sloppy, wasted a round trip.
- **The generic-methods analysis** is sound on name-collision grounds but never compile-probed. I verified the language feature from release notes, not that a hypothetical differently-named generic method on `*Context` behaves as expected. Moot (I rejected adoption), but the ROADMAP "revisit" clause deserves a probe recipe.
- **Ambiguity triage on `go/go.mod`:** I superseded the foreign `go 1.26.7` staged pin without asking. I still judge that right (mechanical one-liner, core to the explicit task, reversible), but given the daemon + concurrent sessions were already visible, a one-line heads-up in the stream would have been safer communication.

### What could I still improve?

- Session-start git snapshot should be treated as immediately suspect in this repo: **poll `git status` before every write**, not once.
- Cross-port impact statement ("golden byte-identical ⇒ Rust/Zig unaffected") was inference from shared fixtures, not a re-run of their checks. Fine given zero rust/zig edits, but say so precisely.
- The synctest rewrite preserves the original tests' structure — good — but I didn't add a single NEW assertion that would have failed under the old real-clock flakiness (e.g. debounce ordering under load). Determinism was the win; coverage breadth unchanged.

### Skill's 11 questions, short form

1. Forgot: re-check merge state, TODO harvest, dprint, local bench. 2. Stupid we do anyway: real-sleep guards in other test files; "~85% coverage" unverified folklore in AGENTS.md. 3. Better: batch re-reads, earlier user ping on the collision. 4. Improve: poll repo state; measure claims. 5. Lied: no — but two claims were stale by delivery (merge locality) or unverified locally (alloc %). 6. Less stupid: single-writer convention or session lock for this repo. 7. Ghost systems: none introduced; `synctest` usage is integrated into the only timing-sensitive suite. 8. Scope creep: held (rejected generic methods, json/v2, simd, uuid with written rationale). 9. Removed something useful: no — CutLast replacement is behavior-identical (golden canary ×2). 10. Split brains: candidate — AGENTS.md now says "requires Go 1.27 / use go_1_27" while HEAD's flake says "relax Go toolchain pin"; intent unknown, flagged as question 2. 11. Tests: suite green, race-clean, golden deterministic, 10× timer sweep; gaps are CI-parity invocation and zero new assertions.

---

## a) FULLY DONE (verified)

| Item                                                    | Evidence                                                                                                                                                                               |
| ------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Go 1.27 release-notes research, primary-source verified | go.dev/doc/go1.27 fetched; generic methods, `synctest.Sleep`, `strings.CutLast`, stdversion vet, goroutineleak GA, json/v2, uuid, simd all confirmed against the page, not the summary |
| Codebase read before edits                              | typed.go, context.go, accessor.go, inject.go, plugin.go, timer.go, timer_test.go, flake.nix, ports.yml, ROADMAP.md, AGENTS.md                                                          |
| `go/go.mod` → `go 1.27`                                 | worktree verified `go 1.27` (line 3); 1.26.7 refuses module (expected gate); 1.27.1 builds                                                                                             |
| `flake.nix` → `go_1_27` (checks + devShell + apps)      | 3 sites; `nix build .#checks.x86_64-linux.go` green end-to-end                                                                                                                         |
| Timer suite → `testing/synctest` virtual clock          | 7 tests rewritten; **457 ms → 2 ms (≈228×)**; `-race -count=3` green; 10× sweep green at 3 ms; 0.00 s per test                                                                         |
| `strings.CutLast` in `funcName` (go/plugin.go:68)       | behavior-identical; golden canary ×2 byte-identical; daemon staged it (`M  go/plugin.go`)                                                                                              |
| ROADMAP.md "Go 1.27 adoption (2026-09-07)"              | adopted / deliberately-not-adopted (generic methods rationale) / considered-no-use (json/v2, goroutineleak, uuid, simd)                                                                |
| AGENTS.md updated                                       | Go bullet (1.27 requirement + synctest pattern) + environment gotcha (system 1.26.7 refuses module; use flake devShell or `nix shell nixpkgs#go_1_27`)                                 |
| Verification battery                                    | `go vet` clean; full suite green; golden ×2; timer ×10; `-race -count=3` timer; sandboxed flake check green; golangci-lint **0 issues** under 1.27                                     |
| Work-integrity protection                               | all 6 session-touched files backed up to `/tmp/cordis-session-backup-20260908/`; worktrees verified free of conflict markers                                                           |

## b) PARTIALLY DONE

1. **Go 1.27 feature mapping** — adopted 3 of ~10 relevant features; json/v2, goroutineleak, uuid, simd, struct-literal field selectors, generalized func-type inference were analyzed and consciously skipped, but only json/v2 + generic methods got written rationale. Field-selector/inference finds no current call-site (grep-level check only).
2. **Uncommitted session work in a hostile index** — content is done and verified, but `go/go.mod`, `go/timer/timer_test.go`, `ROADMAP.md` sit as **unmerged (UU) index entries** with my versions only in the worktree. `go/plugin.go` got auto-staged. Nothing is committed; the work is one foreign `git checkout`/resolution away from loss (backed up, but still).
3. **Timing-test modernization** — timer suite done; `loader/watch_test.go` (real-interval poll loop), `stdctx_test.go`, `fiber_test.go:95`, `coverage_test.go:248` real-time guards analyzed but untouched.
4. **Docs consistency** — ROADMAP + AGENTS updated; TODO_LIST.md, FEATURES.md, CHANGELOG.md not touched (CHANGELOG itself is a staged foreign addition).

## c) NOT STARTED (deliberately, this session)

- CI version-pinning decision (ports.yml still `go-version: stable`).
- `go fix ./...` execution under 1.27.
- `reflect.TypeFor[T]()` migration (3 gopls hints in typed.go), unusedparams/infertypeargs cleanups.
- `errors.AsType[E]` sweep (Go 1.26 API; error-modernization skill exists for exactly this).
- goroutineleak profile as a test gate (prototype only).
- bench_test.go before/after measurement on 1.27.
- TODO_LIST.md harvest; stale-status-report annotation pass.
- Anything Rust/Zig (untouched by design; their ROADMAP items remain).

## d) TOTALLY FUCKED UP

1. **The repo's git state (not authored by this session):** unmerged conflicts (`go/go.mod`, `ROADMAP.md`, `go/timer/timer_test.go`) whose worktree content is MY uncommitted work; bulk staged diff across packages/* + rust/*; daemon auto-committing (e.g. `1fe5e3e` committed the pre-session relaxed pin, message claims "no build logic changed" while touching flake.nix). Any naive `git commit` fails outright on unmerged paths; any foreign resolution could clobber this session's uncommitted edits.
2. **Two sessions driving the same toolchain surface in opposite directions:** HEAD message says "relax Go toolchain pin"; my worktree pins `go_1_27`. One of them is wrong; intent not determinable from inside the session → question 2.
3. **This session's own d-level item:** final answer of the previous turn asserted conflict locality ("confined to packages/*, which I don't touch") that was false by the time anyone read it. Not a lie — a stale snapshot presented without a freshness caveat. Lesson applied in this report (HEAD + fresh status captured at write time).

## e) WHAT WE SHOULD IMPROVE

1. **Single-writer discipline.** Two agents + a daemon mutated one tree concurrently. Either serialize sessions per repo, or partition by path with an announce-before-write convention.
2. **Verification freshness.** Any statement about git state gets a timestamp + re-check immediately before delivery.
3. **Claim provenance tiers.** Distinguish "verified in primary source", "verified locally", "unverified-in-context" in the text itself (the alloc-% claim should have carried the third tag).
4. **Formatter-aware edits.** Run `dprint check` on md/yaml/json edits now that dprint.json exists (it does not cover nix or go — gofmt stays authoritative for go/).
5. **Uncommitted work near conflict zones gets backed up before, not after, noticing danger.** (Backup happened this turn — better luck than process.)
6. **AGENTS.md folklore audit:** "~85% coverage" is asserted, never re-measured; coverage_test.go exists but the number may have drifted.

## f) 50 things we should get done next

**P0 — protect & unblock (before anything else)**

1. ~~Decide ownership of the UU conflicts on `go/go.mod`, `ROADMAP.md`, `go/timer/timer_test.go` and resolve them (my worktree versions are the verified ones; backups exist).~~ done at `51cddf2`
2. ~~Decide whether the foreign staged bulk diff (packages/_, rust/_) proceeds, and keep all other hands off until it commits or unwinds.~~ done at `51cddf2`
3. ~~Reconcile "relax Go toolchain pin" (HEAD intent) vs `go_1_27` pin (worktree) — pick one, align flake.nix + AGENTS.md.~~ done at `51cddf2`
4. ~~After resolution: re-run the full battery (vet, `go test -race -count=3 ./...`, golden ×2, `nix build .#checks.x86_64-linux.go`).~~ done (recovery session verified the full battery green; suites green on 2026-09-08)
5. ~~Commit the Go 1.27 adoption as one coherent commit (module bump, flake, synctest tests, CutLast, ROADMAP/AGENTS).~~ done at `51cddf2`
6. ~~Compare committed result against `/tmp/cordis-session-backup-20260908/` (integrity ritual).~~ done (performed per the resolution appendix — byte-identical or additive)
7. ~~`dprint check` on ROADMAP.md / AGENTS.md / any json edited; fix formatting drift from this session.~~ done (recovery session ran dprint fmt)
8. ~~HARVEST section (f) into TODO_LIST.md (docs-health HARVEST rules; P2/P3 items are ROADMAP-fuel, not commitments).~~ done (docs-health pass this docs-health pass routed §f into TODO_LIST/ROADMAP)
9. ~~CHANGELOG.md entry for the Go 1.27 adoption (file exists as foreign staged addition — coordinate).~~ done (docs-health pass CHANGELOG.md rebuilt by this pass)
10. ~~FEATURES.md: add toolchain + synctest-infra entries if feature-inventory-worthy.~~ done (docs-health pass FEATURES.md refreshed by this pass)
11. ~~Annotate 2026-09-04/05 status reports: superseded toolchain facts (docs-health ANNOTATE, non-destructive).~~ done (docs-health pass the 2026-09-04/05 reports annotated by this pass)
12. ~~Re-measure coverage; fix or re-confirm "~85%" in AGENTS.md.~~ done (docs-health pass measured 2026-09-08 — core 91.7%, group 90.6%, hmr 90.0%, timer 88.9%, loader 74.6%)

**P1 — CI / build hermeticity**
13. ~~Pin `go-version: 1.27.x` in ports.yml or write down the `stable` policy + failure mode.~~ done (stable policy recorded in ROADMAP; 1.26-refusal failure mode in AGENTS.md)
14. Add a pinned golangci-lint setup (action or nix) — the Lint step currently assumes the runner image; first green run is still pending per ROADMAP.
15. Confirm what `actions/setup-go` `stable` resolves to at run time; add a step echoing `go version`.
16. `nix flake check` full pass once the tree is clean; verify `aarch64-linux`/`aarch64-darwin` evaluate with `go_1_27`.
17. ~~Document in AGENTS.md that `nix build .#checks.*` uses the live worktree (`builtins.path`) and works on dirty trees — learned this session, written nowhere.~~ done (AGENTS.md carries the untracked-files/live-tree gotcha)
18. Document gofmt-vs-dprint split (dprint has no Go plugin; gofmt/golangci-lint own go/ formatting).
19. Consider a `golangci-lint` config file (currently defaults) reviewed against the how-to-golang required stack.
20. ~~Add the golden canary (×2) as a flake check step, mirroring ports.yml.~~ done (the flake go check runs the full `go test -race -count=3` suite, goldens included)

**P2 — Go 1.27 follow-through**
21. ~~Run `go fix ./...` under 1.27; triage embedlit/unsafefuncs/atomictypes/slicesbackward suggestions.~~ done at `72e1505` (the named analyzers: zero findings; rangeint/stringsseq/stringscutprefix/reflecttypefor/mapsloop applied)
22. ~~`reflect.TypeFor[T]()` migration in typed.go (3 sites; kills the gopls hints).~~ done at `72e1505`
23. Silence gopls `unusedparams` honestly (golden_test.go:131 `realm`, context.go:86 `name`) — rename to `_` or justify.
24. Fix `infertypeargs` hints (coverage_test.go:291, accessor.go:105).
25. ~~Bench `bench_test.go` on 1.26.7 vs 1.27.1; record numbers in AGENTS.md (substantiate or soften the alloc claim).~~ done at `25ff5fb` (interleaved A/B, ROADMAP numbers)
26. Investigate the ~1.0 s race-mode package overhead in the synctest timer suite (sandbox numbers).
27. Prototype `goroutineleak` profile as an opt-in leak gate in one package; keep or document-reject.
28. ~~`errors.AsType[E]` modernization sweep across go/ (go-error-modernization skill; avoid the sentinel-matching cargo cult).~~ done at `8efd0f6`, `72e1505`
29. `go mod tidy` under 1.27 (expect no-op; verify new require-block normalization doesn't touch the dep-free module).
30. Convert `loader/watch_test.go` to a synctest bubble (real-interval poll loop) if the fs-side permits.
31. Evaluate synctest for `stdctx_test.go` (StdContext renewal timing) and the remaining real-time guards (fiber_test.go:95, coverage_test.go:248).
32. Verify encoding/json v1-under-v2 didn't change any logger/exporter error-text expectations; add a pinning test if fragile.
33. Add a golden scenario exercising the TYPED API (`Provide[T]/Get[T]/On[E]/Emit[E]`) so all three ports prove type-keyed parity end-to-end.
34. PORTS.md: document the synctest timing-test pattern + analogues (Rust: none — document constraint; Zig: `std.testing`).
35. Compile-probe recipe for the "revisit generic methods" clause (a tiny example proving the collision and the interface limitation, committed as a comment-scoped note or test).
36. Decide + document minimum-toolchain policy for consumers of the go module (go.mod 1.27 gate) in go/README.md.
37. Check the module tagging/proxy story for the `go/` submodule (does the fork tag it at all?).
38. ~~Split-brain scan of parity matrix rows vs code (docs-health VERIFY), especially after the foreign rust commits.~~ done (docs-health VERIFY passes 2026-09-08 ×3; matrix corrected against code)
39. Add one new deterministic assertion the old real-clock suite could never make (e.g. debounce collapse of N bursts) — turn the synctest win into coverage, not just speed.

**P3 — Rust / Zig (untouched this session; noted state only)**
40. Verify the thread-safe Rust deadlock regression test for nested-borrow (`Fiber::name` case) exists and is named.
41. Review/coordinate the foreign unstaged `rust/src/*` modifications before they get committed.
42. Zig ROADMAP items remain open (registry view, serial/waterfall/parallel dispatch, RAII disposers, batch, effect labels).
43. Rust ROADMAP items remain open (parallel dispatch, Batch API `Context::batch`, config validation).
44. ~~Re-run `nix build .#checks.*.rust` and `.#checks.*.zig` after the index resolves (they were not run this session).~~ done (`nix flake check` green across sessions on 2026-09-08)
45. ~~Kernovia convergence commit (`a5f610d`) mentions upstream issues #1/#2 — verify those upstream items are reflected in the ports' ROADMAP.~~ done (ROADMAP §Kernovia convergence records the bidirectional feedback)

**P4 — smaller hygiene**
46. ~~Confirm ROADMAP "Repo: first green ports.yml run" gets unblocked post-resolution (it needs a push — user-gated).~~ done (pushed; Build + Ports green on `3da7d0f`)
47. ~~Verify `golden/README.md` (foreign-staged) still matches the GOLDEN_UPDATE workflow description.~~ done (workflow matches; runner matrix added by the 2026-09-08 docs pass)
48. ~~Document GOCACHE precedence (flake devShell GOCACHE vs machine's /tmp override) — two truths, one gotcha paragraph.~~ done (AGENTS.md environment gotcha)
49. ~~Confirm `.github/workflows/build.yml` (foreign-staged) doesn't regress the TS Build workflow's yarn install semantics (upstream-tracking rule).~~ done (Build green in CI on `3da7d0f`)
50. ~~Review whether `Member[V]` nil-receiver semantics (`Set` on nil returns ErrReadOnlyAccessor) are test-covered — noticed while reading accessor.go, never checked.~~ done (go/accessor_test.go:148 pins ErrReadOnlyAccessor)

## g) Questions I cannot figure out myself

1. **The UU conflicts on `go/go.mod`, `ROADMAP.md`, `go/timer/timer_test.go` wrap this session's uncommitted work.** Is the in-flight operation yours/another agent's and still active (I keep hands off + you resolve), or abandoned (I resolve by writing my verified worktree versions into the index)?
2. **What does "relax Go toolchain pin" (`1fe5e3e`) intend?** If the intent is "flake should not pin Go beyond nixpkgs default", my `go_1_27` flake pin contradicts it and I need a ruling: pin `go_1_27` (my choice, reproducible 1.27 semantics) or relax (your commit's direction)?
3. **CI version policy:** keep `go-version: stable` (auto-upgrades with the ecosystem, matches the fork's track-upstream ethos) or pin `1.27.x` for hermetic Ports CI (predictable, but needs manual bumps)?

---

_Format note: user explicitly requested `.md` at `docs/status/`; the status-report skill's canonical format is styled HTML — override honored per user instruction, flagged per skill spec. No commit: user did not request one, and the unmerged index makes committing impossible/hazardous. The brutal-self-review skill's separate HTML report at docs/reviews/ was folded into this file for the same reason._

---

## RESOLUTION APPENDIX (annotated 2026-09-08 ~06:00 CEST)

Section (g) answers arrived; the hazard is closed. Current HEAD: `61ec9f9`, working tree clean.

1. **Q1 (UU conflicts) → resolved by this session.** `git add` recorded my verified worktree versions for `go/go.mod`, `ROADMAP.md`, `go/timer/timer_test.go`; the concurrent recovery session then absorbed them into `51cddf2` ("Recover rebase fallout..."), and its own report landed as `61ec9f9`.
2. **Q2 (toolchain direction) → pin `go_1_27`, kept.** Ruling: `go.mod 1.27` + nixpkgs default at 1.26.7 means a relaxed pin breaks `nix build .#checks.*.go` today. HEAD's flake.nix carries `go_1_27` in all three sites (reformatted by nixfmt); revisit relaxing when nixpkgs default reaches 1.27.
3. **Integrity diff (backups vs HEAD):** `go/go.mod`, `go/timer/timer_test.go`, `go/plugin.go` byte-identical. `flake.nix` / `ROADMAP.md` / `AGENTS.md` carry additive-or-format-only deltas from the recovery session (nixfmt reflow, devShell `gcc` + shellHook GOCACHE fix, root Go module stub note, upstream rebase facts, "appliable"→"applicable"). Nothing of this session's work was lost or altered semantically.
4. **Verification re-run post-resolution (this session, not trusted from the other):** `go vet` + full suite green across all 6 packages (core, group, hmr, loader, timer ×2), golden canary green. `nix flake check` green is claimed by `61ec9f9`'s session.
5. **Cross-reference:** the same hours produced the other session's report (buildflow recovery, upstream rebase to `caab04e`, root Go module stub) — read `61ec9f9`'s status report alongside this one; the two document complementary halves of one tangled night.
6. **New info affecting section (f):** item 16 (aarch64 checks) is the known buildflow platform-mismatch blocker, upstream-side per the other report — re-route that item accordingly. The root `go.mod` stub answers part of item 37's module-layout question (repo root is a stub module; the port stays in `go/`).

Docs-health pass addendum (2026-09-08): §f P0 items 1–12 now carry inline
verdicts above. A second docs-health pass (later the same day) resolved
P1/P2/P4 items 13, 17, 20–22, 25, 28, 38, 44–50 (CI push + green runs,
`go fix`/`TypeFor`/`AsType` sweeps, allocator A/B numbers, golden canary
covered by the flake suite, GOCACHE + golden-README + Member-nil
verifications). The remaining P1–P4 items are tracked where they belong:
bounded work in `TODO_LIST.md`, ideas and user-gated decisions in
`ROADMAP.md` (Open decisions).
