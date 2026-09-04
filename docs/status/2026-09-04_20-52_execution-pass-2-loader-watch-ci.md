# Execution Pass 2 — CI Toolchain Fix, Go Loader (M14), Watch/Reload (M15 in flight)

**Date:** 2026-09-04 20:52 CEST
**Session:** continuation of the 27-task Pareto plan
(`docs/planning/2026-09-04_15-42_ship-parity-ecosystem-pareto-plan.md`).
**Baseline at session start:** `main` @ `271b542`, pushed; M01–M13,
M17–M21, M23 complete per handoff.
**Head now:** `c694a7f` (M14 loader) on top of `86936c6` (CI toolchain
revert). Both pushed.

---

## Direct answers first

**What did I forget?**
1. To verify the CI runs after pushing `86936c6`. I pushed the toolchain
   revert and moved on to M14; the `Build` run is still **failure** (24s)
   and the `Ports` run has been **in_progress for >1h** (normal: 3–6min).
   Both need triage — unverified claims are exactly what this plan exists
   to eliminate.
2. The decode step on the in-place update path. The initial start decoded
   raw config through the registration's `Decode`, but `reconcile` fed the
   raw `map[string]any` straight into `Fiber.Update` — the classic
   two-paths-one-invariant bug. Found by tests, fixed by storing the
   `Registration` on the entry and decoding in `reconcile`
   (`go/loader/entry.go`).

**What could I have done better?**
1. Read `resolveHooks` before writing per-entry event hooks. Go hooks are
   scoped by dispatch-context *filter*, not by owner chain, so my
   per-entry `EventUpdate` hooks were effectively global. That forced a
   mid-task redesign: the `internal/update` waterfall now carries the
   updating fiber and one global tree listener routes saves. Reading the
   event-resolution source first would have produced the right design on
   the first pass.
2. Designed the group/entry interaction against the transition machine
   *before* coding. Three real bugs came out of not doing that fully:
   `host.ctx` still nil during group apply (the recovered panic then
   masked as `ErrInactiveEffect`), a double-append in `EntryGroup.Update`,
   and self-disposed fibers being tracked after their dispose event.
3. Isolated the failing M15 tests immediately instead of fixing forward —
   `TestCloseStopsWatcher` was a pure test-authoring bug (assertions
   placed after `Close`) and cost a debug cycle.

**What could I still improve?**
1. The auto-commit daemon raced my edits twice today (it reverted
   `watch_test.go` back to a stale version while I was fixing it), which
   produced phantom test failures and wasted cycles. Working in a branch,
   or pausing the daemon during active coding, would remove this hazard.
2. Write watch tests against a stub `Watcher` (deterministic) and keep the
   `PollWatcher` tests separate — file mtime polling in tests is the
   slowest, flakiest part of the suite.
3. Commit in smaller units. M14 landed as one commit; the M15 work is
   sitting uncommitted in the tree because the task isn't green yet,
   which is what exposed it to the daemon race.

---

## a) FULLY DONE

