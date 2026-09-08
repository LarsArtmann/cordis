# Status: Zig typed registry + docs gate + 0.16 gotchas — with brutal self-review

**When:** 2026-09-08 20:39 CEST
**Session scope:** the three open `TODO_LIST.md → ## Zig` items: (1) registry
`has`/`delete` keyed by `TypedPlugin` identity, (2) a `-femit-docs` doc-comment
pass, (3) recording verified Zig 0.16 std gotchas in AGENTS.md. One unplanned
root-cause bug fix fell out of item 1. This report covers only this session and
what it directly observed; a parallel session was mutating `rust/` and
`packages/` concurrently (visible in git status, not covered here except where
it collided with this session).

**Format note:** the status-report skill's canonical output is a styled HTML
dashboard; the user explicitly requested `.md` this time, so this file is
Markdown and the HTML treatment was skipped (one-off override, not propagated
into the skill).

---

## Self-review: what did I forget, what could be better?

Brutally honest, in order of severity:

1. **I almost shipped a leaky first fix.** The first version of the
   `Registry.delete` fix removed the registry entry without `list.deinit()`
   and the leak checker caught it. I caught it only because the flake
   discipline (leak-checked testing.allocator) is enforced by the test runner.
   The lesson stands: the first plausible fix is not the fix; run the suite
   before moving on. I did run it — but I should have *reasoned* about the
   ownership transfer before the compiler had to tell me.
