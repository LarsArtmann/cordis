# Status Report: TODO-List Execution Sweep (panic-free follow-ups, golden scenarios #5/#6, audits, docs)

**Date:** 2026-11-15 07:22 CET
**Session:** single Crush session, one operator. Worked straight through the
TODO_LIST of 2026-09-10 vintage (plus the rows harvested on 2026-11-14),
following READ → EXECUTE → VERIFY per item.

## Scope line — what this report VERIFIED vs did NOT

**Verified (command/artifact cited):**

- Every code change is test-green on this machine: `nix run .#test` exit 0
  (Go `-race -count=3` per the flake check profile, Rust both variants,
  Zig leak-checked), `nix develop -c buildflow` exit 0, `nix flake check`
  "all checks passed", markdownlint clean, docs-gates battery 5/5 PASS
  (it started this session red on freshness + audit-all).
- The Zig OOM sweep found three real bugs; each fix has a pinning test
  (`zig/tests/oom.zig`) and a CHANGELOG `[Unreleased]` entry.
- Golden scenario #6 (logger) runs byte-identically in Go, Rust and Zig —
  generated from Go `GOLDEN_UPDATE=1`, matched first-run by Rust and Zig.
- Golden scenario #5 (snapshot/restore) runs byte-identically in Rust and
  Zig; Go has no snapshot API (discovered this session).
- Miri locally: `--test typed` 16/16 in 4.9 s, `--test golden` 6/6 in
  6.3 s, `--test parity` 39/39 in 11.7 s (all under
  `-Zmiri-ignore-leaks -Zmiri-disable-isolation`).
- Benchmarks: `cargo bench` 2026-11-15 vs the 2026-09-08 ROADMAP baseline
  — start/dispose ≈0.47 µs vs ≈0.56 µs, get 25 ns vs 24 ns, waterfall
  ≈0.32 µs vs ≈0.30 µs: all deltas inside run noise.
- erraudit: 0 violations across `go/...` after the loader changes.
- vulnix direct re-run: 9 findings / 3 packages (ShellCheck, cargo
  1.99.1, go 1.27.3), down from 4 packages / 11 CVEs; jq 1.8.2,
  perl 5.42.3, libmicrohttpd 1.0.10 all confirmed shipped.
- CVE verification ran through a subagent against the NVD API + upstream
  advisories; every verdict quoted in TODO_LIST carries that provenance.
- Go/Rust/Zig greeters executed locally with exact-output assertions
  (`Hello, world, from cordis!`).

**NOT verified (and why):**

- The TypeScript greeter was never executed: this tree has no
  `node_modules` (no `yarn install` ever ran here), so
  `examples/typescript/greeter.mjs` is verified by API-shape reading of
  `packages/core/tests/associate.spec.ts` only. Its CI smoke step is
  likewise unrun.
- All three workflow edits (ports.yml zig pin + miri job, nightly.yml
  cron, build.yml smoke) are untested until the next push — no act/
  actionlint pass was run.
- The docs-gates battery and every doc edit are local-state only; nothing
  is committed, so `nix flake check` in CI would still see the old tree.
- cargo-audit/cargo-deny were installed into the devShell and resolved
  (`cargo-deny 0.20.2`), but no `cargo-deny check` was wired into any
  gate (advisories DB needs network; licenses check never added to the
  flake rust check).
- The Zig OOM sweep exercises `restart`/`update` only at their budget
  boundary (their queue write provably allocates nothing today); the
  dispatch-phase OOM aborts stay out of contract per the user-gated end
  state and are untested by design.
- Miri was never run against the `thread-safe` variant.

---

## a) FULLY DONE

1. **Go: gopls infertypeargs cleanups** — `go/accessor.go:127`
   `Accessor[S]` → inferred, `go/coverage_test.go:337` `Start[int]` →
   inferred (config `0` carries the type argument). Both diagnostics
   cleared from gopls.
2. **Go loader: isolate-label validation at update time** — new
   `validateIsolateLabels` in `go/loader/entry_options.go` rejects an
   uncomparable label from `Tree.Update`/`Tree.Replace`/group diffs with
   a field-precise error (`isolate["logger"]: label of type T is not
   comparable`) BEFORE any option is stored or fiber restarted; the
   old path surfaced the generic `Context.Isolate` error at start time.
   Pinned by `TestUpdateRejectsUncomparableIsolateLabel` (three
   uncomparable label kinds + the comparable-label reload path).
