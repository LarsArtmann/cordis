# TODO List

Short- and mid-term actionable tasks, harvested from
`docs/status/2026-08-22_10-19_multi-language-ports-go-flagship.md`.
Long-term direction lives in `ROADMAP.md`.

**Prime directive for this phase: native-max APIs, not TS 1:1 ports.**

## Go (flagship)

- [ ] Type-keyed primary service API: `Provide[T](ctx, value)` / `Get[T](ctx)`
      via `reflect.Type`; named services remain for realm semantics
- [ ] Typed events: `On[E any](ctx, func(E))` / `Emit(ctx, E)`; string
      events only for the `internal/` namespace
- [ ] Stdlib `context.Context` per fiber, cancelled on unload/dispose
      (`fiber.StdContext()`)
- [ ] `fiber.Done() <-chan struct{}` for select-based coordination
- [ ] `slog.Handler` adapter for the logger service
- [ ] Fix `Context.Inject` to return `(*Fiber, error)` (currently logs and
      returns nil)
- [ ] Collision-free isolate labels: `map[any]isolateKey` instead of
      `fmt.Sprintf` synthetic keys
- [ ] Root-fiber behavior tests (Restart/Update/On/Provide on root; the
      root Restart panic was caught by review, not tests)

## Rust

- [ ] TypeId-keyed typed services (`ctx.get::<T>()`)
- [ ] Typed events via TypeId (`ctx.on::<E>(Fn(&E))`)
- [ ] RAII disposer guards (dispose on Drop, `detach()` to opt out)
- [ ] `Plugin` trait with associated `Config` type

## Zig

- [ ] Comptime plugin construction with typed configs
- [ ] Type-keyed services/events via comptime type identity
- [ ] Split domain errors from `Allocator.Error`

## Repo

- [ ] Verify CI action versions; get first green `ports.yml` run
- [ ] `nix flake check` derivations for the three suites
- [ ] `FEATURES.md` and `docs/DOMAIN_LANGUAGE.md`
- [ ] Cross-language golden scenario test (one spec, three runners)
