# TODO List

Short- and mid-term actionable tasks. Long-term direction lives in
`ROADMAP.md`.

**Prime directive for this phase: native-max APIs, not TS 1:1 ports.**

## Go (flagship)

- [x] Type-keyed primary service API: `Provide[T](ctx, value)` / `Get[T](ctx)`
      via `reflect.Type`; named services remain for realm semantics
- [x] Typed events: `On[E any](ctx, func(E))` / `Emit(ctx, E)`; string
      events only for the `internal/` namespace
- [x] Stdlib `context.Context` per fiber, cancelled on unload/dispose
      (`fiber.StdContext()`)
- [x] `fiber.Done() <-chan struct{}` for select-based coordination
- [x] `slog.Handler` adapter for the logger service
- [x] Fix `Context.Inject` to return `(*Fiber, error)` (previously logged
      and returned nil)
- [x] Collision-free isolate labels: `map[any]isolateKey` instead of
      `fmt.Sprintf` synthetic keys
- [x] Root-fiber behavior tests (Restart/Update/On/Provide on root; the
      root Restart panic was caught by review, not tests)
- [ ] `internal/listener`, `internal/dispatch`, `internal/get`, `internal/set`
      interception events (needed by loader and hmr ports)
- [ ] Service accessor/mixin system (Property.Accessor upstream)
- [ ] Callable services and tracker based effect attribution
- [ ] Port `packages/loader`, `packages/hmr`, `packages/timer`,
      `packages/group`

## Rust

- [x] TypeId-keyed typed services (`ctx.get::<T>()`)
- [x] Typed events via TypeId (`ctx.on::<E>(Fn(&E))`)
- [x] RAII disposer guards (dispose on Drop, `detach()` to opt out)
- [x] `Plugin` trait with associated `Config` type
- [ ] Collision-free `isolate_shared` labels: tuple-keyed label table like
      Go's `map[any]isolateKey` (the `"{name}\0{label}"` synthetic key can
      collide)
- [ ] `intercept` equivalent (Go has Intercept/Intercepted)
- [ ] Config validation + `update` with typed configs
- [ ] Thread-safe variant (`Arc`/`Mutex` core) behind a feature flag
- [ ] True parallel dispatch with scoped threads
- [ ] Registry view parity (snapshot restore)
- [ ] Emit `internal/status` fiber-state events (Go parity)
- [ ] Doctests for typed services/events

## Zig

- [x] Comptime plugin construction with typed configs
- [x] Type-keyed services/events via comptime type identity
- [x] Split domain errors from `Allocator.Error`
- [ ] Collision-free `isolateShared` labels (same synthetic-key issue as
      Rust `isolate_shared`)
- [ ] `Context.effect` nested-scope API (Go/Rust parity)
- [ ] Registry view (size / delete) on top of the runtime map
- [ ] serial / waterfall / parallel dispatch modes
- [ ] Typed `once` for events
- [ ] Early disposal handles (Disposer) for `on` and `provide`
- [ ] Batch transactions
- [ ] Effect labels exposed for introspection
- [ ] Emit `internal/status` fiber-state events (Go parity)

## Go follow-ups

- [ ] Enumerate and cover the uncovered 13.5% (Waterfall panic guards,
      Parallel recover paths, slog edges) to reach 90%+
- [ ] `Fiber.Err()` public accessor (apply error currently only via Await)
- [ ] Evaluate `errors.AsType[*Error]` migration in `errors.go` (Go 1.26)
- [ ] Typed-inject sugar (`InjectTypes[T1, T2]()` style) to avoid stringly
      `ServiceName[T]()` at every inject site

## Golden tests

- [ ] Scenario #2: events, filters and global listeners across the three
      ports
- [ ] Scenario #3: nested plugins and registry delete cascade
- [ ] Unit-test the per-port DSL parsers (malformed tokens currently
      surface only as confusing trace mismatches)

## Repo

- [x] Verify CI action versions; first green `ports.yml` run (actions
      verified, workflows linted with actionlint; the run itself requires
      a push)
- [x] `nix flake check` derivations for the three suites
- [x] `FEATURES.md` and `docs/DOMAIN_LANGUAGE.md`
- [x] Cross-language golden scenario test (one spec, three runners; see
      `golden/README.md`)
- [ ] Push the branch and confirm the first green `ports.yml` run; record
      the run URL here
- [ ] Add a `golangci-lint` job to `ports.yml` (clean locally, unenforced
      in CI) and consider `-count=3` on the Go test step as the flake
      canary
- [ ] PORTS.md: cross-port API comparison table (typed service/event/
      plugin forms side by side)
- [ ] AGENTS.md: Zig 0.16 std gotchas (`std.Io.Dir.cwd`,
      `ArrayListUnmanaged .empty`, anonymous non-zig imports → WriteFiles
      + `@embedFile`)
- [ ] Root README: CI badges + port pitch with golden-test mention
