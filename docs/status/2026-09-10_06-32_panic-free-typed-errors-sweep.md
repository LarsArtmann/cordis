# Status Report: Panic-Free Typed-Errors Sweep

**Date:** 2026-09-10 06:32 CEST
**Session scope:** Elimination of panics in favor of typed errors (or compile-time impossibility) across the Go, Rust and Zig ports of cordis. Origin: user directive "I honestly hate ALL panics. Why can't we have typed errors?"
**Status format override:** Written as Markdown per explicit user instruction; the status-report skill defaults to styled HTML (`.html`). One-off override, not propagated into the skill.

---

## Verification Matrix (end of session)

| Gate | Result |
| --- | --- |
| `go vet ./...` + `go test ./...` (Go 1.27, all 5 packages) | PASS |
| `go test -race` (root + loader, touched packages) | PASS |
| `gofmt -l go/` | CLEAN |
| erraudit (Go, per-module) | 0 violations |
| `cargo test` (default features, 6 suites) | PASS (32 parity tests) |
| `cargo test --features thread-safe` | PASS (6 ok suites) |
| `cargo clippy --all-targets` (both variants) | CLEAN |
| `zig build test` | PASS (27/27) |
| `zig build docs` + `zig fmt --check` | PASS |
| markdownlint gate (`.#test-markdown`) | PASS |
| `nix flake check` (the port gate) | **all checks passed** |
| Golden scenarios (4, byte-identical, all ports) | PASS |

