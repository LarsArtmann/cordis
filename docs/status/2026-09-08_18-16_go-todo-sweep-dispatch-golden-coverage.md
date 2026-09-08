# Status Report — 2026-09-08 18:16 — Go TODO sweep: dispatch golden, coverage bar, timer/hmr/loader hardening

Session scope: executed the **entire `## Go (flagship)` section of
`TODO_LIST.md`** (11 items) end-to-end, plus the verification and doc
passes around it. All three port suites green, `nix flake check` fully
passed, erraudit clean, `go fix` clean, markdownlint clean.

Working state at report time: 30 files changed (uncommitted), all gates
green locally. Rust had one post-hoc cleanup (dead clones in
`rust/tests/golden.rs:412-415` removed; clippy 0 after).

---

## a) FULLY DONE

Each item: what, evidence, scope.

1. **Golden scenario #4 — dispatch modes (bail/serial/waterfall/cut/parallel),
   byte-identical across all three runners.**
   Evidence: `golden/scenario-dispatch.txt` + `expected-dispatch.txt`;
   runners in `go/golden_test.go` (`TestGoldenDispatch`), `rust/tests/golden.rs`
   (`golden_scenario_dispatch`), `zig/tests/golden.zig` ("golden scenario
   dispatch"); Zig embeds via `zig/build.zig`. Rust passed the byte-compare on
   the first run against the Go-generated expected file; Zig after fixing a
   shared-counter bug in its runner. Scope: `golden/`, all three runners,
   `golden/README.md` (table row #4).
2. **`timer.IntervalFunc` pump-lifetime fix + constraint documentation.**
   Evidence: `go/timer/timer.go` rewritten (single pump goroutine, post-tick
   stop re-check); `TestIntervalFuncNoCallbackAfterDispose` pins "no callback
   starts after disposal even with pending ticks" — this test FAILS against
   the old queueing implementation by construction; `-race` green. Scope:
   `go/timer/`.
3. **`Tree.Await` decision + implementation: surface, don't discard.**
   Evidence: `go/loader/tree.go` `Await` now routes `f.Await()` errors into
   the entry error sink via new `Entry.recordError` (`go/loader/entry.go`);
   `//nolint:erraudit` removed (error now used); docs updated on `Await`,
   `Errors`, `Entry.Err`; `TestAwaitSurfacesRuntimeFailure` pins the
   poisoned-restart → `Errors()` path. Scope: `go/loader/`.
4. **hmr rollback errors carry `Fiber.Err()` detail.**
   Evidence: `go/hmr/hmr.go` Swap failure path now returns
   `hmr: entry <id> failed under the new implementation: <cause>` with `%w`;
   `TestSwapRollsBackOnFailure` asserts both the context line and the
   `boom` detail. Scope: `go/hmr/`.
5. **`Resolver.ReplaceType[C]` sugar + `TypedRegistration` builder.**
   Evidence: `go/loader/resolver.go`; `RegisterType`, `ReplaceType`, hmr
   `SwapType` and test helpers all compose on `TypedRegistration`; nil apply
   yields zero Registration so `Register`/`Replace` fail fast with the
   missing-factory error (a latent start-time-failure time bomb removed);
   `TestResolverReplaceType` covers fresh-name, swap, decode-through-new-reg,
   nil-apply. Scope: `go/loader/`, `go/hmr/`.
6. **Loader statement coverage 74.6% → 90.8%.**
   Evidence: `go test -coverprofile` total line reads 90.0% → 90.8% after the
   last additions; new `go/loader/coverage_test.go` (root-group
   Create/Entries/Data/Remove, anonymous IDs via `randomID`, group Update
   scope-change reconcile, Move into nested group + no-op case,
   `EntryError` formatting/unwrap, `Resolver.Resolve`/`Decode`,
   `Tree.Locate`/`Context`, config error paths, `Reload` without path,
   double-`Serve` guard, `Tree.Refresh` relink-in-place, `Loader.Locate`).
   Scope: `go/loader/`.
7. **hmr concurrency storm test.**
   Evidence: `TestSwapCreateRemoveStorm` in `go/hmr/hmr_test.go` — 4 swap
   workers × 10 swaps racing 4 creator/remover workers × 10 entries +
   late-entry removals; asserts no Create/Remove errors, all surviving
   entries settle ACTIVE, post-storm clean swap applies. Green under
   `-race -count=3` (CI-parity per TODO_LIST item 35).
8. **Timer debounce/throttle property test.**
   Evidence: `go/timer/property_test.go` (`TestDebounceThrottleProperty`) —
   fixed-seed PCG, 40 rounds, virtual-time bubble, rejection-sampled gaps
   that never land on a pending fire deadline, independent event-simulation
   oracle for debounce + throttle + no-trailing throttle; validated against
   4 different seeds then restored to the canonical seed. The oracle is
   discriminating (it caught a real first-call leading-fire bug in itself).
9. **Wrapped-error regression tests (accessor.go / tree.go).**
   Evidence: `TestAccessorWrappedStartError` (context prefix
   `cordis: accessor of <ServiceName[S]()>` + `errors.Is` to
   `ErrInactiveEffect` through the real pre-flight path — a disposed plugin
   fiber context; projection failures deliberately go to `StateFailed`, NOT
   to the `Accessor` return), `TestTreeCreateMoveErrorContext` (all four
   exact wrapped messages for Create/Move).
10. **Go 1.27 `go fix` modernizer sweep.**
    Evidence: `rangeint` (`for range workers`), `stringsseq` (`SplitSeq`),
    `stringscutprefix` (`CutPrefix` + manual cleanup of the fixer's partial
    hunk in `golden_test.go`), `reflecttypefor` (`reflect.TypeFor[T]()`),
    `mapsloop` (`maps.Copy` in loader); `go fix -diff ./...` now empty;
    build/vet/tests green after. Scope: `go/fiber_test.go`, `go/golden_test.go`,
    `go/typed.go`, `go/loader/entry.go`.
11. **erraudit regression fixed (sentinel declaration).**
    Evidence: `go/errors.go:36` — `ErrInactiveEffect` now declared
    `var ErrInactiveEffect error = &Error{...}` per the tool's
    recommendation (call sites' `errors.Is` matches the sentinel guard);
    `erraudit lint ./... --type-aware` exit 0. All usages were
    interface-compatible (verified before the change). Applied per the
    go-error-modernization skill; `errors.As` → `errors.AsType` migrated in
    the one new call site I introduced.
12. **Loader watch/reload golden transcript (Go-only) + verdict.**
    Evidence: `go/loader/watch_golden_test.go` +
    `go/loader/testdata/watch-golden.txt` — full transcript: initial start,
    in-place config update, reload diff (restart + group diff + fresh entry),
    self-dispose with persistence side effect, close cascade; deterministic
    (`-count=5` and `-race` green); `GOLDEN_UPDATE=1` escape hatch. Verdict
    recorded: a cross-port golden is N/A (loader has no Rust/Zig port);
    `golden/` stays three-runner-only. Scope: `go/loader/`, AGENTS.md.
13. **Docs synchronized.**
    Evidence: `TODO_LIST.md` Go section removed entirely (all 11 items
    resolved; per its header, completed work lives in CHANGELOG only);
    `CHANGELOG.md` Added/Changed/Fixed entries; `FEATURES.md` golden row
    3 → 4 scenarios; `golden/README.md` scenario table; `PORTS.md`,
    `README.md`, `go/README.md`, `ROADMAP.md` golden counts; `AGENTS.md`
    (coverage 90.8%, four golden scenarios + watch-golden fact, the
    untracked-files-invisible-to-flake gotcha). markdownlint clean on all
    touched files.
14. **Full verification gates.**
    Evidence: Go `build`/`vet`/`test -race ./...` green; erraudit exit 0;
    `go fix -diff` empty; Rust `cargo test` green on default AND
    `--features thread-safe` (6/6 golden tests byte-identical);
    `cargo clippy --all-targets` 0 findings (default features — the gated
    config); Zig `build test` 30/30; `nix flake check` **all checks passed**
    (go vet+race, rust clippy+tests, zig tests).

---

## b) PARTIALLY DONE

1. **Zig golden runner does not execute scenario #3 (cascade).**
   Works: Zig runs lifecycle, events, and the new dispatch scenario (30/30).
   Missing: `scenario-cascade.txt` is Go+Rust only — `zig/build.zig` embeds
   no cascade files and `golden.zig` has no cascade test, while
   `golden/README.md` presents `golden/` as "one spec, three runners" and
   its regenerate instructions imply every runner runs every scenario.
   Predates this session; discovered while adding #4. Blocker: cascade ops
   (`spawn`, `delete`, `expect-registry-size`) need registry identity —
   TODO_LIST's Zig item "Registry `has`/`delete` keyed by `TypedPlugin`
   identity" is the same prerequisite. Effort: M.
2. **Dispatch golden covers count-only determinism for `parallel`.**
   Works: fan-out count pinned (interleaving-independent).
   Missing: no combined realm/global-filter ops with bail/waterfall (events
   scenario covers filters, dispatch scenario covers modes, nothing covers
   both together). Effort: S per op added.
3. **Timer property test covers fire counts + debounce arg delivery.**
   Works: counts for all three schedules, debounce last-arg check.
   Missing: throttle trailing-arg delivery (`lastArgs`) is not asserted;
   model would need arg tracking per window. Effort: S–M.
4. **`Tree.recordError` last-writer-wins is undocumented as a contract.**
   Works: runtime failures land in the sink; successful restart clears.
   Missing: which error wins when a concurrent reload's `init()` and an
   `Await`'s `recordError` interleave is serialized by `t.mu` but the
   "latest wins" semantics are not stated in docs or pinned by a test.
   Effort: S.
5. **Rust thread-safe clippy: 19 pre-existing `src/` findings left alone.**
   Verified pre-existing (all in `core.rs`, `events.rs`, `fiber.rs`,
   `plugin.rs`, `service.rs`, `snapshot.rs`; none in tests I touched) and
   not gated per AGENTS.md. Still open by design; listed for completeness.
6. **`go fix` sweep was smaller than the TODO item implied.**
   The item named `embedlit`, `unsafefuncs`, `atomictypes`; those three
   analyzers produced zero findings on this codebase — the applied fixers
   were rangeint/stringsseq/stringscutprefix/reflecttypefor/mapsloop.
   Done, but the item's expected content didn't match reality; the finding
   is recorded here so nobody "re-runs for the named analyzers" expecting
   diff.
7. **Rust/Zig dispatch runner structure diverges slightly from Go.**
   All traces byte-identical, but the Rust helper needs the terminal via
   closure capture while Zig needs file-scope state for its bare-fn-pointer
   terminal. Cosmetic divergence only; documented in runner comments.

---

## c) NOT STARTED

Untouched this session (kept in TODO_LIST; no code written, not researched
beyond what was already in context):

- **Rust section (5 items):** `internal/plugin` + `internal/update`
  interception events (M13 parity); root-fiber status emission
  verification (`FiberData::new_root`); fix/allowlist `significant_drop`
  under `--features thread-safe` then gate clippy on it; `cargo-llvm-cov`
  baseline; `cargo bench` to substantiate the "up to 30% faster small
  allocations" claim.
- **Zig section (2 items beyond the cascade gap in (b)):**
  `zig build -femit-docs` pass / fix doc comments; record Zig 0.16 std
  gotchas in AGENTS.md.
- **Repo section (10 items):** CI `nix flake check` job; `.prettierrc` +
  prettier/yarn-build CI steps; `packages/**` byte-identity CI guard +
  `dprint.json` excludes guard; post-push verification of build.yml and
  ports.yml (user-gated, force-with-lease); gitignore `tmp-*` debris;
  install-from-scratch TS verification; CONTRIBUTING.md accuracy review;
  loader JSON config fuzzing; local-gate `-count=3` alignment with CI.
- **ROADMAP:** logger golden scenario (the core logger service has no
  golden coverage).

Not started because the session was scoped to the Go TODO section; the
Rust/Zig/Repo sections were explicitly out of scope.

---

## d) TOTALLY FUCKED UP

1. **AGENTS.md's "erraudit green" claim was stale — the gate was actually
   red this session.**
   Severity: process-trust damage; blocked the modernizer sweep's exit
   criterion. The 2026-09-08 05:27 session recorded "erraudit is green",
   but this session's first run flagged `errors.go:36` (sentinel declared
   with concrete type `*Error`). Root cause: unknown — either the
   `~/go/bin` erraudit binary was rebuilt with newer analyzers between
   sessions, or the earlier run used different flags (`--type legacy_as`
   vs `--type-aware`). Mitigation: fixed on sight (interface declaration);
   the claim in AGENTS.md was updated to carry the exact command
   (`.status` report 05:27 said "No violations" — unverifiable which
   invocation produced it).
2. **Untracked files are invisible to `nix flake check` — cost a full
   failed gate round trip.**
   Severity: time (one ~5-10 min check wasted + diagnosis). I created
   `golden/scenario-dispatch.txt` etc. but did not `git add` them before
   running the flake gate; Nix flakes in a git repo copy only git-tracked
   files into the store, so the sandbox hit `NotFound` on the new scenario.
   Mitigation: files staged, gotcha documented in AGENTS.md. A repo guard
   (golden files must be git-tracked, or a check that fails fast with a
   clear message) is proposed in (f).
3. **The dispatch scenario shipped two design/implementation bugs that the
   golden mechanism itself cannot catch.**
   Severity: correctness-of-intent, not of determinism. (a) `returns=` was
   absolute, so `serial chain payload=300` produced the nonsensical
   `result=220`; fixed to delta semantics (`payload+returns`), but the DSL
   key name still says "returns" — a rename trap for future scenarios.
   (b) The runner counter maps in BOTH Go and Zig were per-listener
   overwrite instead of per-event shared, so the first generated trace said
   `parallel sync fired=1`. Only caught by hand-verifying the generated
   expected file against intent — "deterministic" ≠ "correct".
4. **Zig golden coverage gap contradicts the docs (split brain).**
   `golden/README.md` and `PORTS.md` present `golden/` as three-runner
   byte-parity, but Zig has no cascade runner at all (see (b)1). Anyone
   trusting the README would assume scenario #3 is pinned in all ports.
5. **Tool-environment friction is permanent and still active:**
   the system Go is 1.26 with `GOTOOLCHAIN=local` while the module demands
   1.27, so **gopls/LSP diagnostics on every Go file in this session were
   garbage** (24 project errors, all the same toolchain error). Harmless
   once you know, but it means editor/LSP output can never be trusted here
   — only CLI runs under the flake toolchain are truth.
6. **Session self-critique — what I forgot / did worse:**
   - Forgot `git add` before the flake gate (item 2 above) — the single
     biggest time sink of the session, and it was foreseeable from the
     flake's own AGENTS.md documentation of `builtins.path` semantics.
   - Should have hand-derived the expected dispatch trace BEFORE generating
     it; the absolute-vs-delta flaw would have been caught at design time
     instead of after (item 3a).
   - Wrote a first-draft property-test model with leftover exploratory
     methods and a first-call leading-fire bug; rewrote it cleanly, but the
     discard should never have been written.
   - `python str.replace` without asserts no-op'd the `zig/build.zig` edit
     twice (backslash escaping in Zig multiline strings); lost two rounds.
   - Three forgotten imports (`strings` twice, `fmt` once) and one
     `fmtSprintf` slip — noisy, cheap, but avoidable with one compile
     before running.
   - Shipped dead `counters2`/`counters3` clones in the Rust refactor that
     clippy could not see through `let _ =` — only found during the
     self-review for THIS report and removed post-hoc (clippy + tests
     re-verified green after removal). Lesson: `let _ =` silencing is a
     code smell that defeats both the compiler and the linter.
   - Two edit-tool retries burned on stale-mtime rejections (file touched
     between read and edit); should have re-read once instead of retrying.
   - One accidental no-op shell call (perl to /dev/null) — noise.

---

## e) WHAT WE SHOULD IMPROVE

1. **Gate claims in AGENTS.md must carry the exact command.** "erraudit is
   green" without `erraudit lint ./... --type-aware` (and the binary build
   date) proved non-reproducible. Fix: every gate-claim line names its
   command; a stale claim then self-identifies when the command drifts.
2. **Golden expected files must be hand-reviewed against intent, not just
   generated.** The golden mechanism pins determinism, not correctness.
   Proposal: scenario files carry a comment block with the hand-derived
   expected trace; the generator output is diffed against it mentally once
   (this session's near-miss).
3. **`let _ =` silencing in Rust tests should be banned** (clippy can't see
   through it). Add `unused_variables`-safe style: remove instead of
   silence; the one real use case (deliberate drops) gets a comment.
4. **Stage new files before running `nix flake check`.** Cheap `git add` at
   the end of every code batch. Better: a tiny guard — `nix flake check`
   wrapper that fails fast if `golden/` files are untracked.
5. **The dispatch DSL key `returns=` is a lie (it is a delta).** Rename to
   `delta=` (or document louder) before more scenarios accrete on top of
   it; the rename cost grows with every new runner op.
6. **Property-test oracles deserve seed sweeps in CI.** The fixed seed is
   reproducible but can hide schedule-dependent divergence; a cheap nightly
   multi-seed run (4 seeds proven to work) would catch drift.
7. **Zig parity story needs a decision, not accretion:** either Zig runs
   all golden scenarios (cascade prerequisite: registry identity work) or
   golden/README gains an explicit per-scenario runner matrix. Right now
   the docs oversell Zig.
8. **Fix the editor story for go/ (1.26 vs 1.27):** point gopls at the
   flake's Go 1.27 (editor config or wrapper) so LSP diagnostics stop
   being noise that must be mentally filtered in every session.
9. **Watch-golden pins debatable semantics** (`partial b` emitted twice —
   once on self-dispose, once again on close for the already-disabled
   entry). If double-emission is deemed wrong, fix the port now while the
   golden is one day old; the transcript makes the debate concrete.
10. **Rust thread-safe clippy debt (19 findings) keeps growing risk** every
    toolchain bump (AGENTS.md notes new nursery lints appear after
    bumps). Timebox a fix-or-allowlist pass so the feature can eventually
    be clippy-gated like the default build.

---

## f) Up to 50 things to get done next

Brainstorm ranked by impact (feeds docs-health HARVEST; not a commitment).
Format: task — Impact / Effort / Category.

1. Push the branch (user-gated, force-with-lease per repo policy) and
   verify build.yml + ports.yml green on GitHub, including the replayed
   `3-stage-hmr` line. — Critical / S / Quality
2. Add a CI job running `nix flake check` so the port gate is enforced
   remotely (ports.yml has no nix step; all flake verification is currently
   local-only). — Critical / M / Quality
3. Install-from-scratch TS verification:
   `rm -rf node_modules && yarn install && yarn build && yarn test`. —
   High / M / Quality
4. Zig: run scenario #3 (cascade) — prerequisite: registry identity.
   Decision needed first (see g1). — High / M–L / Feature
5. Zig: registry `has`/`delete` keyed by `TypedPlugin` identity (also
   unblocks 4). — High / M / Feature
6. Rust: `internal/plugin` + `internal/update` interception events (M13
   parity; Go has them). — High / M / Feature
7. Rust: verify/document root-fiber status emission
   (`FiberData::new_root` bypasses `settle_state`; `rust/src/fiber.rs:82`).
   — Medium / S / Quality
8. Rust: fix or allowlist `significant_drop` findings under
   `thread-safe`, then gate `cargo clippy --features thread-safe` in Ports.
   — High / M / Quality
9. Rust: `cargo-llvm-cov` coverage baseline next to the Go numbers. —
   Medium / M / Quality
10. Rust: `cargo bench` to substantiate or hedge the "up to 30% faster
    small allocations" ROADMAP claim. — Medium / M / Quality
11. Zig: `-femit-docs` pass; fix broken doc comments. — Medium / S / Docs
12. Zig: record 0.16 std gotchas in AGENTS.md (`std.Io.Dir.cwd`,
    `ArrayListUnmanaged .empty`, anonymous non-zig imports). — Medium / S /
    Docs
13. CI: `.prettierrc` (printWidth 100) + `prettier --check` + `yarn build`
    before tests in build.yml. — High / S / Quality
14. CI guards: `packages/**` stays byte-identical to upstream;
    `dprint.json` excludes keep covering `packages/**`. — High / M / Quality
15. Gitignore `tmp-*` test debris. — Low / S / Cleanup
16. Review CONTRIBUTING.md; add flake app list to quickstart; document the
    "upstream semantics + fork formatting" rebase policy. — Medium / M /
    Docs
17. Loader: fuzz the JSON config layer (EncodeConfig/DecodeConfig
    roundtrip with random shapes). — Medium / M / Quality
18. Align local gate with CI race canary: flake checks `-race -count=1`
    vs ports.yml `-count=3`. — Medium / S / Quality
19. Logger golden scenario (core logger service has none). — Medium / M /
    Quality
20. Rename dispatch DSL `returns=` → `delta=` (spec + three runners +
    regenerate) before more scenarios accrete. — Medium / S / Cleanup
21. Add per-scenario runner matrix to `golden/README.md` (or give Zig the
    cascade runner) to kill the oversell. — High / S / Docs
22. Guard: golden files must be git-tracked; fail `nix flake check` fast
    with a clear message when untracked (prevents today's NotFound class).
    — Medium / S / Quality
23. Document erraudit's exact invocation + binary build date next to the
    green claim in AGENTS.md. — Medium / S / Docs
24. Pin the editor/gopls toolchain to the flake's Go 1.27 so LSP output is
    trustworthy. — Medium / S / Quality
25. Nightly multi-seed run for `TestDebounceThrottleProperty` (4 seeds
    proven). — Low / S / Quality
26. Extend the property oracle to throttle trailing-arg delivery. — Low /
    S / Quality
27. Add realm/global-filter ops to the dispatch golden (combined filter ×
    mode coverage). — Low / M / Quality
28. Decide `partial b` double-emission on close (watch-golden transcript
    makes it concrete); fix port or pin docs. — Medium / S / Bug
29. Document `Tree.recordError` last-writer-wins vs first-failure-wins and
    pin with a concurrent-reload test (see g3). — Medium / S / Docs
30. hmr: add a golden-scenario op if cheap (rollback chains, from 05-27
    report §f32). — Low / M / Quality
31. File the buildflow nix-checker platform-mismatch issue upstream
    (filter enumerated checks to the running system) and reference it in
    AGENTS.md. — Medium / S / Cleanup
32. buildflow: language-detect yarn repos in the pnpm-audit step. — Low /
    S / Cleanup
33. buildflow: scope vulnix noise to the flake's own derivations. — Low /
    M / Cleanup
34. Verify the flake checks on aarch64-darwin (CI runner or remote
    builder). — Medium / M / Quality
35. Pin the flake's zig version explicitly if nixpkgs drifts from 0.16.
    — Low / S / Quality
36. `nixConfig`/CI guard asserting the formatter attr exists (the original
    nix-fmt failure mode). — Low / S / Quality
37. Sweep `go/` for bare `_ =` discards erraudit doesn't flag; align style.
    — Low / M / Cleanup
38. Go: consider a debug build tag turning core-lock misuse into panics
    (Rust thread-safe found two deadlocks this way; TODO_LIST item). —
    Medium / M / Feature
39. Close the `Member.Set` service-blip question: document restart-on-write
    or implement update-in-place derivation. — Medium / S / Docs
40. Rust: same three upstream-feature assessments (watch/include/bare
    specifier) → ROADMAP entries or implementations. — Medium / M / Docs
41. Zig: same three assessments → ROADMAP entries. — Low / M / Docs
42. Check upstream for `RequireNoResidue`/issue-#2 progress; update
    ROADMAP's bidirectional-feedback section. — Low / S / Docs
43. Upstreamable PR: make hmr spec replaces style-agnostic regexes. —
    Medium / M / Cleanup
44. CI hash-pin: hmr fixtures byte-identical to upstream (cheap guard).
    — Medium / S / Quality
45. Move include test tmp-* files to os.tmpdir (upstreamable; they litter
    fixtures on failure) + cleanup-on-failure hardening. — Low / M /
    Cleanup
46. Decide yarn.lock policy (commit one vs stay lock-free like upstream).
    — Medium / S / Docs
47. Add a debug/CI flag for the synctest bubble policy (lint or review
    checklist item) so new timing tests keep using virtual time. — Low /
    S / Quality
48. `go/README.md`: mention the four golden scenarios and the watch
    golden transcript. — Low / S / Docs
49. PORTS.md: record `TypedRegistration` as the shared registration
    builder in the port-architecture section. — Low / S / Docs
50. Consider a dispatch-scenario "empty event waterfall" op (terminal-only
    run) — currently untested in the golden, covered only by port unit
    tests. — Low / S / Quality

---

## g) Questions I cannot figure out myself

1. **Zig cascade coverage — invest or document?** Giving Zig the cascade
   golden requires the `TypedPlugin`-keyed registry identity work first
   (M-size, TODO_LIST item). Do you want Zig to run all four golden
   scenarios (do the registry work), or should the docs state honestly
   that Zig runs 3 of 4 and route the registry item to ROADMAP?
2. **Dispatch DSL naming — breaking rename now?** `returns=` actually
   means `payload + N` (delta). Renaming to `delta=` costs one
   regeneration cycle across the three runners and the golden files
   (~30 min) but the cost grows with every future scenario. Rename now,
   or keep `returns=` with its loud comment?
3. **`Tree.Errors()` failure precedence under concurrent reloads.** With
   `recordError` (this session) an `Await`-observed runtime failure and a
   concurrent `init()` start error both write `e.err` under `t.mu` —
   last writer wins. Do you want "latest failure wins" (current, matches
   `start()` clearing on success) or "first failure wins" (keeps the
   original start error) pinned as the documented contract?

---

*Report generated per the status-report skill; format override honored:
user explicitly requested Markdown (`.md`) instead of the skill's HTML
default. Section (f) is HARVEST input for `TODO_LIST.md`/`ROADMAP.md`.*
