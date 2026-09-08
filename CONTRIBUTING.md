# Contributing

Thanks for your interest in contributing!

This fork of [cordiverse/cordis](https://github.com/cordiverse/cordis) keeps
the TypeScript original in `packages/` (synced to upstream) and adds Go,
Rust and Zig ports next to it. `PORTS.md` describes the port architecture;
`AGENTS.md` carries deep session context and environment gotchas.

## Quickstart (Nix)

The flake owns the development environment and the test gates:

```console
nix run .#test          # all three ports: go vet+race-test, cargo clippy+test, zig build test
nix run .#test-go       # Go only
nix run .#test-rust     # Rust only (clippy + tests, default and thread-safe features)
nix run .#test-zig      # Zig only
nix flake check         # the full gate; the same check runs in CI
nix develop             # shell with Go 1.27, Rust, Zig, Node 24 and yarn
```

Before opening a PR, run `nix flake check`. It runs the Go suite with
`-race -count=3` (the same race canary as CI), cargo clippy plus the test
suites of both Rust feature variants, and the leak-checked Zig tests.

## TypeScript workspace

`packages/` tracks upstream and is formatted in upstream's style. Working on
it directly (rare - most TS work belongs upstream):

```console
nix develop -c yarn install
nix develop -c yarn build
nix develop -c yarn test
```

## Upstream sync policy

`packages/**` must stay content-identical to upstream: upstream semantics AND
upstream style. The CI `upstream-parity` job (in `ports.yml`) enforces this
against a pinned upstream commit:

- Non-TypeScript files (fixtures, READMEs, tsconfigs) must be byte-identical
  to the pin.
- TS/JS may differ from the pin only by prettier-normalizable formatting;
  both trees are normalized with the pinned prettier version and compared.
- `**/package.json` is excluded (upstream bumps versions continuously;
  manifest state is reviewed per sync).

When you sync `packages/**` to a newer upstream state, bump `UPSTREAM_PIN`
in `.github/workflows/ports.yml` in the same commit.

## Reporting Issues

Please use GitHub Issues to report bugs or request features. For changes
that belong upstream (TS sources, dependency bumps), open them against
[cordiverse/cordis](https://github.com/cordiverse/cordis) instead.
