# Status Report — samber/do + samber/ro verdict, Go 1.27 verification

**Date:** 2026-09-08 07:46 CEST
**Session type:** Advisory / research. **Zero code changes to cordis.** Working tree clean at report time (`git status --short` empty).
**Scope:** This session only — three exchanges: (1) PRO/CONTRA verdict on adopting `samber/*` (vs cordis) for go-taskqueue, with Go 1.27 considerations; (2) research-quality challenge → full primary-source re-verification; (3) this report.

> Format note: written as `.md` per explicit user instruction (repo convention in `docs/status/`); the status-report skill's HTML default was intentionally overridden.

---

## a) FULLY DONE

1. **Verdict delivered (and survived verification):**
   - `samber/do` v2 → **YES** for go-taskqueue wiring/shutdown; stable, 342 importers, MIT, zero deps.
   - `samber/ro` → **NO**; pre-1.0 (README-acknowledged breaking changes), 47 importers, ecosystem misaligned with our stack policy (plugins wrap testify/logrus/zerolog — all banned in `how-to-golang`); a ~50-line broadcaster covers the hub use case.
   - `samber/*` **inside cordis → NEVER**; cordis is the composition root — a second container is a split brain.
2. **Go 1.27 release notes verified from the primary source** (raw fetch of go.dev/doc/go1.27, not a summary):
   - **Generic methods** confirmed verbatim ("a method declaration may declare its own type parameters"), including the exact caveat: interface methods may not declare type parameters, and interface methods cannot be implemented by generic methods.
   - **Compile-verified locally** under `nix shell nixpkgs#go_1_27`: `func (c *C) Get[T any]() T` compiles and runs (repro in `/tmp/gm`).
   - `asynctimerchan` GODEBUG removed permanently (synchronous timer channels themselves landed in Go 1.23).
   - `goroutineleak` pprof profile GA (`runtime/pprof`, `/debug/pprof/goroutineleak`).
   - `testing/synctest.Sleep`; `encoding/json/v2` + `jsontext` GA; new `uuid` package; `maphash.Hasher`/`ComparableHasher`; `httptest.NewTestServer` (synctest-compatible).
   - **No** stdlib `Future`/structured-concurrency primitive → adopting `do` now will not be obsoleted next cycle.
3. **samber/do v2 verified** (proxy.golang.org + pkg.go.dev + README): module `github.com/samber/do/v2`, v2.0.0 → **v2.1.0** (published 2026-07-20), MIT, zero deps, "v2 and follows SemVer strictly. No breaking changes before v3.0.0". API index confirmed: `Provide*`/`Invoke*`/`InvokeAs`/`InvokeStruct`, `Shutdown*` → `*ShutdownReport`, `Shutdowner[WithContext|WithError]`, `Healthchecker*`, `RootScope`/`Scope` tree, `Override*`, `Package`, `Eager`/`Lazy`/`Transient`, lifecycle hooks (`Add*Hook`), `ShutdownOnSignals`, `Explain*` graph.
4. **Live repro against do/v2 v2.1.0** (`/tmp/docheck`): a failed `Invoke` does **not** auto-rollback already-started services; explicit `Shutdown()` does clean them up. The main CONTRA is empirically confirmed, not documentation folklore.
5. **cordis Go API surface verified locally:** typed API is package-level free functions in `go/typed.go:26-152` (`ServiceName`, `EventName`, `Provide`, `Get`, `MustGet`, `TryGet`, `On`, `Once`, `Emit`) → the Go 1.27 generic-method migration (`ctx.Get[T]()`, `ctx.On[E]()`) is feasible and meaningful.
6. **Corrections issued against my own turn-1 answer** (asynctimerchan framing fixed; `do` richer than first credited: dependency-aware parallel shutdown, hooks, `ShutdownOnSignals`, `Explain`).
7. Required skills loaded and followed each turn (`samber-do-best-practices`, `how-to-golang`, `verify-external-claims`, `status-report`, `brutal-self-review`).

## b) PARTIALLY DONE

1. **"What Go 1.27 enables for cordis"** — identified and compile-proved the headline feature (generic methods), but no migration design, PR, or ROADMAP entry exists yet.
2. **`goroutineleak` CI suggestion** — raised, wired nowhere.
3. **do v2 deeper semantics** — assessed from README bullets + pkg.go.dev index + one rollback repro; parallel dependency-aware shutdown ordering, hook ordering, and `ShutdownReport` contents are not runtime-tested.
4. **Verified statistics are chat-only** — nothing persisted to AGENTS.md, skills, or ROADMAP yet.