| Item | Evidence |
| --- | --- |
| TS toolchain CI regression root-caused and reverted (`86936c6`) | `typescript ^7.0.2` (no `lib/_tsc.js`) broke yarn's builtin patch; `package.json` + loader peer dep now byte-identical to `upstream/main` (verified via `git diff upstream/main`) |
| **M14 Go loader package** (`go/loader`, ~1100 LOC + ~700 test LOC) | `resolver.go` (Registration + generic `RegisterType[C]` + `DecodeInto`), `entry.go` (EntryOptions/Entry lifecycle, reconcile, isolate/intercept scopes), `group.go` (EntryGroup diff, group plugin with **restart veto** — upstream semantics on Go's real-restart `Fiber.Update`), `tree.go` (store, diff APIs, Validate dry-run, self-dispose detection, Locate), `loader.go` (service, Start diff), `config.go` (JSON schema `{plugins:[...]}`, Find/Load/Save/Open) |
| Core additions, tested | `StartAny`, `InjectSpec` (`go/plugin.go` + `TestStartAnyAndInjectSpec`); `internal/update` waterfall now carries `(*Fiber, config, noSave, next)` — no external consumers existed |
| M14 test coverage | 12 tests: file-driven start, per-entry validation, inject wiring (pending→provide→active + Intercepted), nested group diff (in-place child update, group veto, removal cascade), isolate/intercept options, self-dispose marks disabled **and persists to file**, in-place `SetConfig` (fiber identity preserved, siblings untouched), root diff, Update/Replace merge semantics, `DecodeInto` passthrough, config round-trip |
| Verification at M14 commit | `go vet ./...` clean; `go test -race -count=3 ./...` all four packages green; `golangci-lint run` 0 issues; committed as `c694a7f` |
| Repo state | `main` pushed through `c694a7f`; working tree contains only the in-flight M15 files |

## b) PARTIALLY DONE

1. **M15 watch/reload** — code complete (`go/loader/watch.go`:
   `Watcher` interface, stdlib-only `PollWatcher`, `Loader.Reload` with
   parse-failure keep-old-config semantics, `Loader.Serve` goroutine,
   `EventConfigUpdate` emission) and 7 tests written. **Two red:**
   - `TestReloadRollsBackOnlyBrokenSubtree`: the assertion is stale — it
     expects `Reload` to return an error, but the implemented (and now
     documented) semantics are: the group veto diffs in place, the broken
     child records its error per entry in `Tree.Errors()`, and the group
     fiber stays live. The updated test has been drafted twice; see (d)
     about the daemon reverting it.
   - `TestServeReloadsOnChange`: the watcher→`Reload` chain runs but the
     in-place restart does not apply (`[start:v1]`, no stop/start v2).
     One more debug round needed — the direct `SetConfig` restart path is
     proven green by `TestInPlaceConfigUpdate`, so the suspect is the
     watcher→Reload timing or the serve goroutine. NOT yet diagnosed.
   - Fixed inside this task already: `reconcile` now decodes through the
     registration (`entry.reg`) before `Fiber.Update`; `Await` skips
     parked (StatePending) fibers to remove a deadlock class.
2. **CI verification of `86936c6`** — pushed but unverified: `Build`
   failed again (24s, untriaged — could be a *different* failure than the
   yarn one, e.g. the eslint/peer warnings becoming errors), `Ports`
   hung >1h (needs cancel/re-run).
3. **Docs for the new packages** — loader has package docs and the
   port-architecture divergence notes inline; `TODO_LIST.md` /
   `PORTS.md` / README sections for loader+watch not yet written (was
   planned for the final docs harvest).

## c) NOT STARTED

- **M16** Go `hmr` module (blocked behind M15 by plan dependency).
- **M22 remainder** Rust: registry snapshot restore, `internal/status`
  emission, typed doctests.
- **M24** Go accessor/mixin system.
- **M25** Go callable services + tracker attribution.
- **M26** API polish: `Fiber.Err()`, `Await(ctx)`, `errors.AsType` sweep,
  typed-inject sugar, PORTS.md API comparison table, README badges,
  AGENTS.md Zig gotchas.
- **M27** quality batch: benchmarks, LIFO property test, releases/tags,
  parity-matrix generator, `zig build -femit-docs`, cadence note.
- **Rust lint pay-down** (~194 pedantic/nursery findings toward deny).
- **Recording green CI runs** in `TODO_LIST.md` — blocked on (b)2.

## d) TOTALLY FUCKED UP

1. **The auto-commit daemon reverted working-tree edits twice** during M15
   (`watch_test.go` jumped back to stale content mid-fix). This produced
   contradictory test runs, cost at least three debug cycles, and is the
   single biggest process failure of the session. It also makes "current
   file state" untrustworthy — every future session must re-verify files
   before assuming their content.
2. **CI follow-through was skipped** after the toolchain-revert push — the
   exact anti-pattern this plan's tier P1 exists to prevent (claims not
   remotely proven). Red/hung runs sat untriaged for over an hour while I
   built new features on top.
3. **Three design-level bugs shipped into my own tests** (host.ctx nil
   during apply, double-append in group diff, missing decode on the
   restart path). All were caught by the tests I wrote — the safety net
   worked — but each was preventable by 10 minutes of design review
   against `fiber.go`'s transition machine before writing code.
4. **Left junk in committed work**: a `j++` dead statement and a nonsense
   `errors.As(nil, ...)` guard in the new tests. Sloppy; caught by lint
   only partially (ineffassign), the rest by my own re-read.

## e) WHAT WE SHOULD IMPROVE

1. **Serialize agent + daemon.** Either pause the auto-commit daemon
   during active sessions, or do feature work on a feature branch and let
   the daemon own `main`. Today it corrupted in-progress work.
2. **Make CI state a hard gate in the loop**: after every push, block on
   `gh run watch` (or poll ≤5min) before starting new work; cancel and
   re-run hung workflows (a 1h+ "in_progress" for a 3–6min workflow is a
   stuck runner or a missing timeout — add `timeout-minutes` to every
   job).
3. **Design reviews against the state machine before coding**, especially
   for anything touching fiber lifecycle: write down the expected
   transition sequence first.
4. **Single-invariant-two-paths checks**: any place a config flows into a
   plugin through more than one path (initial start, update, reload) needs
   one shared normalize/decode function, not parallel code.
