# Status Report: Upstream rc.10 → rc.13 Assessment (read-only session)

**Date:** 2026-11-14 16:31 CET
**Session type:** Read-only investigation — no repo files were modified this session.

## Scope line — what this report VERIFIED vs did NOT

**Verified (with commands/artifacts):**

- `git fetch upstream`: pin `f8ea3cd` (rc.10) → `68c9d99` (rc.13); new tags `v4.0.0-rc.11/12/13`; new upstream branches `perf/traceable-underscore`, `refactor/migrate-server`.
- `git log --oneline f8ea3cd..upstream/main`: **77 commits**, `git diff --stat`: **224 files, +21,508 / −3,124** in `packages/`.
- Full diff reads: #242 (events.ts, complete), #209 (reflect.ts/service.ts, complete), #133 (fiber.ts, complete), #196 (fiber.ts, partial), plus root `package.json` (upstream/main) and package-list diffs (`git ls-tree` at pin vs main).
- Stat/summary-level reads: #266, #307, #319, #326, #337, #296, #134, #269, #244 (via `--stat` and log subjects).
- **Port gap for #242 verified in source:** Go `resolveHooks` snapshots `*hook` pointers, `hook` struct has no removed flag (`go/events.go:36-40`, `go/events.go:251-269`); Rust `resolve_hooks` returns `Vec<Rc<Hook>>` (`rust/src/events.rs:289`); Zig snapshots `Hook` values (`zig/src/cordis.zig:1354-1370`). All three will call a listener that an earlier listener disposed mid-dispatch.

**NOT verified (and why):**