## c) NOT STARTED

1. go-taskqueue-side enactment (composition root, broadcaster hub) — different repo, not accessible from this session.
2. cordis generic-method API migration.
3. HARVEST of this report's section (f) into TODO_LIST/ROADMAP (docs-health).
4. Runtime evaluation of `ro` — intentionally skipped (recommended against); no repro exists.
5. Wiring `goroutineleak` assertions into cordis Go tests.
6. Persisting the samber/* decision + verified facts into any durable doc.

## d) TOTALLY FUCKED UP!

1. **Turn-1 research quality (the big one).** I delivered a confident verdict built on (i) the user's pasted matrix from a _prior_ session and (ii) ONE AI-summarized fetch (`agentic_fetch`) of the Go 1.27 release notes. I repeated prior-session statistics (~47 importers, v0.4.x, ~200 operators) and a wrong framing as if they were my own verified knowledge. `verify-external-claims` was already triggerable in turn 1 — its documented failure mode is "loading the skill is not the same as applying it"; here I did not even load it. Only the user's challenge triggered the proper pass. The verdict direction happened to survive verification — that is luck, not process.
2. **asynctimerchan mis-framing:** I implied Go 1.27 changed timer-channel behavior; the synchronous behavior landed in 1.23 and 1.27 merely removes the last GODEBUG escape hatch. Corrected in turn 2.
3. **Unlabeled inference:** turn 1 asserted `ro` time operators "ride always-synchronous timer channels" without reading ro's source — plausible, untested; correctly dropped from the verified table but never explicitly retracted.
4. **Stale-context noise:** early LSP diagnostics suggested go.mod merge conflicts that no longer existed at report time (rebase completed earlier today, see `2026-09-08_04-32_rebase-completion...`). A session advising "migrate to 1.27 APIs" should have checked current repo state before speculating about conflict risk.

Nothing in the repository was damaged: no edits, no commits, no reverts.

## e) WHAT WE SHOULD IMPROVE!

1. **Chat-time verification gate:** any sentence about external tool behavior, versions, or statistics gets a primary-source check or an explicit "plausible, untested" label — _before_ sending, not after a challenge.
2. **Raw fetch beats AI-summarized fetch** for release notes/API indexes; `agentic_fetch` only for extraction questions, never as sole evidence.
3. **"Verified elsewhere" ≠ verified.** A paste claiming "verified this session (another session)" is a lead, not a source.
4. **Runnable checks are cheap and decisive here** (`nix shell nixpkgs#go_1_27` + a `/tmp` module): default to compile/run repros for language and library behavior claims.
5. **Persist verified facts** (do v2.1.0, 342 importers, rollback semantics, 1.27 feature list) into the `samber-do-best-practices` verification block / AGENTS.md instead of leaving them chat-only.
6. **Check live repo state before advisory claims** about repo risk (the UU-conflict worry was already stale).

## f) Next things (brainstorm — ROADMAP fuel, not commitments; priority-sorted)

**cordis × Go 1.27 (P1):**

1. ~~Migrate typed free functions to generic methods on `*Context`: `ctx.Get[T]()`, `ctx.TryGet[T]()`, `ctx.MustGet[T]()`, `ctx.Provide[T]()`, `ctx.On[E]()`, `ctx.Once[E]()`, `ctx.Emit[E]()`; keep free functions as deprecated aliases during transition.~~ **Won't implement — ROADMAP records the deliberate non-adoption with rationale (the interface limitation this research confirmed); deprecation timeline parked in ROADMAP Open decisions.**
2. ~~Verify golden traces unchanged after the API migration (Go/Rust/Zig must stay byte-identical).~~ done (moot — no migration happened; golden traces green on 2026-09-08)
3. ~~Document in AGENTS.md which 1.27 feature(s) actually pin the `go 1.27` directive.~~ done (AGENTS.md and ROADMAP record synctest as the pinning 1.27 feature)
4. Add a `goroutineleak` pprof assertion to drain/dispose/rollback tests.
5. ~~Use `testing/synctest.Sleep` in `go/timer` tests where `time.Sleep` + `Wait` combos exist.~~ done (already done — timer suite rewritten on the virtual clock (51cddf2))
6. ~~Run `go fix` modernizers (`atomictypes`, `embedlit`, `slicesbackward`, `unsafesfuncs`).~~ done at `72e1505` (the named analyzers produced zero findings; rangeint/stringsseq/stringscutprefix/reflecttypefor/mapsloop applied)
7. Consider `maphash.Hasher` for service/realm key derivation (design check first — current uint64 realm keys may be fine).
8. Benchmark method vs free-function call paths (expect zero diff; verify).

**go-taskqueue × do (P1–P2, other repo):**
9. ~~Composition root wrapper returning a cleanup function (DO-2).~~ **Won't implement — go-taskqueue repo — out of scope for cordis.**
10. ~~`do.ProvideValue` for config/logger/DB (eager foundation).~~ **Won't implement — go-taskqueue repo — out of scope for cordis.**
11. ~~Lifecycle guards: `var _ do.ShutdownerWithError = (*MyService)(nil)`.~~ **Won't implement — go-taskqueue repo — out of scope for cordis.**
12. ~~Wire `samber-do-auditlog` via `Add*Hook`s.~~ **Won't implement — go-taskqueue repo — out of scope for cordis.**
13. ~~`ShutdownOnSignals` for daemon mode.~~ **Won't implement — go-taskqueue repo — out of scope for cordis.**
14. ~~Write the ~50-line broadcaster for `internal/webui/hub.go` instead of adopting `ro`.~~ **Won't implement — go-taskqueue repo — out of scope for cordis.**
15. ~~`goroutineleak` CI profile after `Shutdown()` in tests.~~ **Won't implement — go-taskqueue repo — out of scope for cordis.**
16. ~~Audit DO-1…DO-6 compliance once wired.~~ **Won't implement — go-taskqueue repo — out of scope for cordis.**

**Docs / memory (P2):**
17. ~~HARVEST this section into TODO_LIST.md / ROADMAP.md (docs-health).~~ done (docs-health pass this pass)
18. ~~Record the samber/* decision + verified stats in go-taskqueue's docs.~~ **Won't implement — go-taskqueue repo — out of scope for cordis.**
19. Update `samber-do-best-practices` skill verification block (v2.1.0, 342 importers, rollback repro result, date).
20. ~~ROADMAP entry: native-max API phase 3 (generic methods), with divergence note (Rust already has generic methods; Zig comptime — no divergence needed).~~ done (docs-health pass ROADMAP Go section carries the phase-3 idea and the open decision)
21. ~~Deprecation timeline decision for the free functions (aliases vs removal pre-v1).~~ done (docs-health pass parked in ROADMAP Open decisions by this pass)

**Research debt (P3):**
22. Runtime-test do v2's dependency-aware parallel shutdown if go-taskqueue will rely on it.
23. If `ro` is ever reconsidered: runtime repro required for timer-channel operators under 1.27 semantics.
24. Check do.samber.dev docs for the canonical `ShutdownReport` error-handling pattern.
25. ~~Evaluate stdlib `uuid` for any cordis uid needs (probably keep uint64 keys).~~ done (ROADMAP Go 1.27 section records considered-no-use (uuid; uint64 realm keys))
26. ~~`encoding/json/v2` adoption pass for the Go port (likely nothing to do — verify).~~ done (ROADMAP records considered-no-use (v1 is v2-backed))
27. `httptest.NewTestServer` if HTTP surfaces ever appear in tests.

**Repo hygiene (P3, mostly pre-existing):**
28. Local gopls/golangci-lint run Go 1.26 vs `go 1.27` go.mod — always use the devShell (already documented in AGENTS.md; consider a shell guard).
29. ~~Confirm the auto-commit daemon picked up nothing unexpected this session (tree was clean at 07:46).~~ done (clean well-scoped commits landed (51cddf2, b4650df))

## g) Questions (cannot be answered from this session)

1. **Enactment:** should the `samber/do` adoption actually happen now in go-taskqueue (not accessible from here) — and if yes, do you want me to draft the composition root + broadcaster there, or hand off this report?
2. ~~**Timing:** adopt the generic-method API migration in cordis now (pre-v1 window, rebase just completed) or park it in ROADMAP behind higher-parity work?~~ done (answered — not adopted; rationale + open decision recorded in ROADMAP)
3. ~~**Persistence:** write the verified facts (do v2.1.0 / 342 importers / rollback semantics / 1.27 feature list) into the `samber-do-best-practices` skill and ROADMAP now, or keep this session chat-only?~~ done (answered — persisted into ROADMAP/AGENTS by this pass)

---

_Generated 2026-09-08 07:46 CEST. Not committed (no commit was requested); the auto-git daemon will pick it up._
