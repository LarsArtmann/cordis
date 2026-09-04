# Status Report — Multi-Language Ports (Go flagship)

**Date:** 2026-08-22 10:19
**Scope:** First porting session: cordis (TypeScript, `packages/`) → Go, Rust, Zig
**Verified at report time:** `nix run .#test` fully green (Go race-tested, Rust 20/20 + clippy deny-warnings, Zig 10/10 leak-free)

---

## Headline

All three ports exist, build, and pass their parity suites. **But the ports
are currently too faithful to the TypeScript original.** The user directive
for everything that follows: _use each language's native features to the
max, do not port TypeScript 1:1._ The current APIs carry TS-isms (stringly
named untyped events, string-keyed services, `any`-typed values, no stdlib
`context.Context`, no `slog`, `Rc<dyn Any>` ceremony in Rust, disabled RAII)
that must be replaced with native designs. Semantics parity (fiber
lifecycle, drain semantics, realms) stays; API surface gets redesigned per
language.

---

## a) FULLY DONE

### Go (`go/`, module `github.com/LarsArtmann/cordis/go`, package `cordis`)

- **Core runtime**: shared `core` state, one-mutex guarding, lock-free user
  callbacks, re-entrant API depth tracking.
- **Drain queue**: fiber transitions coalesce and settle before the
  outermost public call returns; torn intermediate states unobservable.
  Replaces TS microtasks. Verified by the update-coordination test
  (`[1,"old"] → [2,"new"]`, never `[2,"old"]`).
- **Context tree**: `Extend`, `Isolate` (+ shared labels), `Intercept`,
  `WithFilter`, `RealmFilter`, `Batch`.
- **Fiber lifecycle**: all six states, `Dispose`/`Restart`/`Update`/`Await`,
  in-place reload, root-fiber dispose-restarts semantics, UID 0/-1 parity.
- **Effect tree**: labeled nested bags, LIFO rollback, idempotent disposers,
  `GetEffects` introspection, panic recovery into the logger.
- **Events**: emit / parallel (real goroutines, `errors.Join`) / serial /
  bail / waterfall, prepend + global options, filtered emission, `Once`.
- **Services**: `Provide`/`Get`/`Get[T]`/`MustGet[T]`, realm-keyed store,
  optional check functions, duplicate detection with upstream error
  messages.
- **Plugins**: typed config via generics (`Plugin[C]`), `Inject`,
  `InjectConfig`, `Validate`, anonymous `ctx.Inject`, name derivation.
- **Registry**: Size/Has/Get/Delete with exact snapshot restore.
- **Logger**: levels, exporters, bounded buffer, intercept-based naming,
  format verbs (`%s %d %i %f %o %O %%`), joined-error expansion, minimal
  `ConsoleExporter`.
- **Tests**: 30+ tests across 7 files + doc example; `-race` clean;
  84.8% statement coverage; `go vet` + `gofmt` clean.

### Rust (`rust/`, crate `cordis`)

- Same architecture transliterated: core arena of fibers, drain queue,
  effect bags, realm keys, inject reactivity, registry with delete.
- 20/20 parity tests; `cargo clippy --all-targets -- --deny warnings` clean.
- Documented single-threaded (`Rc`/`RefCell`) execution model.

### Zig (`zig/`, Zig 0.16)

- Same architecture: core, drain queue, fibers, realms, events (emit/bail),
  inject reactivity, arena-owned memory (whole tree freed by one `deinit`).
- 10/10 tests, leak-free under `std.testing.allocator`.
- `build.zig` test step.

### Repo

- `flake.nix` + `flake.lock`: devShell (go/rust/zig/node) and
  `nix run .#test` / `#test-go` / `#test-rust` / `#test-zig` — all verified.
- CI: `.github/workflows/ports.yml` (go/rust/zig jobs).
- Docs: root README ports section, `PORTS.md` (shared architecture),
  `ROADMAP.md` (parity matrix), `AGENTS.md` (project memory),
  `go/README.md`.
- `.gitignore` entries for `rust/target`, `zig/.zig-cache`, `zig/zig-out`.

## b) PARTIALLY DONE

- **API idiom fit** — the big one; see (e). Everything works, but the
  surfaces are TS-shaped: untyped `...any` events, string-keyed services as
  the primary API, no stdlib context integration, no slog bridge.
- **Go logger**: no color output parity (`%c`/`%C` render plain);
  `ConsoleExporter` is a minimal stand-in for `packages/logger-console`.
- **Rust `parallel`**: runs sequentially (correct error aggregation, no
  concurrency) — documented limitation of the single-threaded core.
- **Rust effect introspection**: `EffectMeta` exists; nested labels not
  exposed on all registration kinds.
- **Zig events**: emit + bail only; no serial/waterfall/parallel.
- **Zig registry**: runtime map exists internally; no public view or delete.
- **Zig disposers**: no early-removal handles (`on`/`provide` roll back only
  with their fiber).
