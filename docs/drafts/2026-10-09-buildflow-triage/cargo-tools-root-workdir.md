TL;DR: every cargo provider triggers on any-depth `**/Cargo.toml` but executes `cargo <cmd>` at the project root. `cargo` resolves manifests by walking **up** from cwd, never down — so any repo with the crate in a subdirectory fails every cargo step with "could not find Cargo.toml". Ask: discover the manifest directory (or fan out per Cargo.toml) the way the Go tools already do.

## Problem

Observed on cordis (Go + Rust ports in one repo, crate at `rust/`), buildflow `1b99ae2`, 2026-10-09:

- `cargo-update`: `error: could not find`Cargo.toml`in`/home/lars/forks/cordis`or any parent directory` — 6 consecutive identical failures (buildflow's own loop detector flagged it).
- Same root-run failure for `cargo-fmt`, `cargo-clippy-fix`, `cargo-check`, `cargo-doc`.

## Root cause

`tools/providers/rust_tools.go`:

- `rustTrigger` (L21-27) — `Files: []file.FilePattern{cargoManifestPattern}` where `cargoManifestPattern = "**/Cargo.toml"` (L18): matches crates at any depth, so the tools activate in subdir-crate repos.
- All providers (`newCargoDetector` L40, `NewCargoUpdateProvider` L102, ...) run via `simpleDetector`/`blindRepairer` with no WorkDir handling — per AGENTS.md, WorkDir defaults to project root when there's no module fan-out, and cargo has no fan-out.

This is the cargo sibling of gotcha #55 (Go tools running `go ./...` at the root of a workspace with only sub-module go.mods). Difference: Go tools silently reported "all clean"; cargo fails loudly — so the user gets a permanently red pipeline and the only escape is `skip_steps`, which loses the checks entirely (that is what cordis ships today).

## Suggested fix

Either:

1. Manifest discovery: walk down from root for the nearest `Cargo.toml` (mirror `findGoModDirs` in the oxlint/gomod discovery pattern) and set the spawn dir — `--manifest-path` is NOT equivalent for all subcommands (workspace-relative output paths differ), so prefer running _in_ the discovered dir; or
2. A rust analogue of Go's `ModuleFanOut` / `moduleScopedGoToolSpecs`: one execution per Cargo.toml, WorkDir set per manifest.

Option 1 is probably right for single-crate-per-dir repos; option 2 for multi-crate workspaces. Would love `skip_steps` to not be the documented answer in consumer repos.

💘 Generated with Crush

Assisted-By: Crush:glm-5.3-flash
