# Status — Execution Pass 3: M15 finished, M16 hmr, M22 + thread-safe deadlock root-caused

**Date:** 2026-09-04 22:48 CEST
**Session scope:** Continue the 27-task Pareto plan from pass 2 (`4f88e44`): finish M15,
execute M16, M22, unblock CI, then M24–M27.

## HEAD at time of writing

`3118f7d` — pushed. This session: `82d7206` (M15), `4c3f0fe` (CI dep restore),
`f2cd379` (M16 hmr), `d9cc834` (M22 Rust + deadlock fix), `3118f7d` (CI timeouts).

## a) FULLY DONE

### M15 — loader watch/reload (`82d7206`)
- Repaired `watch_test.go`: the broken-subtree test now asserts the implemented veto
  semantics (per-entry errors in `Tree.Errors()`, reload returns nil) instead of the
  stale wholesale-rejection assertion; removed a dead loop variable.
- Root-caused `TestServeReloadsOnChange`: `PollWatcher` captured its baseline at the
  **first poll**, so a config written within the first interval silently became the new
  baseline and no change was ever detected. Fix: baseline is primed in
  `NewPollWatcher` (constructor), so the observation window starts at construction.
  A missing file still defers the baseline to the first successful stat.
- Verified: `go vet`, `go test -race -count=3` on all 4 Go packages, golangci-lint
  0 issues. 12 loader tests + watch suite green.

### CI dependency restore (`4c3f0fe`)
- The `Build` failure after `86936c6` was `yaml.Type is not a constructor`:
  `5dfa504` had bumped `js-yaml ^4.1.0 → ^5.4.1` in `packages/include` (v5 removed the
  API). Swept **all** remaining manifests vs upstream in one pass: restored
  `packages/{create,hmr,include,logger-console}/package.json`. All `**/package.json`
  now byte-identical to upstream. Build progressed from 17–26s compile failure to
  2m24s deep into the unit tests.

### M16 — Go hmr package (`f2cd379`) — "the killer upstream feature"
- New `go/hmr`: `Manager` (module identity via per-module generation counters),
  `Declare` (dependency edges), `Swap`/`SwapType` (swap a resolver registration and
  relink live entries), `EventReload`, full rollback on failed swaps.
- Loader primitives it builds on: `Resolver.Replace` (returns previous registration
  for rollback), `Tree.Refresh`/`Entry.refresh` (dispose + re-resolve with config and
  entry identity preserved).
- Semantics: every live entry whose plugin reaches the swapped module through declared
  imports accepts the change (dispose+relink); everything else declines by omission.
  All-or-nothing rollback: if any entry fails under the new implementation
  (`Tree.Refresh` error **or** fiber lands in `StateFailed` — apply errors do not cross
  the framework boundary), the previous registration is restored and every touched
  entry is relinked back.
- 9 tests mirror the upstream hmr scenarios: basic reload, config preservation,
  `hmr/reload` event, dependents-only, transitive dependents, rollback + recovery,
  unaffected modules stay live, 50 rapid successive swaps, new-module registration.
- Verified: vet, `-race -count=3`, golangci-lint 0 issues.

### M22 — Rust registry snapshot/restore + status events (`d9cc834`)
- `internal/status` emission: `EVENT_STATUS` + `StatusChange{uid, name, old, new}`
  emitted from a `settle_state` choke point wired into `transition`,
  `finish_transition` and `load`. The plugin name is **cached on the fiber** at
  creation, so a dying fiber keeps its identity after its runtime is removed.
