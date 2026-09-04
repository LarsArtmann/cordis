<div align="center">
  <h1 id="satori">Cordis</h1>
</div>

A Meta-Framework of Spatiotemporal Composability.

**Cordis is under active development. The API is not yet stable and may change without notice.**

- Paper: _A Programming Paradigm for Spatiotemporal Composability_ [[arXiv](https://arxiv.org/abs/2608.25512)] [[repository](https://github.com/cordiverse/paper)]
- Documentation: [cordis-primer](https://deepseek-harness.github.io/deepseek-harness/reference/cordis-primer) (official documentation is still under construction)

## Multi-language ports

This fork extends Cordis beyond TypeScript. The Go port is the flagship and
reference implementation; Rust and Zig follow its architecture.

| Language | Directory | Status | Test command |
| -------- | --------- | ------ | ------------ |
| Go | [`go/`](go/) | Core complete with full test parity suite | `cd go && go test ./...` |
| Rust | [`rust/`](rust/) | Foundation: contexts, fibers, events, services, isolation, inject reactivity | `cd rust && cargo test` |
| Zig | [`zig/`](zig/) | Foundation: contexts, fibers, events, services, isolation, inject reactivity | `cd zig && zig build test` |

See [PORTS.md](PORTS.md) for the shared port architecture and
[ROADMAP.md](ROADMAP.md) for the parity matrix and planned work. A nix
flake provides toolchains and test runners: `nix run .#test`.