3. **Go loader: `Entry.recordError` semantics documented + pinned** —
   last-writer-wins is now stated on the sink itself and pinned by
   `TestRecordErrorLastWriterWins` (sequential replace + two concurrent
   failing `SetConfig` reloads leaving exactly one survivor).
4. **go/loader/entry.go split** (Structure TODO trigger: semantic work
   touched it) — 490 lines → `entry.go` 165 (Entry type, getters,
   dispose/track), `entry_lifecycle.go` 211 (update/reconcile/init/start/
   buildContext), `entry_options.go` 165 (EntryOptions algebra +
   validation + EntryError + randomID). All loader tests green; golden
   traces untouched.
5. **Rust: `FiberId` encapsulation** — `rust/src/fiber.rs`: field is
   `pub(crate)`, added `pub(crate) const fn index()` (clippy
   `missing_const_for_fn` forced const), arena resolution goes through
   `id.index()`. Out-of-range indexing is unrepresentable outside the
   crate. Both clippy variants green.
6. **Rust: logger `Arg` conversions** — `From<u32>` (via `i64::from`)
   and `From<usize>` (documented saturation at `i64::MAX`), pinned by
   `logger_arg_from_unsigned_integers` in `tests/parity.rs`.
7. **Zig: per-site dispatch-abort markers** — all 17 `@panic` sites in
   `zig/src/cordis.zig` carry `// dispatch: no error channel`; the
   panic-allowlist gate still passes (17 pinned).
8. **Zig: the OOM sweep** — `zig/tests/oom.zig` (16 tests): an
   `OomPhase` allocator (armed/unarmed, budgeted) sweeps every
   registration allocation index of extend/isolate/isolateShared/
   withFilter/provideNamed/provide/attach/attachLabeled/onNamed/
   addExporter/snapshot/restore(empty)/accessor and asserts
   `error.OutOfMemory` with clean recovery; `start` disarms inside the
   plugin body (registration-phase sweep); `restart`/`update` pin
   "no allocation on the fallible pre-drain path" (their queue write
   reuses drain capacity — a future allocation added there will fail the
   sweep and force an OOM channel). This found the three real bugs in
   (d).
9. **Zig: `provideNamed` OOM fix** — resolves through `isolateKeyE`
   instead of the aborting `isolateKey` void query, so lazy root-realm
   allocation failure returns `error.OutOfMemory` through the API's own
   channel (found by the sweep).
10. **Zig: hooks poison fix** — the `getOrPut`-then-dupe pattern in
    `onNamed`/`onceNamed`/typed `on` left a freshly inserted entry with
    an undefined key and uninitialized ArrayList when the name dupe
    failed; deinit then panicked on garbage (`assertUnlocked`). Now the
    value is initialized first and the entry removed on dupe failure,
    at all three sites.
11. **Zig: `Context.init` leak fix** — the logger's eagerly allocated
    built-in buffer entry was outside the errdefer chain; a failed
    root-fiber allocation leaked it. `errdefer core.logger.deinit(gpa)`
    added.
12. **Zig: restart/update doc cross-links** — both doc comments now name
    the `internal/plugin` (fires before rollback, fiber still looks
    active) and `internal/status` (on settle) events, matching the Go
    and Rust ports' observable behavior.
13. **Golden scenario #6: logger service, all three ports** —
    `golden/scenario-logger.txt` + ops `log`/`dump-logs`/
    `expect-log-count` in all three runners; pins dispatch order
    (sequence = call order), the shared level taxonomy and the buffered
    entry shape at error/warn/info (DEBUG deliberately absent: the
    default exporter exports info and above; Go's nil-levels exporter
    would differ). Generated from Go, matched first-run by Rust and Zig.
14. **Golden scenario #5: registry snapshot/restore, Rust + Zig** —
    `golden/scenario-snapshot.txt` pins delta disposal and stash restart
    with last config. The Rust runner gained `GOLDEN_UPDATE` support
    (with a clippy `significant_drop_tightening` fix). The Zig cascade
    runner shares the lifecycle interpreter; a Runner.snapshot field
    carries the capture.
15. **FEATURES.md truth repair: Go snapshot/restore** — the parity row
    claimed FULLY_FUNCTIONAL for Go; no production snapshot/restore
    exists (only `TestRegistryDeleteRestoresSnapshot`, a different
    mechanism). Corrected to PLANNED with the evidence. This un-blocks
    nothing but stops the doc from lying.
