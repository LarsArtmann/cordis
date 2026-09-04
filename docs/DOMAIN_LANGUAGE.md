# Domain Language

The ubiquitous language of cordis, shared by the TypeScript original and
the Go, Rust and Zig ports. These terms appear in every port's API and
docs; use them consistently in code, commits and discussions.

## Core concepts

**Cordis** — a meta-framework of _spatiotemporal composability_: the
_spatial_ dimension is which services exist where (scopes, realms); the
_temporal_ dimension is which code runs when (fibers reacting to the
service set).

**Context** — a scope in the context tree. Carries services, event
listeners and effects. Created by `New` (root), `Extend` (plain child),
`Isolate` (child with its own service realm) or `Intercept` (child with
service config overrides). The single handle through which plugins
interact with the framework.

**Plugin** — a typed unit of composable behavior: a name, injected
service dependencies, an optional config validator and an apply function.
Definitions: Go `NewPlugin[C]`; Rust `Plugin` trait (associated `Config`)
or `plugin()` closure (`FnPlugin`); Zig `TypedPlugin` comptime constructor
or runtime `Plugin` value; TypeScript function/object with `apply`.

**Fiber** — one instance of a running plugin: an _effect scope with a
lifecycle_. When a fiber leaves the active state, everything it
registered rolls back; when its dependencies return, the same fiber
instance reloads in place. The set of running code is a pure function of
the set of available services.

**Fiber states** — `pending` (waiting for injected services), `loading`
(plugin body executing), `active` (effects live), `failed` (body errored,
partial effects rolled back), `unloading` (rolling back), `disposed`
(permanently dead). The root fiber is always active; "disposing" it
restarts the root scope.

**Effect** — one registration (listener, service, nested plugin, plain
cleanup) tracked as a labeled item in a dispose bag. Effects form a tree:
registrations made while an effect body runs become its children and roll
back together. Disposal is always **LIFO** (last in, first out).

**Cleanup** — the function releasing one resource; runs on rollback.
Panicking cleanups are caught and routed to the logger, never aborting
sibling disposal.

**Disposer** — the handle removing one registration ahead of the fiber's
lifecycle; idempotent. Rust additionally offers `Guard`, the RAII form
(dispose on drop, `detach()` to opt out).

**Drain queue** — the ports' replacement for the TypeScript microtask
queue: fiber state transitions triggered inside a public API call are
coalesced and settle before the outermost call returns, so torn
intermediate states are unobservable. A **Batch** groups several
mutations into one such transaction.

## Services

**Service** — a named value published in a realm (`provide`). Consumers
look it up (`get`) or declare a hard dependency (`inject`).

**Typed service** — the primary service form in the ports: the service
name derives from the value's _type identity_ (Go `ServiceName[T]` /
reflect, Rust `type_name`, Zig `@typeName`), so provider and consumers
cannot drift apart on a hand written string. Named services remain for
dynamic names (loader, hmr) and cross-realm contracts.

**Realm (isolation)** — an isolated namespace for one service name.
`Isolate(name, label)` creates a child scope whose `name` resolves to a
fresh realm; scopes sharing a label share the realm, mirroring the realm
symbols upstream. Lookups walk the scope chain and fall back to the root
realm.

**Inject** — a plugin's hard dependency declaration. The fiber stays
`pending` until every injected service is available and its provider is
active, unloads when one disappears, reloads when it returns. Also the
name of the anonymous-plugin convenience (`Context.Inject`/`inject`).

**Intercept** — a per-scope configuration override for a service,
mirroring `ctx.intercept()` upstream. The logger service honors logger
intercepts (name, level).

## Events

**Event** — a typed (primary) or string (internal namespace) message
delivered to listeners. Listener registration is bound to the registering
fiber and rolls back with it.

**Dispatch modes** — `emit` (synchronous, in order), `parallel`
(concurrent, joined errors), `serial`/`bail` (first non-nil result wins),
`waterfall` (listeners composed around a terminal function).

**Filter** — an emission scope restriction: events emitted through a
filtered context reach only listeners whose owner context passes the
filter. **Global listeners** bypass filters. **Realm filter** — the filter
matching listeners in the emitter's realm for one service name.

## Runtime plumbing

**Runtime** — every live fiber of one plugin. Starting a plugin twice
creates two fibers of one runtime.

**Registry** — the view of all runtimes in a context tree: size, has,
delete (dispose every fiber, restoring the pre-start state).

**Logger service** — the per-tree logging facility with levels, exporters
and a bounded buffer. Go bridges it to `log/slog` via `NewSlogHandler`.

**StdContext (Go)** — the stdlib `context.Context` of a fiber's current
activation, cancelled on unload/restart/dispose and renewed on every
load; `Fiber.Done()` is its select-friendly channel view.

## Testing vocabulary

**Golden scenario** — the shared script in `golden/` executed verbatim by
the Go, Rust and Zig runners; the emitted trace must match
`golden/expected.txt` exactly in every language. A divergence means one
port's semantics drifted.
