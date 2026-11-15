# Upstream issue drafts (T27: F106, F107)

Ready-to-file drafts, NOT filed: submission is user-gated. Both concern
the cordiverse/cordis TS workspace and carry session-verified evidence.
Filing flow when approved: verify-before-filing (claims below were
reproduced in this tree at pin 68c9d99), github-voice register.

## Draft 1 — clean single-step `yarn build` fails; two-step works (F106)

**Title:** `yarn build` from a clean checkout fails with ~35 dts errors
(TS2665/TS2307 in hmr + include); `yarn build core && yarn build` succeeds

**Body (evidence-heavy register):**

- Reproduced: from `git clean -xfd` + `yarn install`, a single `yarn
  build` deterministically reports ~35 errors in `packages/hmr` and
  `packages/include` dts output — the cross-package
  `declare module 'cordis'` augmentation no longer resolves
  (`TS2665: Module 'cordis' resolves to an untyped module`).
- `yarn build core` followed by `yarn build` is green, as is any build
  after a prior successful one. Reproduced on node 24 and 26 with
  identical dependency sets (yarn 4.14.1, TS 5.9.3, yakumo current).
- Suspected mechanism: yakumo tsc builds all projects in one pass; on a
  clean tree `packages/hmr`/`packages/include` compile their dts against
  `cordis` resolving through node_modules to `packages/core/lib` — which
  the same invocation has not written yet. The augmentation then hits an
  untyped module. Building core first materializes `lib/index.d.ts` and
  the augmentation resolves.
- Minimal reproducer sketch: a two-package yarn workspace where package
  B `declare module 'A'`-augments package A and both are built by one
  `tsc -b` invocation over clean output dirs.
- Impact: every fresh checkout/CI cache miss pays a retry; the failure
  mode ("untyped module") points readers at their own config, not the
  build order.

## Draft 2 — TypeScript 7 breaks the dts build with TS2665 (F107)

**Title:** TS 7.0.2: cross-package `declare module 'cordis'` augmentation
fails the dts build (TS2665) while vitest stays green

**Body (evidence-heavy register):**

- With `typescript ^7.0.2` (everything else at the rc.13 pins), tests
  compile and run (vitest never typechecks), but `yarn build` dts output
  dies with `TS2665: Module 'cordis' resolves to an untyped module at
  lib/index.js` across `packages/hmr` and `packages/include`.
- Root observation: under TS 7's resolver, the ambient
  `declare module 'cordis'` augmentation in hmr/include does not bind to
  the workspace package when resolution runs through the built
  `lib/index.js` (JS entry without types that tsc can associate).
- Reproduction: bump only `typescript` in the root manifest, install
  from scratch, run the two-step build. Green again after returning to
  TS 5.9.3.
- Note: rc.13 still pins TS 5.9.3; this blocks any TS 7 migration of the
  workspace and may be worth a compiler-team minimal reproducer
  (workspace + augmentation + `moduleResolution` as configured here).

## Disposition

Both drafts verified against this fork's tree (the two-step build is
also wired into this fork's CI as the workaround). File only on user
approval; re-verify the reproduction on the then-current upstream pin
first (verify-before-filing).
