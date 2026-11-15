# hmr rc.10→rc.13 divergence memo (T18)

Deep-read of the upstream hmr rework absorbed by the rc.13 sync
(pin `f8ea3cd` → `68c9d99`) against the Go `go/hmr` port, confirming
decision **D3 (defer the re-port)** with evidence. Companion to the
program plan (`2026-11-14_16-41_upstream-rc13-sync-port-parity-program.md`)
and the ROADMAP Open-decisions record.

## What changed upstream

| Commit | Change | Essence |
| --- | --- | --- |
| `5b8ad37` #244 | split hmr into a programmatic core and an fs watcher | core (`reconcile`/resolution/module caches in `src/runtime/`) separated from `hmr.watch()` fs driving |
| `bf5af66` #263 | per-group fixture copies, clear barrier timers | test-infrastructure only |
| `31bb050` #264 | stop ignoring `cache`/`data` by default | `hmr.watch()` default `ignored` is now `['**/node_modules', '**/.*']` |
| `5fc4328` #269 | module graph and per-unit atomicity | reload attempts partition into units = connected components of stale plugins across old+new ESM graphs; a failing unit aborts only its unit |
| `d8b7ea1` #282 | reconcile roots resolved by a callback | `reconcile(cb: () => Awaitable<Iterable<string>>)`, AsyncLocalStorage attribution for nested reconciles |
| `b38ef58` #298 | invalidate resolution caches and package manifests | swap evicts module-loader cache entries and re-reads `package.json` manifests on reconcile |

## Go port today (`go/hmr`, 216 lines)

Programmatic core only, by documented design: Go is AOT, so a "changed
file" is a changed factory value (`Swap`), the dependency graph is
declared (`Declare`), the accept set is the fixed point of that graph,
a failed swap rolls back the registration, and generations number
applied swaps. There is no fs watcher, no watch config, and no
resolution cache; observing the world is the embedder's job.

## Divergence assessment (why defer is right)

- **#244 (split): already the port's shape.** The port *is* the
  programmatic core; the fs watcher half is deliberately out of scope.
  Nothing to adopt.
- **#264 (ignore defaults): no Go surface.** The port has no
  `hmr.watch()` config. The loader's watch-golden trace
  (`go/loader/watch_golden_test.go`) pins loader reload semantics and
  stays green untouched — verified in the T17 sweep.
- **#269 (per-unit atomicity): real granularity divergence, low
  urgency.** Upstream batches connected components so independent
  subtrees survive a sibling failure. In the Go API one `Swap` = one
  module; every relinked entry shares that module, so a Go swap is
  exactly one unit — whole-swap rollback *is* per-unit atomicity at
  this granularity. The upstream behavior only differs for a
  multi-module reconcile in one call, a surface the port does not
  expose. If ever needed, add `SwapBatch` (grouped rollback per
  connected component of the declared graph) as a follow-up; the
  current semantics remain pinned by `hmr_test.go`.
- **#282 (reconcile callback): API-shape divergence, by design.** The
  embedder already computes what changed and calls `Swap` per module;
  a callback indirection adds nothing without an fs layer.
- **#298 (cache/manifest invalidation): N/A.** The Go `Resolver` is a
  name→registration map with in-place replacement (`Replace`) for
  swaps; there is no resolution cache or manifest re-read to
  invalidate.

## Verdict

D3 confirmed: the rework's substance (real ESM graphs, resolution
caches, package manifests, per-unit batching across multi-module
reconciles) is inseparable from a runtime module loader, which an AOT
port does not have. The port's swap/generation/accept-set model covers
the semantics that transfer, and its divergences are pinned by tests.
Re-evaluate only if the fork grows a multi-module batch swap
(`SwapBatch`) or an embedder asks for reconcile-callback semantics.

Decided 2026-11-15 during the rc.13 sync execution session
(`docs/status/2026-11-15_08-18_rc13-sync-execution-session.md`).
