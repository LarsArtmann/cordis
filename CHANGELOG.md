# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Releases are cut as port tags (`go/v*`, `rust/v*`); the TypeScript packages
in `packages/` track upstream and are not released from this fork.

## [Unreleased]

### Added

- Multi-language ports of the cordis core — Go (flagship), Rust and Zig —
  with semantics parity (fiber lifecycle, drain queue, isolation realms,
  LIFO rollback) and native, type-keyed APIs per language (from
  2026-08-22).
- Go ecosystem packages: `timer` (synctest-tested), `group`, `loader`
  (config-driven start, watch/reload with rollback) and `hmr` (module swap
  with all-or-nothing rollback).
- Go service extensions: accessor/mixin derived services, callable services
  with tracker attribution, `Fiber.Err`, cancellable await, typed inject
  sugar (`Inject1/2/3`).
- Rust: opt-in `thread-safe` feature (Mutex core, stress-tested), registry
  snapshot/restore, `internal/status` events, intercept and config
  validation.
- Zig: all five dispatch modes, batch transactions, effect scopes with
  introspection, disposers, registry view.
- Cross-language assurance: three golden scenarios executed byte-identically
  by the Go, Rust and Zig runners, `nix flake check` derivations, Go
  benchmarks, a randomized LIFO-disposal property test and the
  `scripts/parity-matrix.sh` navigator.
- Go 1.27 adoption: `testing/synctest` virtual-clock timer tests,
  `strings.CutLast` in plugin name derivation.

### Changed

- Rebased onto upstream `caab04e` (2026-09-08): inherited the three-stage
  reload (#111), include journal reconciliation (#121), bare-specifier
  resolution (#123) and `hmr.watch()` (#128), plus the upstream
  `3-stage-hmr` fix branch replayed on top — the loader now commits
  `EntryChange` records instead of rewriting config files, and include
  writes are atomic.
- Root `README.md` is now a fork-owned sales page;
  `packages/core/README.md` is byte-identical to upstream again.

### Fixed

- Thread-safe Rust: `Fiber::name` self-deadlock (nested core locks) that
  hung CI Ports runs for over an hour; lock-order invariant documented.
- Go loader: `PollWatcher` captured its baseline at the first poll, so a
  config written right after `Serve` was never detected; the baseline is
  now primed in the constructor.
- hmr test fixtures restored byte-identical to upstream after a formatting
  pass silently broke the specs' literal string replaces (every reload
  test timed out).

### Releases

- `go/v0.1.0` (2026-09-05): first Go module tag — core plus ecosystem.
- `rust/v0.2.0` (2026-09-05): registry snapshot/restore and status events.