- **CI**: `ports.yml` written but never executed; action versions
  (`setup-go@v6`, `mlugg/setup-zig@v2`, `dtolnay/rust-toolchain@stable`)
  unverified.
- **Isolate labels (Go)**: non-key labels map through `fmt.Sprintf` synthetic
  keys — collision-prone for exotic label values.
- **`Context.Inject` (Go)**: returns `nil` fiber and logs on error instead of
  returning `(nil, error)` — API wart.
- **Docs set**: README/PORTS/ROADMAP/AGENTS done; `FEATURES.md`,
  `TODO_LIST.md`, `docs/DOMAIN_LANGUAGE.md` missing per repo conventions.

## c) NOT STARTED

- Ports of `packages/loader`, `packages/hmr`, `packages/timer`,
  `packages/group`, `packages/include`, `packages/create`.
- `internal/listener`, `internal/dispatch`, `internal/get`, `internal/set`
  interception events (prerequisite for loader/hmr ports).
- Service accessor/mixin system and tracker-based effect attribution
  (`Service.tracker` upstream).
- Thread-safe Rust variant; comptime-typed Zig APIs; typed/native event and
  service APIs in all three languages.
- Cross-language golden tests; publishing (Go module tags, crates.io, Zig
  package index); benchmarks; fuzz/property tests; `nix flake check`
  derivations.

## d) TOTALLY FUCKED UP

Nothing shipped in a broken state. Things that were broken mid-session and
fixed before landing, kept here for honesty:

1. **Event-filter self-deadlock (Go)** — `resolveHooks` ran user filter
   closures while holding `core.mu`; `RealmFilter` closures lock `core.mu`.
   Found by a 600s test timeout. Fixed: snapshot under lock, filter outside.
2. **Root-fiber `Restart()` nil-pointer panic (Go)** — `Restart` queued the
   root fiber into `transition()`, whose load path dereferences
   `runtime.base.apply` (nil for root). Found in self-review, not by a test
   — no test covered it. Fixed + root restart path shared with `Dispose`.
3. **`Once` race window (Go)** — event firing between registration and
   disposer assignment leaked the listener. Fixed with a mutex + late-fire
   check.
4. **Silent cleanup leak (Go)** — registration racing a fiber unload pushed
   into an already-drained bag; cleanup would never run. Fixed with a
   post-push staleness check that disposes immediately and returns
   `ErrInactiveEffect`.
5. **Rust test file first draft** — contained unreachable `!`-type hack
   helpers; rewritten properly with a new public `Context::attach` API
   (which turned out to be genuinely useful).
6. **Zig double-cleanup on deinit** — entries were copied by value during
   rollback, so `done` flags never persisted and `deinit` re-ran cleanups.
   Fixed by iterating bag items by pointer.