- No port code was executed; no test suite was run (assessment only, user asked "worth considering?").
- #209 / #196 / #133 / #191 / #56 / #134 were **not** checked against port sources — flagged "needs parity test" by upstream-diff reasoning only.
- The hmr rework (#244, #269, #282, #298) and loader fixes (#329, #271, #267, #239, #210, #246, #188, #245, #241) were read at commit-subject/stat level only, not diff level.
- `scripts/manifest-parity.mjs` and `.github/workflows` were not opened; claims about their staleness are inferred from AGENTS.md descriptions + observed upstream deltas.
- Whether upstream-parity CI (byte + prettier-normalizable guards) survives the sync was not simulated.

---

## a) FULLY DONE

1. **Upstream fetched and delta quantified:** rc.10 → rc.13, 77 commits, +21.5k lines, 224 files. Two new upstream branches and three release tags catalogued.
2. **Port-relevant semantic fixes identified and triaged** into: verified gap (#242), likely gaps needing parity tests (#209, #196, #133, #191, #56, #134, #205, #136), features to evaluate (#266 registry.replace, #296 unshift, #206 intercept merge, #277/#280 perf).
3. **#242 mid-dispatch listener removal verified as a real gap in all three ports** with exact file:line evidence (see scope line).
4. **Fork policy invalidated facts identified:** upstream now tracks `yarn.lock` (#319); upstream replaced eslint with oxlint + `oxlint.config.ts` (#326); upstream `@types/node` is now `^25.3.5` (fork's deliberate delta is `^26.5.0`); root manifest churn (yakumo `^3.4.2`, `tsx` → `npm:@cordisjs/tsx`, `semver`/`shx` added, workspaces gained `external/*`).
5. **Sync scope mapped:** 8 new upstream packages (`cli`, `cli-cordis`, `env`, `fs`, `fs-node`, `http`, `http-socks`, `package-manager`), `utils` removed; at the pin upstream had exactly the fork's 9 packages.
6. **Breaking rename surfaced:** #307 renames plugin → component across core/hmr/group APIs, tests, fixtures — a fork-level naming decision, not a mechanical sync.
7. **Delivered the assessment** to the user with a two-step recommendation (sync, then port #242 + parity tests) and asked whether to start. Awaiting answer.

## b) PARTIALLY DONE

1. **The upstream assessment itself** — core semantics read at full-diff level; hmr rework and loader fixes only at subject/stat level. Enough for triage, not enough to port from.
2. **Port-gap verification** — only #242 checked against port source. The other six core fixes are reasoned flags, not verified gaps.
3. **Fork-mechanics impact analysis** — stale-fact identification is complete; concrete updates to `.buildflow.yml`, `scripts/manifest-parity.mjs`, `scripts/hmr-fixture-canary.mjs`, CI workflows are analyzed by inference only, files not opened.

## c) NOT STARTED

1. The actual `packages/` sync + `.github/UPSTREAM_PIN` bump to `68c9d99`.
2. Porting #242 to Go, Rust, or Zig.
3. Parity tests for #209 / #196 / #133 / #191 / #56 / #134.
4. The plugin → component naming decision and any resulting doc churn (DOMAIN_LANGUAGE.md, ROADMAP.md, PORTS.md).
5. yarn.lock adoption decision.
6. `.buildflow.yml` `eslint-fix` skip-rationale rewrite (oxlint era).
7. `manifest-parity.mjs` allowlist update (eslint out; oxlint/semver/shx/@cordisjs/tsx in; @types/node delta re-decision).
8. `hmr-fixture-canary.mjs` compatibility check (the rename renamed fixtures `plugin*.ts` → `component*.ts`; spec replace literals may have changed).
9. hmr re-port evaluation (#244 programmatic core + #269 module graph vs current swap/generation/accept-set model).
10. `registry.replace()` port evaluation for the Go Registry.
11. TS suite + two-step clean build + `nix flake check` after a sync (nothing synced yet).
12. AGENTS.md stale-fact updates (yarn.lock, eslint, pin, @types/node) — deferred to the sync commit, which hasn't happened.
13. HARVEST of this report's section (f) into `TODO_LIST.md` / `ROADMAP.md` — explicitly deferred: the user ordered WAIT FOR INSTRUCTIONS, and three policy questions (section g) gate several items.

## d) TOTALLY FUCKED UP

Nothing destructive — the session was strictly read-only; zero files modified, zero tests run, zero commits made. One owned text defect in the final chat message: the new-package list read "`cli`, `fs`, `fs`, `fs-node`, `http`, `http-socks`, `package-manager`, `env`" — duplicated `fs` and buried `cli-cordis`. The correct list (8): `cli`, `cli-cordis`, `env`, `fs`, `fs-node`, `http`, `http-socks`, `package-manager`.

## e) WHAT WE SHOULD IMPROVE

1. **AGENTS.md already contains a now-false fact** ("Upstream commits no yarn.lock" / "Never commit yarn.lock"). It became false the moment upstream merged #319. Should be corrected in the sync commit at the latest — or now, since a fresh session could be misled into deleting a lock upstream deliberately ships.
2. I flagged #209/#196/#133/etc. as "needs parity test" without opening the port code — a 10-minute source read per fix would have upgraded flags into verified gaps/confirmed-parity and made the recommendation sharper.
3. The #242 gap claim rests on source reading, not a failing test. A tiny reproduction test per port would have converted "verified by eyeball" into "verified by execution."
4. Did not simulate the upstream-parity guards against the delta (prettier-normalizable check over +21.5k lines, byte guard over deleted `utils/` and renamed fixtures). The sync's riskiest unknown is unexamined.
5. Did not look at the two new upstream branches (`perf/traceable-underscore`, `refactor/migrate-server`) — possible early signals of the _next_ delta.
6. The final message asked "Want me to start?" — correct per the assessment-only ask, but the report now bakes the decision points into 3 crisp questions (section g) instead of one vague one.

## f) Things to get done next (up to 50, grouped, impact-ordered)

**Sync mechanics (gates everything):**

1. Decide yarn.lock policy: adopt upstream's tracked lock (hermetic installs) vs keep floating — question g1.
2. Sync `packages/` to `68c9d99` and bump `.github/UPSTREAM_PIN` in one commit (per AGENTS.md protocol).
3. Update `scripts/manifest-parity.mjs` allowlist: remove eslint, add `oxlint`/`semver`/`shx`/`tsx→@cordisjs/tsx`, re-decide `@types/node` (`^25.3.5` upstream vs `^26.5.0` fork delta).
4. Verify `scripts/hmr-fixture-canary.mjs` still matches: fixtures were renamed (`plugin-fast.ts` → `component-fast.ts`, etc.) and specs rewritten.
5. Verify the upstream-parity byte guard handles: deleted `packages/utils`, 8 new packages, renamed spec/fixture files, new `oxlint.config.ts`, tracked `yarn.lock`.
6. Re-check the prettier CI pin (3.9.6) against the rename-rewritten TS files.
7. Update `.buildflow.yml` `eslint-fix` skip rationale for the oxlint era (possibly re-point to the existing oxlint step).
8. Update AGENTS.md stale facts in the sync commit: yarn.lock policy, eslint→oxlint, new pin, @types/node decision, new-package inventory.
9. Clean-tree two-step build verification (`trash packages/*/lib` + `tsconfig.tsbuildinfo`, then `yarn build core && yarn build`).
10. Run the full TS suite (baseline was 248/248 — new count expected with new packages).

**Port semantics — verified/near-verified bugs first:**

11. Port #242 to Go: `removed` flag on `hook`, honored in `Emit`/`Parallel`/`Serial`/`Bail`/`Waterfall` dispatch loops + unit tests (Go is flagship).
12. Port #242 to Rust (`events.rs` dispatch paths) + tests.
13. Port #242 to Zig (both snapshot loops, `cordis.zig:1354` + `:1367`) + tests.
14. Parity test: service lookup stops at re-isolation boundaries (#209) — replicate upstream `isolate.spec`/`shadow.spec` cases in all three ports; fix where they fail.
15. Parity test: no injected metadata for a fiber's own provisions (#209 second half).
16. Parity test: providers disposed only after dependents unloaded (#196) — cross-fiber unload ordering.
17. Parity test: disposed fiber settles to `StateDisposed` even when disposed mid-unload (#133).
18. Parity test: service impl unregistered after an isolate move (#191).
19. Parity test: `internal/update` hooks disposed with the fiber (#56).
20. Decide and implement #134 (config re-resolved on every reload) in the Go port — current hmr swap preserves entry config by design; upstream semantics changed.
21. Parallel dispatch error should name the failing event (#205) and the mode (#136) — check all three ports' error text.
22. Evaluate `registry.replace()` (#266) for the Go Registry (Delete exists; upstream hmr is now built on replace).
23. Evaluate Rust/Zig equivalents for in-place replacement.
24. Perf: notify only the services a fiber provides (#277) — audit ports' notify-on-unload pattern.
25. Perf: defer access-error stack construction (#280) — likely N/A for Go (`errors` are cheap) but confirm Zig/Rust.
26. `DisposableList.unshift()` (#296) — Go has a prepend path; confirm Rust/Zig parity.
27. Intercept metadata merge order (#206) — parity test for intercept precedence.

**hmr (Go-only port; upstream reworked the model):**

28. Read #244 (programmatic core / fs-watcher split) and #269 (module graph, per-unit atomicity) in full.
29. Decide: re-port Go hmr onto the new upstream core vs keep the swap/generation/accept-set model pinned by `go/loader/watch-golden_test.go` — question g3.
30. Check #264 (stop ignoring cache/data by default) against the Go port's defaults.
31. Check #290/#4ca7e2e (`watch()` returns `Watcher`) against Go `hmr.watch` surface.
32. Map #282 (reconcile roots via callback) and #298 (resolution-cache + package-manifest invalidation) onto the Go port's swap behavior.

**Loader (Go port):**

33. Review #329 (isolation boundaries through isolate maps) vs Go loader realm handling.
34. Review #271 (entry transfer on cross-tree moves), #267 (empty-string isolate label), #518b8bc (entry-id tree-separator rejection), #172-era leftovers for port applicability.
35. Review #239/#210/#246 (entry-option validation + source-location tracing) for Go loader error UX parity.
36. Review #34c8ac5 (`cordis:` builtin specifier removal) and #1d9ad95 (`!!js` in `disabled`) for Go loader parity.

**Docs / fork policy:**

37. plugin → component naming decision across ports and docs — question g2.
38. Update `docs/DOMAIN_LANGUAGE.md` post-decision (rename note or full adoption).
39. Update `ROADMAP.md` parity matrix: new upstream packages (fs/http/cli/package-manager) as unported columns; new core features (replace, unshift) as parity rows.
40. Update `PORTS.md` if any port semantics move.
41. Adopt #337 (type-check tests in CI) in the fork's CI — directly addresses the "vitest never typechecks" TS 7 lesson.
42. Verify `internal/plugin` event name survived the component rename upstream (#307 touched `events.ts`) — ports emit `internal/plugin`; confirm upstream contract unchanged for cross-language listeners.
43. Check whether the golden scenarios need a new op covering mid-dispatch disposal once #242 lands (or keep it unit-test-only).

**Verification sweep (after the above lands):**

44. `nix run .#test` (all languages) + `nix flake check` + `nix develop -c buildflow`.
45. Re-baseline coverage numbers (Go 91.4%, Rust 86.7%) in AGENTS.md if the port fixes move them.
46. Re-run `scripts/vulnix-audit.sh direct` if the sync changes the TS toolchain closure inputs.
47. Watch upstream `perf/traceable-underscore` and `refactor/migrate-server` branches for the next delta.
48. After decisions g1–g3: HARVEST this report's section (f) into `TODO_LIST.md`/`ROADMAP.md` per docs-health (deliberately not done now — waiting on instructions + answers).

## g) Questions I cannot figure out myself

1. **plugin → component (#307):** keep `Plugin` naming across the Go/Rust/Zig ports and document the divergence, or adopt upstream's `component` rename (churn: every port API, DOMAIN_LANGUAGE/ROADMAP/PORTS, golden op names)? This is a branding/taste call with real migration cost either way.
2. **yarn.lock (#319):** adopt upstream's now-tracked lock for hermetic installs (and make it a byte-parity-guarded file), or keep the fork's floating no-lock protocol that upstream itself just abandoned?
3. **hmr re-port scope:** re-base the Go hmr port onto upstream's reworked programmatic-core hmr (#244/#269), or is the current swap/generation/accept-set model — pinned by its own Go-only golden trace — the port's deliberate steady state? This is an effort-vs-parity-value judgment only you can make.

---

_Report ends. Session was read-only; awaiting instructions on a) sync start, b) #242 porting, c) the three questions above._