**Working tree:** 28 files modified, **uncommitted by design** (no commit instruction given; the skill's "commit the report" step was skipped for the same reason).

---

## a) FULLY DONE

1. **Go `Waterfall` misuse is now a compile error.** The terminal function moved from "last variadic `any`" to a typed parameter: `Waterfall(name string, terminal func(...any) any, args ...any)`. Both runtime panics deleted. All 3 production callers (`events.go` interceptGet, `reflect.go` EventSet, `fiber.go` EventUpdate) and 4 test files reordered. Dispatch-observer args unchanged.
2. **Go `Isolate` returns `(*Context, error)`** (`go/context.go`). `realmKey` reports uncomparable labels as an error instead of panicking. Loader propagation: `Entry.buildContext()` → `(*Context, error)`, `Entry.start` wraps with entry name. This closes a real bug class: labels come straight from entry config (YAML/JSON), so an uncomparable decoded value (e.g. a map/slice) previously **panicked the process on user input**.
3. **Go `randomID` returns `(string, error)`** (`crypto/rand` failure propagated): `ensureIDLocked` → `(string, error)` → `Tree.Create`, `EntryGroup.Create`, and the group sync loop (errs-aggregation pattern), with mutex-unlock care on the new early returns.
4. **Rust fiber arena stores `Rc` directly** (`Vec<Rc<RefCell<FiberData>>>`, no `Option`): the never-taken `.expect("fiber arena entry")` is structurally gone; `notify_dependents`, `snapshot.rs`, `Registry::delete_id` simplified. Remaining contract is the standard `Vec` index bound (ids are arena-allocated, arena never shrinks).
5. **Rust `FnPlugin.inject` returns `crate::Result<Self>`** with the new `Error::PluginShared { name }` variant (Display arm added). `Context::inject` propagates via `?`. `# Panics` doc section replaced by `# Errors`. Typestate was evaluated and rejected with written rationale: `FnPlugin` is `Clone` over a shared `Rc` for registry identity; owning the base pre-start would break the cheap-clone model.
6. **Zig `Error` set now includes `OutOfMemory`**; 30 allocation sites in `zig/src/cordis.zig` return `error.OutOfMemory`; scope constructors `extend`/`isolate`/`isolateShared`/`withFilter` return `Error!*Context`; test call sites updated to `try`.
7. **Two new regression tests** pin the behavior changes: Go `TestIsolateUncomparableLabelConfigErrors` (loader, config-driven label error) and Rust `inject_after_clone_returns_plugin_shared` (clone → `PluginShared`, unshared original still injectable, pending→active dependency cycle).
8. **Documentation updated:** ROADMAP.md gained the "Panic-free surface (2026-09-10)" divergence entry (full rationale per port, including what deliberately remains); AGENTS.md Zig API bullet and "Semantics deliberately adapted" section updated (old "OOM panics, std style" policy line is gone).
9. **Golden scenarios remain byte-identical** across Go/Rust/Zig after all signature changes.
10. **Full gate sweep green** including `nix flake check` ("all checks passed").

## b) PARTIALLY DONE

1. **"Panic-free" is ~75% by site count.** 13 production sites eliminated; ~37 remain and are documented as contract-bound: Go `Must*` sugar (3 fns) + typed-event dispatch guards (4 sites), Rust typed-event guards (2 sites), Zig dispatch-path allocations (27 sites, distinct message `cordis: out of memory in dispatch`). See (f) items for the path to zero.
2. **Zig keep/convert split has a known inconsistency (found post-hoc):** `isolateShared` is now fallible, but it calls `core.sharedKey`, whose internal `dupe`/`put` allocations still `@panic` — a panic is still reachable from a fallible public API. Same question applies to `rootKey` reachability from registration paths. Not yet fixed; not yet fully audited. This is the top follow-up item.
3. **Zig dispatch-panic elimination not designed.** The "no error channel in any port" claim is true given today's callback contracts (`func(E)` Go, `Fn(&E)` Rust, fn-pointer listeners Zig). Redesign options (pre-allocation before dispatch, error-carrying envelope values, callback contract change) were scoped mentally but not designed or attempted.
4. **Rust typestate builder not prototyped** — rejected on identity-model grounds; a prototype could still disprove the rationale.
5. **Per-site documentation of the 27 Zig dispatch panics** is only via the distinct message + file-level doc comment; no inline `// dispatch: no error channel` markers per site.
6. **Coverage numbers not re-measured.** Go statement coverage was 90.8% (2026-09-08), Rust 86.4%; this session deleted misuse-path tests (Waterfall panic tests) and added new error paths — baselines in AGENTS.md are now stale.

## c) NOT STARTED

1. Committing/pushing the working tree (28 modified files) and observing remote Ports CI.
2. HARVEST of this report into `TODO_LIST.md` / `ROADMAP.md` (docs-health).
3. CHANGELOG.md entry for the breaking API changes (Waterfall, Isolate, FnPlugin.inject, Zig constructors).
4. Version-bump decision for the breaking surface (all three ports).
5. Panic-allowlist CI gate (grep/clippy-based) to prevent regressions of this work.
6. Zig `failure_allocator`-based tests asserting `error.OutOfMemory` surfaces from public APIs.
7. `FiberId` encapsulation in Rust (pub field → pub(crate) + accessor) so OOB indexing is unrepresentable from outside the crate.
8. LSP environment fix: gopls/golangci-lint produced zero usable diagnostics all session (system Go 1.26 vs `go 1.27` in go.mod; known gotcha, unfixed).

## d) TOTALLY FUCKED UP

**Nothing.** All gates are green and `nix flake check` passes; no residual damage. Honest near-misses (all caught and fixed in-session, each costing a round trip):

- The Zig bulk `sed` initially converted **all 57** catch sites, including the 27 no-channel ones; the revert used line-numbered `sed`, which only worked because replacements were line-count-neutral. Fragile technique; a policy split before the bulk edit was the correct order.
- An em dash slipped into a Go comment (violates the no-em-dash rule); caught and fixed.
- One Rust edit used `old_string` identical to `new_string` plus a trailing newline, silently gluing two lines together; caught by reading back the file.
- `expect_err` was used without checking its `T: Debug` bound (`FnPlugin` is not `Debug`); compile error, replaced with `let...else`.
- The new Rust test initially asserted `Active` for a plugin whose injected dependency was never provided — the framework was right, the test was wrong; fixed to assert the pending→active cycle.
- Clippy (pedantic) rejected the first test shape twice (`manual_let_else`, `ignored_unit_patterns`); fixed to `let Err(err) = ... else`.

## e) WHAT WE SHOULD IMPROVE

1. **Design the classification before bulk-editing.** The convert/keep split should have been a written table (site → enclosing fn → verdict) before any sed.
2. **Regression tests first (TDD)** for behavior changes: both new tests were written after the conversion; writing them first would have driven the API shape.
3. **Prefer explicit per-site edits over line-numbered sed** when the edit encodes a *policy* (which sites keep panicking).
4. **Read lints' preconditions** (`expect_err`'s Debug bound) before writing test code.
5. **Run `nix flake check` earlier** — it was the final gate; running it mid-way would have caught integration issues sooner.
6. **Audit reachability when changing a policy boundary.** Making `isolateShared` fallible while its callee `sharedKey` still panics is exactly the class of half-converted boundary this sweep was meant to eliminate — a systematic "who calls the keep-set?" audit should have accompanied the Zig conversion.
7. **Re-measure coverage after deleting tests**, not just after adding them — the stale baselines in AGENTS.md are a docs-drift risk.
8. **Fix the LSP environment once** (per-project gopls wrapper with the flake's Go 1.27) — zero diagnostics for a whole session is a quality-of-life and safety tax.

## f) NEXT: 50 things to get done (brainstorm — most are ROADMAP fuel; top ~10 are commitments)

| # | Task | Impact |
| --- | --- | --- |
| 1 | Audit the 27 Zig dispatch panics for reachability from fallible public APIs; convert reachable ones (known: `isolateShared` → `sharedKey` dupe/put) | HIGH |
| 2 | Commit + push this sweep; watch Ports CI (Build + upstream-parity + flake jobs) | HIGH |
| 3 | Harvest this report into TODO_LIST.md / ROADMAP.md (docs-health HARVEST) | HIGH |
| 4 | CHANGELOG entry + version-bump decision for the breaking API surface (all 3 ports) | HIGH |
| 5 | Panic-allowlist CI gate: fail on new `panic(`/`@panic`/`expect` outside a reviewed allowlist (flake check + ports.yml) | HIGH |
| 6 | Decide fate of Go `Must*` helpers (delete vs keep-as-opt-in) | MED |
| 7 | Design error-channel option for typed event guards (Go + Rust) — envelope type or `Result`-listener variant — to remove the last semantic panics | MED |
| 8 | Zig: `failure_allocator` tests pinning `error.OutOfMemory` from public APIs | MED |
| 9 | Zig: per-site `// dispatch: no error channel` comments on the 27 sites | MED |
| 10 | Re-measure coverage baselines (Go statements, Rust llvm-cov) and update AGENTS.md | MED |
| 11 | Loader: validate `Isolate` label comparability at config-decode time with a field-precise error, before `buildContext` | MED |
| 12 | Loader: add entry/group context to `randomID`/`ensureIDLocked` error wrapping | LOW |
| 13 | Rust: encapsulate `FiberId` field (pub(crate) + accessor) so OOB is unrepresentable externally | MED |
| 14 | Rust: prototype typestate `FnPlugin` builder to test the identity-model rationale | LOW |
| 15 | Rust: doc-example for `Error::PluginShared` in `FnPlugin` docs | LOW |
| 16 | Zig: include function context in dispatch panic messages (e.g. `OOM in emitNamed`) | LOW |
| 17 | Pin Waterfall dispatch-observer args contract with an explicit test (behavior preserved, not pinned) | LOW |
| 18 | Re-run `cargo bench` (`benches/core.rs`) to confirm no perf change from the arena `Option` removal | MED |
| 19 | Re-run `BenchmarkWaterfallEvent` to confirm arg-reordering had no dispatch cost | LOW |
| 20 | Run `nix run .#test` meta-app + buildflow to mirror the full local CI sequence | MED |
| 21 | PR workflow (jj-fork-pr-workflow): branch, commit, PR for this sweep | MED |
| 22 | Next upstream pin bump: add a rebase-checklist line — Go `Waterfall`/`Isolate` signatures now diverge from TS shapes by design | MED |
| 23 | PORTS.md: note the per-port fallibility shapes (Go `(T, error)`, Rust `Result`, Zig `Error!T`) in the shared architecture section | LOW |
| 24 | FEATURES.md: add "panic-free surface" to the inventory with per-port status | LOW |
| 25 | AGENTS.md: add a gotcha for "policy boundary changes require callee reachability audit" | LOW |
| 26 | Review `#[allow]` inventories in rust/src (expect_used allow shrank; crate-level allows may be stale) | LOW |
| 27 | Zig golden/typed test harnesses: replace remaining `catch @panic("oom")`/`catch unreachable` with `try`/`expect` where the test fn allows | LOW |
| 28 | Go: `ctx.Waterfall` godoc review + example for the new signature | LOW |
| 29 | Check docs.rs render of the new `Error::PluginShared` doc links (doc-ci) | LOW |
| 30 | Zig: `deinit`/error-path leak audit for the new early returns (arena semantics make this safe; verify once with a failing allocator) | MED |
| 31 | Group sync loop: confirm aggregated `errs` from `ensureIDLocked` carry entry context for debugging (ties to #12) | LOW |
| 32 | Add "panic-free surface" row to the ROADMAP parity matrix if per-port features are tracked there | LOW |
| 33 | Annotate the 2026-09-09 CI-restoration status report if this session supersedes parts of it (docs-health ANNOTATE) | LOW |
| 34 | Sweep rust/tests for remaining `unwrap`-heavy misuse assertions that could silently depend on panic semantics | LOW |
| 35 | Zig: consider `std.debug.assert` audit — confirm none exist in src (none found this session; pin with the CI gate #5) | LOW |
| 36 | Go: consider a vet-style custom check banning `panic(` in non-test library code (stronger than grep) | LOW |
| 37 | Rust: consider `#[deny(clippy::panic)]` crate-wide with site-level allows only (events.rs is the only allow today) | LOW |
| 38 | Zig: evaluate propagating OOM from `Core.queue`/`logError` via a sticky OOM latch (record once, surface on next fallible call) as an alternative to dispatch panics | LOW |
| 39 | Evaluate error-carrying dispatch envelope (`Value` → `Result<Value, OomEnvelope>`) across all three ports as the long-term zero-panic design | LOW |
| 40 | Update the fork README's port table if it mentions API stability guarantees | LOW |
| 41 | Verify hmr/include TS suites still green post-change (untouched; flake check covers, but one explicit local run for the record) | LOW |
| 42 | AGENTS.md: refresh the "statement coverage ≈90%" claim after #10 | LOW |
| 43 | Add the panic-allowlist file itself (ALLOWLIST comments per entry with rationale + date) | LOW |
| 44 | Consider renaming Zig `Error` to include OOM semantics in docs (already done in code comment; mirror in doc site) | LOW |
| 45 | `zig build docs` output review for the new `Error!*Context` signatures rendering correctly | LOW |
| 46 | Go: expose `IsolateErr`-style helper? No — review whether `(ctx, error)` breaks the fluent `ctx.Isolate(...).X()` chains in docs/examples and fix examples | LOW |
| 47 | Cross-port API drift doc: one table listing every intentional signature divergence (typed terminal, isolate error, inject Result, Zig fallibility) | MED |
| 48 | Session-process improvement: codify "policy split table before bulk edit" into AGENTS.md lessons | LOW |
| 49 | Re-verify erraudit after any further Go error-path changes (it is the gate for the new error plumbing) | LOW |
| 50 | Close the loop: after #2 CI is green, mark this sweep DONE in the next status report | LOW |

## g) QUESTIONS I CANNOT FIGURE OUT MYSELF

1. **API-break release policy:** Waterfall, Isolate, `FnPlugin.inject` and the Zig scope constructors are all breaking public-API changes. Do you want a major version bump + CHANGELOG now (and release cadence per ROADMAP), or should this ride along un-versioned until the next planned release?
2. **End-state for the remaining 27 Zig dispatch panics (and the sharedKey/rootKey leakage):** is "dispatch-boundary abort" an acceptable *permanent* end state, or do you want the full conversion — including changing the shared dispatch callback contract across all three ports — to reach literal zero panics outside `Must*` helpers?
3. **Go `Must*` helpers:** you said you hate ALL panics. Should `MustGet`/`MustGetNamed`/`MustRegister` be deleted outright (breaking wiring code that relies on them), or kept as the documented, explicitly-opt-in escape hatch?

---

*Awaiting instructions.*