5. **Test hygiene**: no test may assert on events after the action that
   produces them without an explicit ordering comment; no `j++`-style
   leftovers; prefer stub collaborators over real timers/files where the
   SUT allows it.
6. **Add `timeout-minutes`** to `build.yml` and `ports.yml` jobs so hung
   runs fail loudly instead of blocking the queue.

## f) NEXT — prioritized backlog (36 items)

**Immediate — finish M15 (est. 1–2h)**
1. Debug `TestServeReloadsOnChange`: verify the watcher fires and trace
   `Reload`→`Start`→`reconcile`→`Fiber.Update` for the served path.
2. Re-apply (and keep) the corrected rollback-test assertions matching
   the documented veto semantics.
3. Run the full Go suite `-race -count=3`, golangci-lint, commit M15 as
   its own commit.
4. Confirm no daemon reverts landed (`git status` + content spot-check
   before every commit).

**CI trust (est. 30min)**
5. Triage the new `Build` failure on `86936c6` (24s — read the log; if
   peer-dependency warnings became errors, pin accordingly).
6. Cancel + re-run the hung `Ports` run; add `timeout-minutes` to all
   jobs in both workflows.
7. Record the first green `ports.yml` URL for this batch in
   `TODO_LIST.md` (M01/M02 follow-through).

**M16 — Go hmr (est. 2–3h)**
8. Module identity model (key, dispose+relink contract) on top of the
   loader's entry disposal.
9. `Accept`/`Decline` API on the hmr context.
10. Dispose+relink pipeline through `internal/plugin`/`internal/update`.
11. Tests: module swap preserves siblings; declined update keeps old
    module live.
12. Port TS hmr fixtures; parity-style assertions; docs.

**M22 remainder — Rust (est. 1.5h)**
13. Registry snapshot struct (runtimes + fibers view).
14. `restore` semantics: dispose delta, re-start missing.
15. `internal/status`-equivalent emission on transitions.
16. Tests: snapshot restore == pre-state; status emission order.
17. Doctests: typed services, typed events, `get_named`, start identity.

**M24 — accessor/mixin (est. 1.5h)**
18. Accessor prop type + accessor store beside the service store.
19. Mixin registration API on Context.
20. Get/set routing through accessors when declared.
21. Tests: accessor shadowing + realm interaction; introspection + docs.

**M25 — callable services + tracker (est. 1.5h)**
22. Callable service type (services that are funcs).
23. Tracker: current calling fiber during service ops.
24. Attribution wiring in registration paths.
25. Tests: effects created via service attribute to the caller; tree
    shape under attribution.

**M26 — API polish + docs (est. 1.5h)**
26. `Fiber.Err()` accessor (apply error).
27. `Await` variant honoring a stdlib `context.Context`/timeout.
28. `errors.As` → `errors.AsType` sweep in `go/errors.go`.
29. Typed-inject sugar `Plugin.InjectTypes[T1, T2]()`.
30. PORTS.md cross-port API table; README badges + port pitch; AGENTS.md
    Zig 0.16 gotchas; loader/hmr sections; Batch semantics notes.

**M27 — quality batch (est. 1.5h)**
31. `nix flake check -L` CI job; benchmark skeleton (drain throughput).
32. Rust+Zig benchmark stubs + README results table skeleton.
33. LIFO rollback property test (randomized registration sequences).
34. Releases: tag `go/v0.1.0`, Rust `v0.2.0`, Zig version note.
35. `zig build -femit-docs` pass; parity-matrix generator; cadence note.
36. Final: full 3-language verification + actionlint + docs harvest +
    status report.

## g) QUESTIONS (cannot answer myself)

1. **Daemon policy:** the auto-commit daemon reverted in-progress edits
   twice today. Should I (a) pause/disable it during active coding
   sessions, (b) work on a feature branch and merge when green, or (c)
   leave it as is and just re-verify files before each commit?
2. **CI ops:** the `Ports` run for `86936c6` has been "in_progress" for
   over an hour (normal 3–6min) and `Build` failed again after my fix.
   May I cancel the hung run and re-run both workflows via `gh`, and do
   you want `timeout-minutes` hard limits (e.g. 15m) added to every job?
3. **Loader/hmr layout (carried over — still unanswered):** I proceeded
   with Go *subpackages* (`go/loader`, `go/hmr`) inside the existing
   single Go module, consistent with `go/timer`/`go/group`. Confirm, or
   tell me to split standalone modules before M16 builds on top.

---

*Point-in-time report. Test facts measured at 20:40–20:52 CEST against
`c694a7f` + working tree (watch.go/watch_test.go uncommitted,
entry.go/config.go/loader.go modified).*
