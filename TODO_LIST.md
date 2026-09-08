# TODO List

Short- and mid-term actionable tasks. Long-term direction and open decisions
live in `ROADMAP.md`; completed work is logged in `CHANGELOG.md`, never here.

**Prime directive: native-max APIs, not TS 1:1 ports.**

## Go (flagship)

- [ ] Golden scenario #4: bail/serial/waterfall dispatch parity, byte-identical
      across all three runners (source:
      docs/status/2026-09-04_19-09_pareto-execution-pass.md §f10;
      `golden/` still ships only the lifecycle, events and cascade scenarios)
- [ ] hmr: include `Fiber.Err()` detail in rollback errors (now possible
      post-M26) (source: docs/status/2026-09-05_03-03 §f20)
- [ ] loader: `Resolver.ReplaceType[C]` sugar, parity with `RegisterType[C]`
      (source: docs/status/2026-09-04_22-48 §f30)
- [ ] hmr: concurrency storm test — parallel `Swap` racing
      `Tree.Create`/`Remove` (source: docs/status/2026-09-04_22-48 §f31)
- [ ] Raise `go/loader` statement coverage (74.6% measured 2026-09-08)
      toward the ~90% bar the other packages hold (source: measured via
      `go test -cover` under Go 1.27)
- [ ] `go fix ./...` modernizer sweep under Go 1.27 (embedlit, unsafefuncs,
      atomictypes) (source: docs/status/2026-09-08_04-04 §f21)
- [ ] `IntervalFunc`: a slow callback can outlive disposal — fix the
      pump-goroutine lifetime or document the constraint
      (source: docs/status/2026-09-05_03-03 §f13)
- [ ] Timer property test: debounce/throttle fire counts under randomized
      call sequences (source: docs/status/2026-09-05_03-03 §f45)
- [ ] Regression tests asserting the wrapped error messages in
      `accessor.go`/`loader/tree.go` (source:
      docs/status/2026-09-08_05-27 §f31)
- [ ] `Tree.Await`: decide whether the discarded fiber error should surface
      through the loader's error sink, or document the discard as final
      (source: docs/status/2026-09-08_05-27 §f33)
- [ ] Golden scenario candidate: loader watch/reload trace
      (source: docs/status/2026-09-08_15-43 §f21)

## Rust

- [ ] `internal/plugin` + `internal/update` interception events (M13 parity;
      Go has them, Rust does not) (source:
      docs/status/2026-09-04_22-48 §f32)
- [ ] Verify root-fiber status emission: `FiberData::new_root` writes state
      without going through `settle_state` — cover or document
      (source: docs/status/2026-09-04_22-48 §f33; `rust/src/fiber.rs:82`)
- [ ] Fix or allowlist the `significant_drop` nursery findings under
      `--features thread-safe`, then gate
      `cargo clippy --features thread-safe` in Ports (source:
      docs/status/2026-09-08_04-32 §f19–20)
- [ ] `cargo-llvm-cov` coverage baseline next to the Go numbers (source:
      docs/status/2026-09-08_04-32 §f22)
- [ ] `cargo bench` to substantiate or hedge the "up to 30% faster small
      allocations" claim recorded in ROADMAP (source:
      docs/status/2026-09-08_04-32 §f23)

## Zig

- [ ] Registry `has`/`delete` keyed by `TypedPlugin` identity (the golden
      runner still goes through address-keyed dynamic plugins)
- [ ] `zig build -femit-docs` pass; fix broken doc comments
- [ ] Record Zig 0.16 std gotchas in AGENTS.md (`std.Io.Dir.cwd`,
      `ArrayListUnmanaged .empty`, anonymous non-zig imports → WriteFiles +
      `@embedFile`)

## Repo

- [ ] CI job running `nix flake check` so the flake gate is enforced
      remotely, not only locally (source:
      docs/status/2026-09-08_04-32 §f40; `ports.yml` has no nix step)
- [ ] CI: add `.prettierrc` (printWidth 100) plus a `prettier --check` and
      `yarn build` step before tests in build.yml (source:
      docs/status/2026-09-08_15-43 §f24–25)
- [ ] CI guards: `packages/**` stays byte-identical to upstream, and
      `dprint.json` excludes keep covering `packages/**` (source:
      docs/status/2026-09-08_05-27 §f13–14)
- [ ] After the next push (user-gated, force-with-lease): verify build.yml
      and ports.yml green on GitHub, including the replayed `3-stage-hmr`
      line (source: docs/status/2026-09-08_15-43 §c; replayed in `b4650df`)
- [ ] Gitignore `tmp-*` test debris (`lib/`, `node_modules/`,
      `tsconfig.tsbuildinfo` are already covered) (source:
      docs/status/2026-09-08_15-43 §f32)
- [ ] TS: one full install-from-scratch verification
      (`rm -rf node_modules && yarn install && yarn build && yarn test`)
      (source: docs/status/2026-09-08_15-43 §f40)
- [ ] Review CONTRIBUTING.md for accuracy (never reviewed since pick 12);
      add the flake app list to the quickstart; document the
      "upstream semantics + fork formatting (prettier-100)" rebase policy
      (source: docs/status/2026-09-08_04-32 §f38,
      docs/status/2026-09-08_15-43 §f42, §f45)
- [ ] Loader: fuzz the JSON config layer (EncodeConfig/DecodeConfig
      roundtrip with random shapes) (source:
      docs/status/2026-09-05_03-03 §f46)
- [ ] Align the local gate with CI race canary: flake checks run
      `-race -count=1`, ports.yml runs `-count=3` (source:
      docs/status/2026-09-08_05-27 §f35)