16. **Greeter example in four trees + CI smoke** —
    `go/examples/greeter/main.go` (typed service + typed event),
    `rust/examples/greeter.rs` (Result-returning main, clippy
    `expect_used` denied), `zig/examples/greeter.zig` (installed by
    `zig build`, build.zig wiring added), `examples/typescript/
    greeter.mjs` (Service subclass). `Example smoke` steps assert the
    exact line in ports.yml (go/rust/zig) and build.yml (after the
    two-step build). Go/Rust/Zig verified locally with exact output.
17. **CI: zig pin repaired** — ports.yml pinned 0.16.0 while the port
    needs 0.17-only APIs (`@backingInt` at `cordis.zig:1898/1961`): the
    zig job was failing-by-config. Now 0.17.1 (tracking the nixpkgs zig
    the flake check uses) + `zig fmt --check src tests build.zig` step;
    the flake zig fmt check gained `examples`.
18. **CI: miri job** — nightly miri over golden/typed/parity with
    `-Zmiri-ignore-leaks -Zmiri-disable-isolation`, both flags justified
    in a job comment (tests intentionally leave Contexts undisposed;
    the golden runner reads scenario files from disk). Timed locally
    before pinning the 25-minute budget.
19. **CI: nightly multi-seed** — `TestDebounceThrottlePropertyMultiSeed`
    replays the debounce/throttle oracle across 32 PCG seeds behind
    `CORDIS_MULTI_SEED` (default suite stays on the one reproducible
    seed); `.github/workflows/nightly.yml` cron runs it with `-race`
    plus a full-suite sanity pass.
20. **Supply-chain tooling in the devShell** — `cargo-audit`,
    `cargo-deny` added to `flake.nix` devShell (verified resolvable via
    `nix develop -c`); buildflow's nix-run fallback for them is now
    unnecessary. `rust/deny.toml` was already committed (f6bf182).
21. **Workflow review verdict** — recorded: the flake job deliberately
    duplicates go/rust/zig with a hermetic `-race -count=3` profile
    (fast feedback vs hermetic gate — keep both). Resolved the
    TODO's "job-count/runtime review" row as a decision, not a trim.
22. **Environment audit closure** — vulnix direct re-run + CVE
    verification (subagent, NVD API + upstream advisories): the jq HIGH
    trio is CVE-2026-49839 / CVE-2026-40164 / CVE-2026-32316, all fixed
    in the shipped jq 1.8.2 — the IDs named in the 2026-09-28 report
    were wrong (2810 = Draeger ICMHelper, 21734 = Imagination DDK, 21735
    does not exist); perl CVE-2026-57432 real, fixed 5.40.5/5.42.3;
    libmicrohttpd CVE-2025-62689 real, fixed 1.0.3/commit-level;
    CVE-2023-6992 is the CLOUDFLARE zlib fork, not zlib-ng and not
    vanilla zlib — the 2026-09-28 "zlib-ng mismatch" suspicion was
    wrong. `yarn npm audit`: 7 findings (vite 7.3.6 ×4 incl. 1 high
    launch-editor, esbuild 0.27.7, eslint 8.57.1 deprecation), all in
    upstream-owned manifests.
23. **Doc pass** — PORTS.md "Fallibility shapes" section + the golden
    bullet refreshed to six scenarios; README.md "Native fallibility"
    invariant; ROADMAP.md "Typed-event guard error channel — decided"
    (keep the contract-bound panic; a Result-listener variant is the
    only sanctioned escape hatch, deferred to the breaking-surface
    window) + bench re-verification line; AGENTS.md OOM-sweep facts +
    the Waterfall/Isolate rebase-checklist note; FEATURES.md greeter/
    OOM-sweep/miri/logger-golden rows + the Planned section emptied;
    CHANGELOG `[Unreleased]` Added (9 entries) + Fixed (3 Zig bugs);
    TODO_LIST.md rewritten (done rows removed with evidence, open rows
    kept, new routed rows).
24. **LSP environment row closed as stale** — system Go is 1.27.3 and
    gopls v0.24.0/golangci-lint diagnostics verified working all session
    (the infertypeargs fixes above were driven by live gopls findings);
    the original root cause was the broken GOCACHE, already repaired
    2026-11-14. No wrapper needed.
25. **docs-gates battery red → green** — started FAIL (freshness +
    audit-all on the 2026-11-14_16-31 upstream report), ended 5/5 PASS
    after routing the #209 parity-test row, the branch-watch row, and
    correcting my own closure wording (see (d) 6).

## b) PARTIALLY DONE