7. **README symlink surprise** — root `README.md` is a symlink to
   `packages/core/README.md`; my root edit wrote through it. Content is
   correct for the root view; direct blob view of `packages/core/README.md`
   has root-relative links that only resolve from the root. Accepted
   (upstream's symlink design).

## e) WHAT WE SHOULD IMPROVE

**Theme: native-max, not TS 1:1.** Per language, highest leverage first:

### Go

1. **Typed services as the primary API**: `Provide[T](ctx, value)` /
   `Get[T](ctx)` keyed by `reflect.Type`; keep named services for realm
   isolation semantics (realms are name-scoped upstream).
2. **Typed events**: `On[E any](ctx, func(E))` / `Emit(ctx, E)` with
   type-keyed dispatch; keep string events only for the `internal/`
   namespace that loader/hmr need.
3. **Stdlib `context.Context` per fiber**, cancelled on unload/dispose —
   the native way to stop plugin goroutines. `fiber.StdContext()`.
4. **`fiber.Done() <-chan struct{}`** for select-based coordination.
5. **`slog` integration**: logger service as/behind a `slog.Handler`.
6. Fix `Context.Inject` to return `(*Fiber, error)`.
7. Isolate labels: replace `fmt`-string keys with `map[any]isolateKey`
   (comparable labels, zero collision risk).

### Rust

8. **Type-keyed service store** (`ctx.get::<T>()`) and **typed events**
   (`ctx.on::<E>(Fn(&E))`) via `TypeId`; the `Rc<dyn Any>` `Value` ceremony
   should disappear from the public API.
9. **RAII disposers** (guard that disposes on `Drop`, with `detach()` to opt
   out) — the current Drop-is-no-op choice is a TS-ism.
10. **Plugin as a trait** with an associated `Config` type instead of the
    closure-factory alone.
11. True `parallel` via `std::thread::scope`; Send/Sync variant behind a
    feature flag.

### Zig

12. **Comptime plugin construction** (`Plugin(comptime C, apply)`) with
    typed config unwrapping.
13. **Type-keyed services/events** using comptime type identity instead of
    strings where possible.
14. Split error sets: domain errors vs `Allocator.Error` propagation.

### Process

15. The root-Restart panic was caught by review, not tests — add tests for
    every public method on the root fiber (they behave differently).
16. Verify CI action versions; the workflow has never run.
17. Keep the parity matrix in `ROADMAP.md` updated as native APIs land —
    mark where native design intentionally diverges from TS.

## f) Next tasks (50, impact-ordered)

Native-max redesign (Go flagship first, then propagate):

1. Go: type-keyed `Provide[T]`/`Get[T]` primary service API.
2. Go: typed events `On[E]`/`Emit(E)` alongside string events.
3. Go: stdlib `context.Context` per fiber (cancel on unload/dispose).
4. Go: `fiber.Done() <-chan struct{}`.
5. Go: `slog.Handler` adapter for the logger service.
6. Go: fix `Context.Inject` signature to return an error.
7. Go: collision-free isolate labels (`map[any]isolateKey`).
8. Go: root-fiber behavior tests (Restart/Update/On/Provide on root).
9. Rust: TypeId-keyed typed services.
10. Rust: typed events via TypeId.
11. Rust: RAII disposer guards with `detach()`.
12. Rust: `Plugin` trait with associated `Config`.
13. Zig: comptime plugin construction with typed configs.
14. Zig: type-keyed services/events.
15. Zig: split domain vs allocator error sets.
16. Go: `internal/listener` + `internal/dispatch` interception events.
17. Go: `internal/get` + `internal/set` (accessor system foundation).
18. Go: service accessor/mixin equivalent (typed, not dynamic).
19. Go: tracker-based effect attribution (effects via services attributed to
    the calling fiber).
20. Go: port `packages/timer` — showcase of native design (time.Timer +
    fiber-bound cleanup).
21. Go: port `packages/group`.
22. Go: port `packages/loader` (schema-validated, config-file driven).
23. Go: port `packages/hmr`.
24. Go: `errors.AsType` modernization pass (Go 1.26 idiom).
25. Go: benchmarks (emit, provide+notify storm, start/dispose cycle).
26. Go: fuzz/property tests for the drain/transition state machine.
27. Go: race stress test — provide/unprovide storms vs inject consumers.
28. Go: coverage gate in CI (≥85%).
29. Go: example tests per feature (events, inject, isolate, batch, update).
30. Go: godoc polish pass (every exported symbol has an example-quality
    comment).
31. Rust: true parallel dispatch with scoped threads.
32. Rust: Send/Sync core behind a feature flag.
33. Rust: miri job in CI.
34. Rust: config validation + typed `update`.
35. Rust: `Context::batch`.
36. Zig: public registry view + delete.
37. Zig: early `Disposer` handles for `on`/`provide`.
38. Zig: serial/waterfall/parallel dispatch modes.
39. Zig: batch transactions.
40. Zig: stable plugin identity without address fragility.
41. Repo: cross-language golden scenario tests (one spec, three runners).
42. Repo: `nix flake check` derivations wrapping the three suites.
43. Repo: verify/pin CI action versions; first green CI run.
44. Repo: `TODO_LIST.md` (harvested from this report) + `FEATURES.md`.
45. Repo: `docs/DOMAIN_LANGUAGE.md` (fiber, realm, drain, effect, inject).
46. Repo: publishing plan — Go `go/v0.x` tags, crates.io name, Zig index.
47. Repo: ADR for drain-queue-vs-microtasks and native-API divergence rules.
48. Repo: example app (greeter) in all three languages.
49. Repo: data-driven runner executing TS suite expectations against Go.
50. Repo: perf comparison note (Go vs TS event dispatch).

## g) Questions I cannot answer myself

1. **Fork identity and publishing targets.** The Go module path assumes
   `github.com/LarsArtmann/cordis` and nothing is published anywhere yet.
   Where will this fork live, and should the ports be published (Go module
   tags, crates.io, Zig package index) or stay workspace-internal?
2. **Divergence budget.** When native idiom and TS semantics conflict
   (example: type-keyed DI makes `Isolate(name)` realms awkward, since
   realms are name-scoped upstream), which wins — a native API with
   documented semantic divergence, or parity with a less native API? My
   default absent an answer: hybrid — type-keyed primary API, name-keyed
   realms preserved underneath.
3. **Threading requirements.** Are the Rust and Zig ports expected to
   support multi-threaded runtimes (Send/Sync core, real parallel dispatch),
   or is single-threaded acceptable for the intended use cases? This decides
   whether the Rust core gets an `Arc<Mutex>` redesign now or later.

---

_Report written per status-report skill; user explicitly requested Markdown
over the skill's HTML dashboard default. Not committed — waiting for
instructions._
