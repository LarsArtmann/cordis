# Status Report — Paper Review → Unload-Guard Fix (Go), Dual-Mode Async Design

Session: 2026-09-10, ~06:30–08:34 · Branch: `main` · My commit: `69e4e2f`
(pushed). Concurrent session's commits interleaved: `6fbc1b4`, `75b75c3`.
Scope: this session only — paper analysis of arXiv 2608.25512, ports
assessment, the Go unload-guard fix, and the sync/async design question.

---

## a) FULLY DONE (verified)

1. **Paper deep-read** (arXiv 2608.25512, 92 pp., fetched + extracted):
   identified the real insights (effects/coeffects as runtime plugin
   semantics; pointwise one-sided inverses; observational equivalence as
   the honest meaning of "reverted"; commutativity decomposition;
   confluence Thm 80; boundary honesty) vs the weak parts (elementary
   math, unchecked witnesses, observational case study).
2. **Ports-vs-paper audit**: mapped every paper construct to Go/Rust/Zig;
   surfaced the unload-guard divergence class, `check()` parity gap,
   interception-events and Zig-tail gaps, missing confluence tests.
3. **Divergence sharpened by evidence**: initial claim ("guard missing
   everywhere, dependents always settle after provider") corrected after
   reading the golden traces — settle-inside-the-API-call already worked;
   the true divergence was the _interleaving point_ (dependents before
   the provider's remaining bag). This narrowing is what made the fix
   golden-byte-safe.
4. **Go unload guard implemented and proven**:
   - `core.settlePending()` extracted from `leave()`'s drain loop
     (`go/core.go`); provide cleanup calls it after notify + service
     event (`go/reflect.go`) — the synchronous counterpart of upstream's
     `await Promise.allSettled`.
   - 4 tests (`go/teardown_order_test.go`): fiber dispose, dependency
     chain leaf-first, direct effect disposal inside `Batch`, restart.
   - **Vacuity-checked**: all scenarios fail with the old ordering
     (`pool destroyed` before dependents; restart case loses the
     consumer teardown from the window), pass with the fix.
   - Full suite green, `go vet` clean, `go test -race .` clean (root
     pkg), **all golden traces byte-identical** (incl. loader
     watch-golden) — no `GOLDEN_UPDATE` needed, Kernovia oracle safe.
5. **Plan file** `docs/planning/2026-09-10_07-55_unload-guard-parity.md`:
   Pareto tiers (1%→51 / 4%→64 / 20%→80 / other-80), L1 (30–100 min) and
   L2 (≤12 min) task tables, mermaid execution graph, frozen-tree
   contingency, plus the **dual-mode sync/async design track** (§10).
6. **Cross-session coordination executed correctly**: detected the
   concurrent session's ownership of rust/** and zig/**, never touched
   their dirty files, committed with an explicit pathspec so their staged
   `rust/src/logger.rs` stayed staged for them. Their commit `75b75c3`
   subsequently landed with my CHANGELOG entry and their follow-up #16
   ("port settlePending semantics to Rust/Zig") — handoff closed.
7. **Two knowledge corrections, verified**:
   - Our pinned Zig 0.16.0 **does** ship colorless async
     (`std.Io.async`/`concurrent`, `Io.Threaded`/`Io.Evented`) —
     verified in the nix store std source, not just the article.
   - Rust libraries can be executor-agnostic async (expose futures);
     the constraint is only "don't pick the executor".
     Both corrections are baked into the plan's dual-mode track.

## b) PARTIALLY DONE

1. **Rust/Zig unload guard**: 0% code, 100% design (exact mirror sites
   identified: `rust/src/service.rs:106-111` closure,
   `zig/src/cordis.zig` Removal.run + `Core.leave` drain extraction).
   Blocked all session by the concurrent session's dirty trees; **now
   unblocked** — `75b75c3` landed 08:33, tree clean.
2. **CHANGELOG entry**: authored, landed via the other session's commit
   (verified in `75b75c3`). ROADMAP divergence/promotion note: authored
   in my head, _not_ applied — their session dirtied ROADMAP mid-edit;
   my multiedit correctly refused. ROADMAP still lacks the guard row and
   the Rust/Zig task promotions.
3. **Synchronous-settle trade analysis**: delivered (stronger
   postcondition, language-forced, paper-blessed inertial host; costs:
   no async bodies, concentrated cascade latency, root cause of the
   dropped guard). The _mitigation_ (LongRunning helper) is only
   designed, not built.

## c) NOT STARTED

1. Golden `scenario-guard.txt` (three-runner cross-language pin of the
   guard ordering) — plan L1.6.
2. Confluence test (paper Thm 80: different op orders → identical
   settled state) — untested in every port. Plan L1.7.
3. `check()` readiness gate for Rust/Zig provide. Plan L1.8.
4. Dual-mode async track: Zig `std.Io` injection spike, Rust async
   surface spike, Go `LongRunning` helper. Plan §10.
5. Loader rc.10 parity reassessment; Algorithm-7 realm migration.
6. AGENTS.md update for the settlePending pattern (their tree was dirty;
   never applied).

## d) TOTALLY FUCKED UP (nothing destructive; three judgment errors)

1. **Announced the broad divergence before checking the goldens.** My
   first verdict ("dependents settle after the provider's entire
   teardown — guard missing") was overstated; the golden trace
   (`cleanup consumer` before `withdrawn config`) later proved the
   API-call settle already worked. Correct sequence: read the pinned
   traces _first_, then make claims. The user read the wrong version.
2. **Two stale-knowledge assertions, both user-corrected:**
   "Zig 0.16 has no async at all" (false — `std.Io` async shipped in
   0.16.0-final; verified only after the user pushed the dev.to link)
   and "Rust can't ship async in a library" (overstated — the real rule
   is executor-agnosticism). I violated my own verify-before-claiming
   discipline on toolchain facts I "remembered".
3. **Attempted a ROADMAP edit without re-checking file freshness** —
   the concurrent session had rewritten it 30s earlier; the multiedit
   failed safely (no damage), but the round trip was avoidable: I knew
   a second session was live, so any shared doc needed a freshness
   check at edit time, not just at read time.

Minor waste: three mis-aimed file reads on the Zig side (stale line
numbers from an earlier grep after their edit shifted the file, wrong
working dir for one nix invocation). Each cost one round trip.

## e) WHAT WE SHOULD IMPROVE (process, from this session's scars)

1. **Golden-first verification rule**: before declaring any behavioral
   divergence between ports and TS, read the golden traces that pin
   that behavior. They encode settled decisions cheaply.
2. **Toolchain facts expire**: pin-era claims ("Zig has no X") must be
   re-verified against the _pinned_ toolchain (nix store std source),
   not memory. Cheap check, prevented two public wrong claims.
3. **Shared-tree edit protocol**: with a concurrent session live,
   re-stat files (`git status` / mtime) immediately before every edit
   to shared docs; pathspec-isolate commits (this worked — keep it).
4. **Pin the dangling question**: mid-analysis I noticed a potential
   "lost transition" (fiber popped from `dirty` while `executing`
   early-returns and may never re-queue) and reasoned it was
   pre-existing — but wrote no test and no note. It is now in the
   follow-up list (item f/22); dangling observations should become
   list entries the moment they're noticed.
5. **Race-test matrix**: I ran `-race` only on the root package
   (defensible — the diff is there), but the flake app runs the full
   matrix; match it for concurrency-adjacent diffs.
6. **Coverage delta**: new code paths landed without re-measuring the
   ≈90% statement-coverage baseline. One `go test -cover` run would
   have kept the recorded number honest.

## f) NEXT — up to 50, ordered by impact (top ≈ Pareto)

**Unblocked right now (tree clean since 08:33):**

1. Port settlePending guard to Zig (`Core.leave` drain extraction +
   Removal.run call) + parity test (their follow-up #16, mine to take).
2. Port guard to Rust (provide_inner closure) + parity test, both
   feature variants.
3. Golden `scenario-guard.txt` — new ops if needed in all three
   runners; byte-identical expected trace; update golden/README matrix.
4. ROADMAP: add the guard row (fixed-in-Go date, Rust/Zig status),
   promote it to top of both ports' task lists, refresh matrix rows
   their session landed (intercept/logger/snapshot for Zig).
5. Re-run all three suites + `nix flake check` post-`75b75c3` (their
   landing changed both other ports; verify the combined tree).
6. AGENTS.md: settlePending pattern + "guard is port semantics" note.

**Paper-driven gaps (highest intellectual value):**
7. Confluence test (Thm 80) in Go: same final config via three op
orders → identical settled state + registry.
8. Any-order rollback test (Thm 43): independent disposers in every
permutation reach the same state.
9. `check()` parity in Rust/Zig provide + dependents-guard test.
10. Document the ≃ (observational equivalence) reading for port
semantics: what "reverted" means per service key (README/PORTS).
11. Loader confluence: reconcile entries in two different orders →
identical quiescent state (loader-level Thm 80).
12. Committed-view (ω) explicitness audit: confirm generation-counter
retry covers every mid-transition change (write the invariant down).
13. "Lost transition" pin test (see e/4): fiber queued while executing
must eventually re-transition.
14. Failure-isolation test: sibling fibers unaffected by a failed apply
(paper §4.4 Failure) — may exist partially; audit + golden.
15. Guard + failure interaction: failing cleanup during a dependent
drain must not strand the provider mid-bag.

**Dual-mode sync/async track (plan §10):**
16. Zig `std.Io` injection spike: Core takes an `Io`; drain awaits
dependents' tasks at guard points; `Io.Threaded` = blocking.
17. Verify `task.cancel` idempotence semantics against our fiber
dispose contract in a prototype (cancel-as-inverse mapping).
18. Rust async surface spike: async apply + `join_all(dependents).await`
guard, `LocalSet` for Rc core, executor-agnostic (no tokio dep).
19. Go `LongRunning` helper: goroutine + `check()` gate + `StdContext`

- done-channel, one call; makes the async-body idiom first-class.

20. Bench the guard: nested-drain depth cost vs. plain queue on a deep
    chain (10/100 deps) — document the latency profile.
21. Decide the async contract per port and write it into PORTS.md.

**Parity long tail:**
22. Rust/Zig effect-introspection label parity (their session moved it
forward; close the remaining registration kinds).
23. Interception events for Zig `internal/get|set` (Rust got them in
`75b75c3`; Zig has intercept but verify events).
24. Logger golden scenario (Go logger still zero golden coverage).
25. Rust/Zig fuzz + property tests mirroring Go's loader fuzz/timer
property suites.
26. Coverage floors as CI gates (record-only baselines today).
27. Loader rc.10 reassessment (3-stage reload, include journal,
bare-specifier resolution, `hmr.watch`) — port or document.
28. Algorithm-7 realm migration without provider reload (Go loader).
29. `Must*` fate decision (ROADMAP open decision) + guard-aware docs.
30. Panic-free sweep release policy decision (breaking surface).

**Ecosystem/hygiene:**
31. Kernovia: re-run its parity scenarios against the new guard
semantics (oracle consumer; traces unchanged, but confirm).
32. TS upstream: check whether upstream has a test pinning the await
ordering; if not, contribute one upstream (verify-before-filing).
33. Add the guard ordering to PORTS.md architecture section (one
paragraph + diagram reference).
34. Erraudit + clippy (both variants) + zig fmt/docs gates on the
combined tree post-`75b75c3`.
35. Sweep `docs/status/` harvest: fold this report + the 08-25 report
into TODO_LIST per docs-health protocol.
36. Update the parity matrix for everything `75b75c3` landed (Zig
snapshot/restore, validation, status events, intercept).
37. Consider a `CONTRIBUTING` note on the frozen-tree/pathspec commit
protocol for concurrent sessions (worked well; write it down).
38. Golden runner: add `withdraw` op variant that disposes mid-effect
(currently only provider-fiber dispose) for finer pinning.
39. Go doc-comment pass on core.go drain functions (they now carry the
guard invariant; make the doc the single source).
40. CHANGELOG: add the Rust/Zig guard entries when they land (keep the
narrative one divergence-class, one bullet per port).

_(41–50 intentionally unfilled: no padding — the list above is what I
actually derived from this session's work and observations.)_

## g) QUESTIONS ONLY YOU CAN ANSWER (3)

1. **Who lands the Rust/Zig guard?** The other session queued it as
   their follow-up #16, but the tree is clean now and I have the full
   context loaded. Take it back in a fresh session here, or leave it to
   them to avoid another interleave?
2. **Release policy for the breaking-ish surface:** panic-free sweep +
   the guard + (pending) Rust `FnPlugin.inject` signature changes —
   cut `go/v0.2.0` + `rust/v0.3.0` tags now, or hold until the guard
   lands in all three ports so the ports tag together?
3. **Dual-mode async priority:** if we build it, which port first —
   Zig `std.Io` spike (most native-max, unblocks true async bodies),
   Rust executor-agnostic surface (largest ecosystem pull), or Go
   `LongRunning` helper (cheapest, least interesting)? My read: Zig
   first because the calculus maps 1:1 and it validates the design for
   the other two — but that is taste, not a fact I can derive.

---

_Session artifacts: `69e4e2f` (Go guard + plan, pushed), this report,
`docs/planning/2026-09-10_07-55_unload-guard-parity.md`. Tree clean at
08:34; all three suites were green in their last runs this session._