1. **TypeScript greeter** — written, never executed (no node_modules in
   this tree; `yarn install` never ran here). Verified by API-shape
   reading only. The build.yml smoke step is unrun.
2. **Miri in CI** — job added on the strength of local runs, but the
   workflow itself (and every other workflow edit) awaits a push; the
   thread-safe variant was never miri-run.
3. **The OOM harness's contract boundary** — registration paths are
   strict; the dispatch-phase aborts are out of scope by the user-gated
   end state, and the accessor path's success-with-refused-allocation is
   tolerated because the harness cannot distinguish sanctioned
   degradation drops (`catch return` on emission payloads, the logger's
   `catch {}`) from real swallows.
4. **`restart`/`update` OOM coverage** — only the "no allocation today"
   regression pin exists; their drain internals remain abort-by-contract
   and untestable without changing that gate.
5. **cargo-deny wiring** — tools installed, no check wired into the
   flake rust check (licenses check would be offline-capable; advisories
   needs network).
6. **Golden #5 post-restore assertions** — a post-restore
   `expect-state consumer` reads the runner's stale pre-restore fiber
   handle (restore does not surface new fibers), so the restored state
   is pinned by the re-apply line only. A `expect-runtime-state`-style
   op would be stronger.
7. **erraudit scope** — run over the `go` module (where the changes
   are); the root stub module was not separately stamped.
8. **The snapshot/restore golden vs the report's "v2"** — the
   2026-09-10_08-25 report's §14 "Golden scenario v2" meant status/plugin
   EVENT ops, a different idea than §15's snapshot/restore ops. I landed
   the snapshot/restore one and initially overclaimed the other in
   TODO_LIST; corrected in this session's final minutes (row re-opened
   under Quality backlog). The status/plugin-events golden is NOT done.

## c) NOT STARTED