2. **I did not check for a `Registry.Get`-shaped gap.** Go's registry exposes
   `Has`/`Get`/`Delete` (`Get` returns the runtime with its fibers); Rust has
   `Runtime` introspection. Zig still has only `size`/`has`/`delete` (+ my
   typed variants). That matches the ROADMAP parity row ("size / has /
   delete"), so it is not drift — but I added typed operations without
   surfacing that the *read* side of the registry is thinner than the other
   ports. Should have been mentioned in the final summary; it was not.
3. **I left a scratch repro file in the watched tree.** `zig/tests/repro.zig`
   existed on disk for several minutes while the auto-commit daemon and the
   flake gate both read this tree. It got deleted before any harm, but the
   safe pattern is `/tmp` for throwaway repros in this repo. Luck is not a
   process.
4. **My first `nix flake check` failure cost me a detour.** I treated the Rust
   clippy failure as potentially mine and started investigating toolchains
   before checking `git status`/`git log` for concurrent sessions. The AGENTS
   rule "trust the state you just ran, and respect changes you didn't author"
   applies to *failures* too: identify ownership first, debug second.
5. **The docs gate produces a 17 MB artifact nobody consumes.** `zig build
   docs` installs the full autodoc bundle (`index.html`, `main.wasm`,
   `sources.tar`) into `zig-out/` on every flake check run. Its purpose is
   validation, not publication. Works, but it is store/copy overhead on every
   check and the artifact is a ghost byproduct.
6. **Unexplained observation, unrecorded:** while verifying the embed gotchas,
   an *absolute-path* `@embedFile("/tmp/data.txt")` compiled fine in 0.16 —
   which contradicts the simple "cannot leave the module root" story. The
   AGENTS.md gotcha cites the verified *relative-path* failure ("embed of file
   outside package path"), so the recorded claim is correct, but the absolute
   case is an open oddity I chose not to chase (session scope) and did not
   document.
7. **No compile-refutation test for the `pluginView` guard.** The
   `@compileError` for non-TypedPlugin types is untested (Zig has no cheap
   in-suite compile-error test without extra build plumbing). Low value, but
   the gap is real and unstated until now.
8. **Mixed-concern commit landed.** The auto-commit daemon bundled my Zig
   registry work with the parallel session's Rust internal-event parity into
   `75fb408` ("ports: Rust internal-event parity and Zig typed registry +
   delete fix"). Not something I controlled — but I also did not commit my
   finished, verified work myself, which is what invited the bundling.
9. **Did I lie anywhere?** Checked: "docs emit cleanly" was backed by bundle
   inspection (sources.tar contains `cordis/cordis.zig`; decl data lives in
   the wasm which I could not plaintext-grep — I said so at the time);
   "ReleaseSafe 32/32" was re-confirmed with the summary line, not inferred
   from a truncated tail. No overclaims found.

---

## a) FULLY DONE

| Item | Evidence |
| --- | --- |
| `Registry.hasTyped(P)` / `deleteTyped(P)` keyed by `TypedPlugin` identity, with a `@compileError` guard (`pluginView`) for non-TypedPlugin types | `zig/src/cordis.zig`; committed in `75fb408`; covered by 3 rewritten/new tests in `zig/tests/typed.zig` |
| Root-cause fix: `Registry.delete` no longer iterates the live runtime list while disposing (stale-slice read of `0xAA`-poisoned slots crashed on plugins with ≥2 fibers). Now snapshot-ids → `deinit` list → drop registry entry → dispose, matching Go `Registry.Delete` / Rust `delete_id` order | Empirical repro confirmed the poison mechanism (`replaceRangeAssumeCapacity` fills vacated slots with `undefined`); CHANGELOG "Fixed" entry; regression test "deleteTyped disposes every fiber of the plugin type" |
| Zig suite green and leak-clean: 32/32 tests Debug **and** ReleaseSafe, zero leaks (baseline before session: 30/30) | `zig build test --summary all`, `zig build -Doptimize=ReleaseSafe test` |
| Docs emission gate: `zig build docs` step (0.16 replacement for the nonexistent `zig build -femit-docs` flag) via `Compile.getEmittedDocs()` + `addInstallDirectory`; bundle verified to contain the module source | `zig/build.zig`; 17 MB bundle in `zig-out/docs`; doc comments validated by full library compile + `zig fmt --check` + `ast-check`, all clean |
| Flake gate hardened: zig check now runs `zig fmt --check` and `zig build docs` in addition to tests | `flake.nix` zig check; verified passing inside `nix flake check` sandbox |
| AGENTS.md: new "Zig 0.16 std gotchas" section — five gotchas, each verified empirically against 0.16.0 before recording (`std.Io.Dir.cwd`, `ArrayList .empty` requirement, non-zig `@import` / relative `@embedFile` failures → WriteFiles+shim pattern, missing `-femit-docs` + `source_dir` rename, `orderedRemove` poisoning) | `AGENTS.md`; commit `75fb408`; verification transcripts in session (compile errors reproduced deliberately) |
| TODO_LIST.md: Zig section cleared (all three items done); CHANGELOG.md: Added + Fixed entries; AGENTS.md build section documents the docs gate | `75fb408`; markdownlint clean |
| `nix flake check`: **all checks passed** (go, rust, zig) on the final tree | Run twice after final state; second run rebuilt the zig derivation with the new gates |

## b) PARTIALLY DONE

| Item | Works | Open | Effort |
| --- | --- | --- | --- |
| Flake `zig-out` source filter (my last edit, uncommitted) | Filter line added and formatted; final flake check passed with it | Not yet committed as its own scoped change — sitting in a tree where the daemon can bundle it with the parallel session's work again | S |
| Typed registry API coverage | Typed has/delete, multi-fiber delete, dynamic-address delete all tested | Delete of a plugin with a *pending* (deps-unmet) fiber is untested; the `pluginView` compile-error guard is untested | S |
| `zig build docs` as a gate | Enforced in `checks.zig`; catches broken doc comments via full compile | Not mirrored in the `apps.test-zig` / `apps.test` quickstart apps, and not run in ReleaseSafe anywhere; artifact is throwaway 17 MB | S |

## c) NOT STARTED

- Zig registry runtime introspection (`Get`-equivalent returning fibers), if
  parity with Go/Rust is wanted — the ROADMAP row does not demand it, so this
  is a deliberate gap, not drift.
- Any Zig work on the remaining ROADMAP "-" cells (registry snapshot/restore,
  interception events, status events, config validation, logger) — untouched,
  as before this session.
- A multi-fiber `delete` scenario in the golden corpus — the corpus only ever
  deletes single-fiber plugins, which is exactly why the `Registry.delete`
  bug survived this long; changing the corpus means regenerating expected
  traces for all three runners (Go is the reference).

All other open work (Rust items, CI items, TS verification) predates this
session and was left alone per scope.

## d) TOTALLY FUCKED UP

Nothing in the Zig port is currently broken — final gates are green. The
genuinely fucked-up things observed this session are process-level:

1. **The auto-commit daemon created a mixed-concern commit.** `75fb408`
   contains two sessions' work (Rust internal-event parity + Zig registry/docs)
   in one commit. History now attributes two unrelated change-sets to one
   message; bisecting or reverting either half is painful. Severity: hurts
   archaeology, not correctness. Workaround: none retroactively; the report's
   section (e) proposes the fix.
2. **TODO_LIST drift created by the parallel session.** `TODO_LIST.md → Rust`
   still lists "internal/plugin + internal/update interception events" as
   open, but `75fb408` landed exactly that parity (`EVENT_UPDATE` etc. in
   `rust/src/fiber.rs`). The item is stale and will misdirect the next sweep.
   Severity: medium — docs-health HARVEST/VERIFY will false-positive on it.
   Mitigation: verify + check off in the next docs pass (listed in (f)).
3. **Concurrent-session collision surface.** The flake reads the *live*
   working tree, and two sessions were mutating it simultaneously; my first
   `nix flake check` failed on the other session's half-finished Rust code.
   Transient, self-healed (their commit completed, next check green), but the
   repo has no guard against gating on someone else's in-flight state.
   Severity: low-frequency, high-confusion.

## e) WHAT WE SHOULD IMPROVE

1. **Commit finished work myself, immediately, scoped.** In this repo the
   daemon will commit anything left in the tree, bundled with whatever else is
   in flight. Finished + verified → commit it scoped, same hour. (Direct root
   cause of (d)1.)
2. **Scratch repros live in `/tmp`, never in the watched tree.** The tree is
   read by the daemon, the flake gate, and buildflow; a broken scratch file is
   one daemon tick away from a bad commit.
3. **Gate ownership triage before debugging.** When `nix flake check` fails,
   first `git status` + `git log` to attribute the failure to a session;
   only then debug. Saves the toolchain-drift detour I took.
4. **Validation gates should not build artifacts nobody consumes.** The docs
   gate could emit into a throwaway path (or `zig build docs --prefix
   "$TMPDIR"`) so checks stop copying 17 MB per run.
5. **Reason about ownership before fixing, not after.** The leak after my
   first `delete` fix was avoidable by stating the ownership transfer (who
   deinits the list) before writing the patch.
6. **Recurring observation across reports → skill/tooling.** "Parallel
   sessions + live-tree flake gate" has now bitten twice in different forms
   (untracked-file invisibility documented; in-flight edits undocumented). A
   short AGENTS.md extension or a pre-check helper would close it.

## f) Next tasks (brainstorm — HARVEST fuel, not commitments)

Impact / Effort (S <30 min, M 30 min–2 h, L >2 h) / Category:

| # | Task | Impact | Effort | Category |
| --- | --- | --- | --- | --- |
| ~~1~~ | ~~Commit the flake `zig-out` filter line as its own scoped commit~~ done at `ff84d10` | Medium | S | Cleanup |
| ~~2~~ | ~~Verify Rust `internal/plugin` + `internal/update` interception parity landed in `75fb408`, then check off the stale TODO_LIST item~~ done at `25ff5fb` (verified and recorded in CHANGELOG/FEATURES; TODO_LIST cleared) | High | S | Documentation |
| 3 | Add a golden-corpus `delete` scenario that removes a multi-fiber plugin (regenerate expected traces for all three runners) | High | M | Quality |
| 4 | Zig test: delete a plugin whose fiber is `pending` (deps unmet) and assert rollback/no-crash | Medium | S | Quality |
| 5 | Zig test: compile-refutation coverage for the `pluginView` `@compileError` guard | Low | S | Quality |
| 6 | Point the docs gate at a throwaway prefix so checks stop producing a 17 MB `zig-out/docs` | Medium | S | Cleanup |
| 7 | Mirror `zig fmt --check` + `zig build docs` into the `apps.test-zig` / `apps.test` quickstart so app and check gate the same things | Medium | S | Quality |
| 8 | Pin down the absolute-path `@embedFile` behavior in 0.16 and correct/extend the AGENTS.md embed gotcha if needed | Low | S | Documentation |
| 9 | Decide and document whether Zig needs a registry `Get`/`Runtime` introspection API (parity gap vs deliberate) | Medium | S | Decision |
| 10 | Add ReleaseSafe (or ReleaseFast) to one automated gate — a `.#checks` variant or a periodic app — ReleaseSafe is currently run only ad hoc | Medium | S | Quality |
| 11 | Extend AGENTS.md repo-hygiene: record the "flake gates the live tree; concurrent sessions can fail each other's checks" hazard explicitly | Medium | S | Documentation |
| ~~12~~ | ~~CI job running `nix flake check` so the port gate is enforced remotely (pre-existing TODO_LIST item)~~ done at `fa45896` | High | M | Quality |
| ~~13~~ | ~~Rust: verify root-fiber status emission (`FiberData::new_root` bypasses `settle_state`) — cover or document (pre-existing TODO_LIST item)~~ done at `25ff5fb` | Medium | S | Quality |
| ~~14~~ | ~~Rust: fix or allowlist `significant_drop` nursery findings under `--features thread-safe`, then gate clippy on it (pre-existing)~~ done at `25ff5fb` | Medium | M | Quality |
| ~~15~~ | ~~Rust: `cargo-llvm-cov` coverage baseline next to the Go numbers (pre-existing)~~ done at `25ff5fb` | Medium | M | Quality |
| ~~16~~ | ~~Rust: `cargo bench` to substantiate or hedge the "up to 30% faster" ROADMAP claim (pre-existing)~~ done at `25ff5fb` | Low | M | Quality |
| ~~17~~ | ~~CI: add `.prettierrc` (printWidth 100) + `prettier --check` + `yarn build` before tests in build.yml (pre-existing)~~ **Won't implement — no prettier-stable style exists to pin; upstream style CI-enforced (`fa45896`)** | Medium | S | Quality |
| ~~18~~ | ~~CI guards: `packages/**` byte-identical to upstream; `dprint.json` excludes keep covering `packages/**` (pre-existing)~~ done at `fa45896` | Medium | S | Quality |
| ~~19~~ | ~~After next push: verify build.yml + ports.yml green on GitHub, including the replayed `3-stage-hmr` line (user-gated, pre-existing)~~ done (Build + Ports green on `3da7d0f`) | High | S | Quality |
| ~~20~~ | ~~Gitignore `tmp-*` test debris (pre-existing)~~ done at `fa45896` | Low | S | Cleanup |
| ~~21~~ | ~~TS: one full install-from-scratch verification (`rm -rf node_modules && yarn install && yarn build && yarn test`) (pre-existing)~~ done at `fa45896` (248/248) | Medium | M | Quality |
| ~~22~~ | ~~CONTRIBUTING.md review: flake app list in quickstart + document the "upstream semantics + fork formatting" rebase policy (pre-existing)~~ done at `fa45896` | Medium | M | Documentation |
| ~~23~~ | ~~Loader: fuzz the JSON config layer (EncodeConfig/DecodeConfig roundtrip) (pre-existing)~~ done at `fa45896` | Medium | L | Quality |
| ~~24~~ | ~~Align local gate with CI race canary: flake checks `-count=1` vs ports.yml `-count=3` (pre-existing)~~ done at `fa45896` | Low | S | Quality |
| 25 | Document/decide daemon commit scoping (per-concern commits or pre-staging) to stop mixed-concern history (root cause of (d)1) | High | M | Process |
| 26 | Zig: consider a `Registry` doc example showing the typed form in the module doc comment (docs currently show only `TypedPlugin.start`) | Low | S | Documentation |
| 27 | Zig: exercise `Registry.delete` through `Context.registry()` from a non-root context to pin scope semantics (works by construction, untested) | Low | S | Quality |
| 28 | buildflow: fix nix-step system filtering upstream (pre-existing known limitation; belongs in buildflow, not here) | Medium | L | Bug |
| 29 | Re-run the three-runner golden parity after any future semantics change — reminder cadence, not new work (pre-existing convention) | Medium | S | Process |
| ~~30~~ | ~~docs-health HARVEST this report's (f) items into TODO_LIST/ROADMAP after instructions arrive~~ done (second docs-health pass, 2026-09-08) | High | S | Documentation |

Items 12–24, 28 are pre-existing TODO_LIST/known-limitation items (listed
because the user asked for the full next-up picture); 1–11, 25–27, 29–30 come
directly out of this session. 30 is the closing move of the skill loop.

## g) Questions I cannot answer myself

1. **Golden corpus policy:** the `delete` bug survived because the shared
   golden scenarios only ever delete single-fiber plugins. Do you want a
   multi-fiber `delete` scenario added to the corpus (DSL + regenerated
   expected traces pinned across Go/Rust/Zig), or should the corpus stay
   frozen at the four upstream-anchored scenarios and this stay a unit test
   concern?
2. **Daemon scoping:** commit `75fb408` bundles my Zig work with the parallel
   session's Rust parity work. Do you want the auto-commit daemon scoped
   (per-concern commits, or only commit when one session is active), or is
   bundled history acceptable as long as reports document it?
3. **Zig registry read side:** Go exposes `Registry.Get` → runtime/fibers and
   Rust exposes `Runtime` introspection; Zig has only size/has/delete. Is the
   thinner read side a deliberate native-minimal choice to keep in ROADMAP's
   "-" column, or should Zig grow a `Runtime`-style accessor next?

---

**Handoff:** section (f) is HARVEST input for TODO_LIST/ROADMAP — not executed
yet, per "wait for instructions". The only uncommitted change from this
session is the one-line flake `zig-out` filter (item 1).

---

## Resolution (annotated 2026-09-08, second docs-health pass)

Rows 1 (`ff84d10`), 2 (`25ff5fb`), 12–24 (`fa45896`, `25ff5fb`, green CI
on `3da7d0f`) and 30 carry inline verdicts above. Still open: the Zig
test/gate gaps (3–7, 10), the `@embedFile` absolute-path oddity (8), the
registry `Get` decision (9), the live-tree hazard note (11 — partially
covered by AGENTS.md's untracked-files gotcha), daemon commit scoping
(25), the Zig doc example (26), `registry()` delete test (27), the
buildflow upstream fix (28) and the golden re-run cadence (29). Items
3, 4, 5, 6 and 7 are routed to `TODO_LIST.md`.