- `snapshot.rs`: `RegistrySnapshot`/`RuntimeSnapshot`/`FiberSnapshot`,
  `Context::snapshot()` and `Context::restore()`. Restore disposes the delta,
  restarts removed runtimes from `Core.stash` (bodies kept by `delete_id` and by
  restore's own delta pass) with their last config **on the caller's context**, and
  requeues surviving-but-pending fibers.
- Tests: exact status-emission order for start/restart/dispose;
  snapshot→delete→restore roundtrip (service re-provided, config preserved) and
  older-snapshot restore disposing later runtimes. Doctests for typed services
  (`provide`/`get`), named services (`get_named`) and typed events (`on`/`emit`).
- Verified: `cargo test` **and** `cargo test --features thread-safe` fully green;
  clippy clean in new code (`snapshot.rs` 0 findings; no new fiber.rs findings).

### THE BIG ONE — pre-existing thread-safe deadlock root-caused and fixed (`d9cc834`)
- Symptom: three CI `Ports` runs hung for 1h27m+ today (cancelled); locally the
  `--features thread-safe` suite froze on `parity::plugin_lifecycle` for an hour.
- Verified pre-existing: stashing my M22 changes still hung on committed HEAD.
- Root cause: **thread-safe mode swaps `RefCell` for `std::sync::Mutex`
  (non-reentrant), and `Fiber::name()` locked the core and then locked it again
  through `self.data()`** → instant self-deadlock. My initial M22 draft had a second
  instance of the same class (core borrow held across `settle_state`); both fixed by
  releasing every core lock before re-locking.
- The thread-safe suite is green for the first time since the Mutex port.

### CI hardening (`3118f7d`)
- `timeout-minutes: 15` on all Ports jobs, `20` on all Build jobs — a stuck job now
  fails fast instead of hanging the workflow for hours.

## b) PARTIALLY DONE

- **CI verification of the deadlock fix**: the push of `d9cc834`/`3118f7d` triggered a
  fresh Ports run. It should **complete for the first time today** (the hang was the
  thread-safe job). Not yet confirmed at time of writing — check first thing next
  session and record the URL.
- **Status report** (this document) — being committed now.

## c) NOT STARTED

- M24 accessor/mixin, M25 callable services + tracker, M26 API polish
  (`Fiber.Err()`, `Await(ctx)`, `errors.AsType` sweep, typed-inject sugar, PORTS.md
  table, README badges, AGENTS.md gotchas), M27 quality batch (benchmarks, LIFO
  property test, `go/v0.1.0` + Rust `v0.2.0` releases, parity-matrix generator, zig
  docs, cadence note), Rust lint pay-down (~210 clippy warnings, all pre-existing),
  final 3-language verification + `nix flake check -L` + docs harvest.
- `Fiber.Err()` (M26) will let hmr rollback errors carry the actual apply error
  instead of "entry failed under the new implementation".

## d) TOTALLY FUCKED UP (honest ledger)

1. **First `hmr_test.go` draft was garbage** — placeholder identifiers
   (`errors_New`, `mgrResolver`, `mustReg`…) and an incoherent dependents test.
   Rewrote the whole file properly. Should draft completely before writing files.
2. **Missed a documented core fact**: apply errors don't cross the framework boundary
   (`StateFailed`) — AGENTS.md literally documents this, and my first rollback
   implementation only trusted `Refresh`'s error return. The test caught it; I should
   have re-read the lessons before designing.
3. **Rollback set bug**: the failing entry wasn't included in the rollback list
   (appended only after success), so a failed swap left the entry dead. Test caught it.
4. **Sloppy first `snapshot.rs` draft**: referenced phantom APIs (`mark_dirty`),
   junk imports, leftover cruft. Rewrote from scratch.
5. **Stash design churn**: v1 stashed the dead fiber's context (restart always failed
   `assert_active` → restore silently no-op'd); v2 stashed on *every* last-fiber
   disposal (unbounded memory leak); v3 (shipped) = stash on explicit removals only,
   `(base, config)` tuple, restart on caller's context. Should have thought the
   lifecycle through before coding.
6. **Blind CI fix push**: pushed `4c3f0fe` believing it would green Build; it fixed
   install but exposed the next failure. Checking upstream's CI state *first* would
   have saved a cycle.
7. **Thread-safe suite hung locally for ~1h** before I killed it and made the
   RefCell→Mutex connection. The SIGTERM trail (frozen test output) was visible much
   earlier.
8. **Let-else + shadowing compile errors** in the `load()` rework — two avoidable
   round trips from writing Rust in one pass instead of compiling incrementally.

## e) WHAT WE SHOULD IMPROVE

- **Read AGENTS.md "Go core facts" before designing against framework boundaries** —
  two of three bugs were already documented lessons.
- **Check upstream CI state before debugging fork CI** (`gh run list --repo
  cordiverse/cordis`): would have identified the hmr suite as upstream-broken in
  minutes instead of an hour. Their `3-stage-hmr` branch ("separate stale selection
  from cache invalidation", green) confirms upstream main's hmr tests are broken
  upstream too.
- **Bounded runs everywhere**: `timeout N cargo test` for anything that can deadlock;
  the same discipline now exists in CI via `timeout-minutes`.
- **Trust compilers over LSP diagnostics** — the Go LSP served stale typecheck errors
  on green code all session.
- **Draft-then-write**: two files were written half-formed and immediately rewritten.
- **Thread-safe review checklist**: every new borrow scope gets audited against
  "no nested core locks" — Mutex mode turns RefCell discipline violations into hangs,
  not panics. Consider a debug mode where `RefCell` is a re-entrancy-detecting wrapper.
- **Branch-based workflow** (still unadoptioned from pass 2's §g): the auto-commit
  daemon reverted `watch_test.go` twice in pass 2; this session survived by
  grep-verifying before every edit/commit, but the hazard remains structural.

## f) NEXT 50 (ordered, roughly Pareto)

1. Confirm the fresh Ports run completed green; record URL in TODO_LIST.md.
2. Watch upstream `3-stage-hmr` merge; when merged, sync TS tree to it (Build greens).
3. Decide yarn.lock policy (upstream commits none; CI uses `--no-immutable` →
   floating deps). Recommended: commit a generated lockfile to the fork for
   reproducible CI, documented as divergence.
4. M24: accessor/mixin system (Go).
5. M25: callable services + tracker (Go).
6. M26: `Fiber.Err()` (Go) — then use it in hmr rollback error detail.
7. M26: `Await(ctx)` cancellation-aware await (Go).
8. M26: `errors.AsType` sweep across go/.
9. M26: typed-inject sugar (`Inject` with typed config).
10. M26: PORTS.md parity table refresh (loader, hmr, timer, group, snapshot/status).
11. M26: README badges (CI, GoDoc, crates.io, coverage).
12. M26: AGENTS.md gotchas — add Mutex non-reentrancy + `--no-immutable` + upstream
    hmr-flake notes.
13. M27: Go benchmarks (start/dispose, event emit, drain queue, hmr swap).
14. M27: LIFO disposal property test (Go, randomized).
15. M27: tag `go/v0.1.0` (annotated; module is `github.com/LarsArtmann/cordis/go`).
16. M27: release Rust `v0.2.0` (snapshot/status added).
17. M27: parity-matrix generator script (PORTS.md from test inventory).
18. M27: zig package docs (doc comments on public decls).
19. M27: release-cadence note in ROADMAP.md.
20. Rust lint pay-down (~210 findings; `cargo clippy --fix` applies ~132).
21. ROADMAP.md: hmr native divergence entry (file-watch → factory swap; watcher layer
    owned by embedder).
22. ROADMAP.md: loader entries (Resolver registry, veto semantics) if not yet there.
23. Final 3-language verification: go `-race -count=3`, cargo both modes, zig
    `nix run nixpkgs#zig -- build test --summary all`.
24. `nix flake check -L`.
25. actionlint on both workflows (done for current state; re-run after edits).
26. TODO_LIST.md harvest: mark M14–M16, M22 done; move §g questions forward.
27. Cancel/ignore stale runs; confirm no other hung runs remain.
28. Go: hmr — consider `Swap` option to preserve fiber identity vs new fiber (currently
    new fiber, same entry — document explicitly in package doc).
29. Go: hmr — add `hmr/reload` report to include per-entry failure detail once
    `Fiber.Err()` exists.
30. Go: loader — `Resolver.ReplaceType[C]` sugar (parity with `RegisterType`).
31. Go: hmr — concurrency test: parallel `Swap` + `Tree.Create/Remove` storm.
32. Rust: `internal/plugin` + `internal/update` events (M13 parity — Go has them,
    Rust does not).
33. Rust: status event emission for the root fiber covered? (root writes state in
    `new_root` without emission — check upstream parity).
34. Rust: snapshot — prune stale stash entries (currently replaced per id; growth
    bounded by distinct deleted plugins — acceptable, document).
35. Rust: `Registry::delete` doc now mentions stash/restore (done in delete_id doc —
    verify rendered docs).
36. Rust: doctests for `EVENT_STATUS`/`StatusChange` usage.
37. Zig: confirm 0.16 build green after all changes (final verify).
38. Zig: consider porting hmr equivalents only if roadmap demands (currently not
    planned — note in matrix as N/A with rationale).
39. CI: consider `--locked`/`--frozen` for cargo (Cargo.lock IS committed — verify).
40. CI: build.yml — Node version pin decision (see §g Q2).
41. Docs: README (root) — add loader/hmr/snapshot to feature list.
42. Docs: packages/core/README.md divergence note (fork rewrote it — ensure it
    explains why it differs from upstream).
43. Verify `.gitignore` covers `node_modules/`, `packages/*/lib`, `target/`
    (local TS build created `lib/` dirs — check git status cleanliness).
44. AGENTS.md: record the `yarn.lock`-not-tracked upstream fact + local TS repro
    recipe (`nix shell nixpkgs#nodejs_24 nixpkgs#corepack`).
45. AGENTS.md: record thread-safe audit rule (no nested core locks).
46. Coverage: `go test -cover` for go/loader and go/hmr; record in TODO_LIST.
47. Consider `golangci-lint` config pinning (CI uses repo config — verify parity with
    local).
48. Sweep: any leftover debug prints/todos in go/hmr, rust/src/snapshot.rs (none
    found by lint; final grep in final-verify step).
49. Plan doc: mark M15/M16/M22 + CI tasks complete in the Pareto plan checkboxes.
50. Next session: M24 → M25 → M26 → M27 in dependency order, then harvest.

## g) QUESTIONS I CANNOT ANSWER MYSELF

1. **Upstream hmr suite policy**: Build's remaining failure is 11 upstream hmr tests
   timing out — reproduced locally on clean Node 24.19 + fresh deps; upstream has a
   green fix branch (`3-stage-hmr`) unmerged. Options: (a) leave Build red until
   upstream merges, gating on Ports only; (b) cherry-pick upstream's fix branch into
   our TS tree now; (c) pin Node versions in build.yml hoping it dodges the flake
   (my local repro says it won't). My recommendation is (a). Which do you want?
2. **yarn.lock policy**: upstream commits no lockfile and installs with
   `--no-immutable`, so CI resolves floating ranges on every run. I can commit a
   generated lockfile (pins CI, small divergence from upstream) or track upstream
   exactly (unpinned). Preference?
3. **Rust restore scope**: `restore()` currently restarts removed runtimes on the
   *caller's* context (documented). The alternative is stashing full context chains
   (isolate/intercept) per runtime — more fidelity, more memory, more machinery.
   Is caller-context restore acceptable as the documented native semantic?