1. **The commit** — 39 changed files + this report sit uncommitted,
   awaiting the standing instruction (also TODO_LIST's own open row).
2. **Ports CI watch** — miri job, zig 0.17.1, example smokes, nightly
   cron: all first-run on the next push.
3. **rc.13 sync** — user-gated on three policy questions (plugin →
   component rename, yarn.lock adoption, hmr re-port scope; report
   2026-11-14_16-31 section g). Untouched, per that report's WAIT
   instruction.
4. **Upstream filings** — yarn-build bisect and the TS 7 TS2665
   reproducer (both cross-repo rows, verify-before-filing gated).
5. **crush-config lessons/oxfmt default** — two cross-repo rows needing
   commits in that repo.
6. **Cold-cache vulnix smoke** (existing row), **residual vulnix
   bucketing** (ShellCheck/cargo/go; existing row), **govulncheck
   one-off** and **godoc polish pass** (newly routed rows).
7. **#209 parity test** (routed; gated on the sync decision).
8. **Miri over thread-safe**; **actionlint over the edited workflows**;
   **cargo-deny gate wiring**; **status/plugin-events golden scenario**;
   **Go registry snapshot/restore** (now a FEATURES PLANNED row).

## d) TOTALLY FUCKED UP

1. **The FEATURES.md table realigner, twice.** My first pass split
   table cells on every `|` including the escaped `\|` inside backtick
   spans — silently redistributing cell content across phantom columns
   and recreating, verbatim, the historical bug the annotate gate
   documents as rule G ("pipe-mangled rows rebuilt from git history").
   I caught it only because the row count looked wrong on re-read; the
   rebuild came from `git show HEAD:FEATURES.md` + re-applying my four
   content edits + a pipe-aware splitter. If I had not diffed against
   HEAD, session content would have been silently destroyed. This was
   the single most dangerous move of the session.
2. **MD060 vs character width.** The rebuilt tables then failed
   markdownlint again because my splitter padded by Python `len()`
   while MD060 counts display width (✅ = 2 columns) — one more pass
   with a display-width-aware padder. Three formatting rounds for one
   formatting job because I hand-rolled a table formatter instead of
   reading the rule.
3. **Scripted-edit anchor rot.** Three separate python edit blocks
   failed their `assert old in s` (row padding drifted after each
   reflow, one partial-application ordering confusion with
   CHANGELOG/TODO_LIST). Each cost a round trip; the discipline I
   should have had: re-read the file immediately before every anchored
   edit batch.
4. **The zig cache half-delete.** `rm -rf .zig-cache/o` to force a fresh
   run left dangling manifest references and phantom "run test failure"
   noise; a full `.zig-cache` clean was needed. Never partially prune a
   content-addressed cache.
5. **The property_test.go refactor** — a two-step edit left a stray
   closing brace and mixed indentation (syntax error before gofmt could
   even run), plus one unused import later. Sloppy construction of a
   mechanical refactor.
6. **The Rust import surgery** — my regex import patcher duplicated
   `Arg` and mangled the use block; caught by clippy, fixed by hand.
7. **The miri PATH mixing** — the first miri invocation mixed the
   nixpkgs rustc with the rustup toolchain and produced 354 bogus
   compile errors (plus `cargo miri build` not existing as a subcommand)
   that sent me chasing phantom nightly breakage for two round trips.
   Also: TODO_LIST's quality-closure briefly overclaimed the report's
   "v2" item as done (see (b) 8) — caught by the battery, corrected.

## e) WHAT WE SHOULD IMPROVE

1. **Never hand-roll markdown table formatting again.** The failure
   mode (escaped pipes + display-width emoji) is exactly what the
   annotate gate's rule G exists for. Either edit rows in place with
   matching padding, or run a proven formatter.
2. **`git diff` immediately after any scripted multi-edit.** The
   FEATURES damage was invisible until diffed; a diff after the first
   pass would have cut two rounds.
3. **Read the lint rule before writing code to satisfy it** (MD060's
   display-width semantics).
4. **Verify example programs in the language's own build first**: the
   TS greeter's non-verification was known at write time and left
   standing.
5. **For golden scenarios, run first and review the trace, then commit
   the expected file** — my two wrong registry-size expectations
   (forgot the provider runtime counts; the stale-handle boundary) would
   have been skipped.
6. **Validate YAML workflows with actionlint before declaring CI work
   done** — none of the three edited workflows passed any local check.
7. **The OOM harness could keep its strict swallow-tripwire** if
   sanctioned drop sites were distinguishable (e.g. a tagged allocator
   keyed by return address) instead of globally tolerated on success.
8. **Subagent fan-out was underused**: the CVE research went to a
   subagent (right call), but the doc pass and the two golden scenarios
   ran serially in-session; the interpreters could have been fanned out
   per language with the shared scenario fixed first.
9. **Anchor-edit discipline**: for docs, prefer unique-substring
   replacement over full-line anchors that include table padding.
10. **Close the loop on buildflow's timing regressions** — this run's
    summary showed cargo-audit +22996%, vulnix +355%, golangci-lint
    +316% etc.; observed again, still never investigated (same note as
    the 2026-11-14 report).

## f) Things to get done next (up to 50, grouped, impact-ordered)

**Immediately gated on the user:**

1. Commit the working tree (39 files + this report) — one commit or
   logical splits (question g1).
2. Push and watch Ports CI: the miri job, the zig 0.17.1 pin, three
   example-smoke steps and the nightly cron are all first-run.
3. Answer the rc.13 sync questions (plugin → component rename,
   yarn.lock adoption, hmr re-port scope — 2026-11-14_16-31 section g).
4. Decide whether the TS example gets a local `yarn install`
   verification or CI-only (question g2).
5. Decide whether Go registry snapshot/restore gets implemented now
   (question g3).

**First coding session after the commit:**

6. `actionlint` (or `act`) pass over ports.yml, build.yml, nightly.yml.
7. TS greeter local verification (`yarn install && yarn build core &&
   node examples/typescript/greeter.mjs`).
8. Miri over the `thread-safe` variant locally; decide CI scope from
   the timing.
9. Wire `cargo-deny check licenses sources bans --offline` into the
   flake rust check; leave advisories to a scheduled CI run.
10. Add a scheduled `cargo-audit` (advisories) job to nightly.yml.
11. Golden DSL: a post-restore state assertion op (restore surfaces new
    fibers, or `expect-runtime-state <plugin> <state>`), strengthening
    scenario #5.
12. Golden scenario: status/plugin event ops (the 2026-09-10_08-25 §14
    idea) — re-opened row.
13. Extend the logger golden with exporter ops and level-intercept ops
    (pins the level-filtering contract that currently lives only in
    unit tests).
14. Extend the logger golden (or a new one) with `set-buffer-size` to
    pin bounded-buffer eviction.
15. Go: design + implement registry snapshot/restore (closes the
    FEATURES PLANNED row; un-blocks #5 in Go).
16. OOM harness: tagged-allocator variant to distinguish sanctioned
    degradation drops from real swallows, restoring the strict
    success-branch tripwire.
17. Zig: audit every `catch return` / `catch {}` degradation drop in the
    logger/emission paths against the "the error log drops its line"
    contract — drops should be observable, not silent.
18. Zig: `getNamed`/`depsReady` still resolve through the aborting
    `isolateKey`; add `isolateKeyE`-based internal callers where a
    channel exists.
19. Loader: fuzz `validateIsolateLabels` (extend `config_fuzz_test.go`).
20. Re-check `nix flake check` output for new nix deprecations on the
    next bump (clean this run).
21. Split `go/fiber.go` (759) / `go/coverage_test.go` (660) when next
    touched semantically (existing row).
22. godoc polish pass (newly routed row).
23. `govulncheck ./...` one-off for the Go port (newly routed row).
24. Cold-cache smoke of `scripts/vulnix-audit.sh` (existing row).
25. Bucket the residual vulnix findings (ShellCheck/cargo/go; existing
    row).
26. Investigate buildflow timing regressions (cargo-audit +22996%,
    vulnix +355%, golangci-lint +316% — observed twice now, never
    investigated).
27. golangci 311 warnings: triage class-by-class, then decide keep-
    visible vs suppress-with-rationale (carried from 2026-11-14).
28. Update `docs/DOMAIN_LANGUAGE.md`: the Snapshot/restore entry says
    "the Rust registry checkpoint" — Zig has it too, Go does not.
29. PORTS.md fallibility section: add the Zig OOM-sweep pin reference
    (`zig/tests/oom.zig`) next to the abort list — partially there
    already, tighten.
30. Consider a `rust/examples/README` or crate-doc mention of the
    greeter example (cargo run --example greeter).

**Gated on the rc.13 sync (do not start before the decision):**

31. Sync `packages/` + bump `.github/UPSTREAM_PIN` (2026-11-14_16-31
    §f1–f10 protocol).
32. Port #242 (mid-dispatch listener removal) to all three ports + tests.
33. Parity tests for #209/#196/#133/#191/#56/#134 (incl. the routed
    #209-second-half row).
34. `manifest-parity.mjs` allowlist update (eslint out, oxlint/semver/
    shx/@cordisjs/tsx in; re-decide @types/node).
35. `hmr-fixture-canary.mjs` compatibility after the fixture renames.
36. `.buildflow.yml` eslint-fix skip rationale rewrite (oxlint era).
37. AGENTS.md stale-facts update in the sync commit (yarn.lock policy,
    eslint → oxlint, pin, @types/node).
38. Two-step clean build + full TS suite after the sync.
39. Decide/adopt upstream's type-check tests (#337) in CI.
40. Verify `internal/plugin` event name survived the component rename
    upstream.
41. ROADMAP parity matrix: new upstream packages as unported columns;
    replace/unshift as parity rows.
42. hmr re-port evaluation (#244/#269) if g3 says re-port.

**Cross-repo (separate sessions):**

43. Bisect the single-step clean `yarn build` failure upstream (existing
    row).
44. TS 7 TS2665 minimal reproducer + upstream issue (existing row).
45. crush-config: oxfmt/strict-JSONC lesson (existing row).
46. crush-config: fleet `.oxfmtrc.json` default (existing row).

**Hygiene:**

47. Annotate this report's §f items as they land (the freshness gate now
    watches it).
48. Keep `audit_max`-style ratchets honest: the battery's audit-all went
    1 → 5 → 0 during this session; never raise thresholds to pass.
49. Re-run `scripts/vulnix-audit.sh direct` after the next nixpkgs bump
    (standing row).
50. Trim TODO_LIST's "Closed" narrative paragraphs if they grow past
    scan-length — they are history, CHANGELOG owns history.

## g) Questions I cannot figure out myself

1. **Commit shape:** the 39-file working tree (+ this report) awaits
   your commit instruction. One commit ("TODO-list execution sweep") or
   logical splits (loader fixes / Zig OOM fixes / golden scenarios / CI
   / docs)?
2. **TS greeter verification:** may I run `yarn install` on this tree to
   verify `examples/typescript/greeter.mjs` locally (it creates
   node_modules and re-resolves the floating dependency set — the
   known risk class from the TS 7 incident), or should the build.yml
   smoke step be its only verification until the next push?
3. **Go registry snapshot/restore:** implement it now (closes the
   corrected PLANNED row and un-blocks golden #5 in Go, but touches the
   registry right before a sync where upstream may reshape it), or
   park it behind the rc.13 sync decision?

---

_Report ends. Working tree uncommitted by standing instruction; all
local gates green at report time._
