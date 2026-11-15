# registry.replace evaluation memo (T19)

Evaluation of upstream `registry.replace()` (#266, commit `74b7905`,
absorbed by the rc.13 sync) against the Go port, per the program plan's
T19 ("evaluation + Go prototype/rejection memo").

## What upstream added

`RegistryService.replace(oldPlugin, newPlugin)`:

- aliases the old plugin key to the new one (`_aliases` WeakMap, so
  later lookups through the old reference forward to the replacement),
- moves every fiber of the old runtime onto the new plugin's runtime
  via `fiber._replace(target, newInjects)`, which preserves the fiber's
  identity (uid, config, lifetime ownership), swaps its runtime and
  inject set, clears its provided-service store, bumps its revision and
  refreshes — reloading in place without a dispose window,
- emits a new `internal/replace` event per fiber,
- keeps the old plugin's fibers list empty and unregisters the old
  runtime only when the last fiber moves.

It is the substrate the rc.13 hmr core is built on: a module swap is a
registry replace, so entries keep their fibers across implementation
changes.

## The Go port's model

`Registry` (go/plugin.go) exposes Size/Has/Get/Delete; Delete disposes
every fiber of a runtime, restoring the pre-start state. The Go hmr
port relinks differently by documented design: a swap disposes the old
fiber and starts a fresh one with the entry's config and identity
preserved (PORTS.md; DOMAIN_LANGUAGE: "module identity survives swaps
while fiber instances do not"). The observable outcome — the entry
keeps running on the new implementation, config intact — is the same;
what differs is fiber identity continuity and the dispose window
(unload events fire, siblings briefly see the entry gone).

## Verdict: reject for now, documented divergence

- **No port consumer needs fiber identity across a swap.** The Go hmr
  relink model is pinned by `go/hmr` tests and the loader watch-golden
  trace; nothing in the Go/Rust/Zig surfaces asks whether a fiber
  survived an implementation change.
- **The cost is not the method but the semantics.** A faithful
  `Replace` needs fiber migration between runtimes, in-place inject
  re-resolution, the `internal/replace` event, alias forwarding on
  every Has/Get — and then the hmr port would face a second swap
  substrate (replace vs relink) with different observable event
  sequences, unpinned by any current test.
- **The fresh-fiber divergence is already documented** (PORTS.md,
  ROADMAP parity matrix lists `replace` as not ported, pointer here).

Revisit triggers: an embedder needs fiber-identity-preserving swaps;
the D3 hmr re-evaluation (`2026-11-15_hmr-rc13-divergence-memo.md`)
fires and a re-port adopts the rc.13 core wholesale; or upstream
builds loader features on replace that the Go loader must mirror.
Rust and Zig: same conclusion — snapshot/restore (Rust) and the Zig
registry cover their surfaces; note as non-ported in the matrix.

Decided 2026-11-15 during the rc.13 sync execution session
(`docs/status/2026-11-15_08-18_rc13-sync-execution-session.md`).
