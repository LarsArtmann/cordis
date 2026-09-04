# TODO List

Short- and mid-term actionable tasks. Long-term direction lives in
`ROADMAP.md`. Full 27-task breakdown:
`docs/planning/2026-09-04_15-42_ship-parity-ecosystem-pareto-plan.md`.

**Prime directive for this phase: native-max APIs, not TS 1:1 ports.**

## Go (flagship)

- [x] Type-keyed primary service API (`Provide[T]`/`Get[T]`/`TryGet[T]`/
      `MustGet[T]`), typed events (`On[E]`/`Once[E]`/`Emit[E]`), stdlib
      `context.Context` per fiber, `slog` adapter, `Inject` returning
      `(*Fiber, error)`, collision-free isolate labels, root-fiber tests
- [x] `internal/get|set|listener|dispatch` interception events
      (loader/hmr prerequisite); `Context.Cleanup` exported for plugins
- [x] `go/timer` package: `AfterFunc`/`Await`/`Interval`/`IntervalFunc`/
      `Throttle`/`Debounce`, effect-scoped, race-tested
- [x] `go/group` package: id-keyed child fibers with diffed `Update`,
      rollback with the owning fiber
- [x] Coverage ≥ 90% (90.4%): dispatch modes, waterfall guards, logger
      edges, interception events, StdContext lifecycle, error types
- [ ] Service accessor/mixin system (Property.Accessor upstream)
- [ ] Callable services and tracker based effect attribution
- [ ] Port `packages/loader`, `packages/hmr`

## Rust

- [x] TypeId-keyed typed services/events, RAII `Guard` disposer,
      `Plugin` trait with associated `Config`, collision-free
      `isolate_shared` labels (`(name, label)` hash map)
- [x] `thread-safe` feature: `Arc<Mutex>` core, `Send + Sync` bounds via
      `sync::Shared`/`MaybeSendSync`, lock-order invariant (Core before
      FiberData), multi-thread stress tests, CI job
- [x] `Context::intercept`/`intercepted` config overrides,
      `FnPlugin::validate` config validation gating start,
      `Fiber::update_config` typed update
- [x] `parallel` dispatch concurrent under `thread-safe` (scoped threads)
- [ ] `internal/status` fiber-state event emission (Go parity)
- [ ] Registry snapshot restore
- [ ] Reach deny-level for pedantic/nursery clippy lints (~194 findings;
      softened to warn in 5dfa504-follow-up to unbreak the flake)

## Zig

- [x] Collision-free `isolateShared` labels (content-hashed pair keys)
- [x] `Context.effect` named effect scopes + `effects()` introspection
      (`EffectMeta` trees), idempotent `Effect.dispose`
- [x] `Disposer` early-disposal handles for `on`/`provide` (shared done
      flag with scope rollback), `Registry` view (size/has/delete),
      `onceTyped`, `onGlobal`
- [x] `serial`/`waterfall` (runtime-name `Next` chain)/`parallel`/`batch`
      dispatch modes
- [ ] Registry `has`/`delete` by plugin identity for the golden runner
      (currently address-keyed dynamic plugins)
- [ ] `zig build -femit-docs` pass; fix broken doc comments

## Golden tests

- [x] Scenario #1 lifecycle (byte-identical across Go/Rust/Zig)
- [x] Scenario #2 events, filters and global listeners
      (`golden/scenario-events.txt`)
- [x] Scenario #3 nested plugins + registry delete cascade
      (`golden/scenario-cascade.txt`)
- [x] DSL parser unit tests in all three runners (malformed tokens)
- [ ] Scenario #4: bail/serial/waterfall dispatch parity

## Repo

- [x] CI action versions verified; green `ports.yml` runs recorded (see
      below); golangci-lint job, `-count=3` flake canary, golden
      double-run canary
- [x] `nix flake check` derivations green (re-verified after the Rust
      sync refactor; clippy scope moved to the crate lint table)
- [x] `FEATURES.md` and `docs/DOMAIN_LANGUAGE.md`
- [x] Push + first green runs:
      https://github.com/LarsArtmann/cordis/actions/runs/33877878539 and
      https://github.com/LarsArtmann/cordis/actions/runs/33879875106
- [ ] Re-record a green run for the 2026-09-04 evening batch (timer,
      group, interception events, golden #2/#3, Rust thread-safe)
- [ ] PORTS.md: cross-port API comparison table (typed service/event/
      plugin forms side by side)
- [ ] AGENTS.md: Zig 0.16 std gotchas (`std.Io.Dir.cwd`,
      `ArrayListUnmanaged .empty`, anonymous non-zig imports → WriteFiles
      + `@embedFile`); Rust lock-order invariant (Core before FiberData)
- [ ] Root README: CI badges + port pitch with golden-test mention

## Quality extras

- [ ] Benchmark skeletons: drain-queue throughput (Go first) with results
      table
- [ ] Property test: LIFO rollback order under randomized registration
      (Go)
- [ ] Parity-matrix generator from FEATURES.md (drift alarm vs ROADMAP)
- [ ] Releases: tag `go/v0.1.0`, bump+tag Rust `v0.2.0`, note the Zig
      version
- [ ] Weekly "run all three suites + flake check" cadence note in
      AGENTS.md
