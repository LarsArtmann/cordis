//! # cordis
//!
//! Zig port of [Cordis](https://github.com/cordiverse/cordis), a
//! meta-framework of spatiotemporal composability.
//!
//! An application is a tree of `Context` scopes. Every plugin instance runs
//! inside a `Fiber`, an effect scope with a lifecycle: when a fiber leaves
//! the active state, everything it registered rolls back in reverse order.
//! Fibers declare service dependencies via `inject` and are activated,
//! unloaded and reloaded as dependencies appear and disappear.
//!
//! Like the Go and Rust ports, state transitions are coalesced through a
//! drain queue and settle before the outermost framework call returns, and
//! user callbacks never run while internal state is locked.
//!
//! Memory: every context, fiber and plugin lives in the tree's arena and is
//! freed by `Context.deinit`. Cleanups run on rollback, never double.

const std = @import("std");
const Allocator = std.mem.Allocator;

/// A type erased event argument or service value. Zig has no RTTI; the
/// producer and consumer agree on the pointee type out of band and
/// `getTyped` / listener code restores it with @ptrCast.
pub const Value = *const anyopaque;

/// Wrap a typed pointer as a Value.
pub fn value(ptr: anytype) Value {
    const T = @TypeOf(ptr);
    if (@typeInfo(T) != .pointer) @compileError("cordis.value expects a pointer, got " ++ @typeName(T));
    return @ptrCast(ptr);
}

/// Errors surfaced by the framework API. `OutOfMemory` is part of the set
/// for every fallible registration, scope constructor and service
/// publication. Only paths with no error channel remain: dispatch
/// callbacks (matching Go and Rust) and void queries abort the process,
/// while the error log drops its line rather than abort.
pub const Error = error{
    /// An effect, listener, service or plugin was registered on a context
    /// whose fiber is no longer active.
    InactiveEffect,
    /// A service was provided twice in the same realm.
    DuplicateService,
    /// A plugin body failed.
    PluginFailed,
    /// The allocator refused a registration or scope construction.
    OutOfMemory,
};

/// The lifecycle state of a fiber, mirroring FiberState upstream.
pub const FiberState = enum {
    /// Waiting for injected services.
    pending,
    /// The plugin body is executing.
    loading,
    /// The plugin body completed and its effects are live.
    active,
    /// The plugin body failed; partial effects were rolled back.
    failed,
    /// Permanently disposed.
    disposed,
    /// Effects are being rolled back.
    unloading,
};

/// An event listener: a function pointer plus its user data.
pub const Listener = struct {
    ctx: *anyopaque,
    call: *const fn (ctx: *anyopaque, args: []const Value) ?Value,

    /// Build a Listener from a typed function and a typed data pointer.
    pub fn bind(comptime T: type, data: *T, comptime f: *const fn (*T, []const Value) ?Value) Listener {
        const wrapper = struct {
            fn call(raw: *anyopaque, args: []const Value) ?Value {
                return f(@ptrCast(@alignCast(raw)), args);
            }
        };
        return .{ .ctx = @ptrCast(data), .call = &wrapper.call };
    }
};

/// Restricts which listeners receive events emitted through a context,
/// mirroring Context.filter upstream. Build one with a data pointer and a
/// typed call, like Listener.
pub const Filter = struct {
    ctx: *anyopaque,
    call: *const fn (ctx: *anyopaque, emitter: *const Context, listener_owner: *const Context) bool,

    /// Build a Filter from a typed function and a typed data pointer.
    pub fn bind(comptime T: type, data: *T, comptime f: *const fn (*T, *const Context, *const Context) bool) Filter {
        const wrapper = struct {
            fn call(raw: *anyopaque, emitter: *const Context, listener_owner: *const Context) bool {
                return f(@ptrCast(@alignCast(raw)), emitter, listener_owner);
            }
        };
        return .{ .ctx = @ptrCast(data), .call = &wrapper.call };
    }
};

const Hook = struct {
    owner: *Context,
    listener: Listener,
    global: bool,
};

/// A plugin body. It receives the plugin it runs on, the fiber's context
/// and the raw config value; comptime constructed plugins unwrap the config
/// in their bridge before user code sees it.
pub const ApplyFn = *const fn (plugin: *const Plugin, ctx: *Context, config: ?Value) Error!void;

/// A unit of composable behavior: a name, injected dependencies and an
/// apply function. The Plugin value address is its registry identity, so
/// plugins must have a stable address (static or arena allocated).
pub const Plugin = struct {
    name: []const u8,
    inject: []const []const u8 = &.{},
    apply: ApplyFn,
    /// Runtime context passed through to apply, mirroring the
    /// std.mem.Function pattern. Comptime constructed plugins leave it null.
    data: ?*const anyopaque = null,

    /// Start this plugin on ctx with an optional config value.
    pub fn start(self: *const Plugin, ctx: *Context, config: ?Value) Error!Fiber {
        return startPlugin(ctx, self, config);
    }
};

/// Comptime plugin construction: returns a plugin type carrying the name,
/// the typed apply function and the injected dependencies, all resolved at
/// compile time. The type is the registry identity (its embedded view has
/// a unique address per instantiation), so starting it twice creates two
/// fibers of one runtime:
///
/// ```zig
/// const Greeter = cordis.TypedPlugin("greeter", Config, apply, &.{});
/// const fiber = try Greeter.start(ctx, Config{ .name = "ada" });
/// ```
///
/// The apply function receives the fully typed config; the bridge that
/// erases its type is generated at comptime.
pub fn TypedPlugin(
    comptime name: []const u8,
    comptime Config: type,
    comptime apply: *const fn (*Context, Config) Error!void,
    comptime inject: []const []const u8,
) type {
    return struct {
        pub const view = Plugin{ .name = name, .inject = inject, .apply = bridge };

        fn bridge(_: *const Plugin, ctx: *Context, raw: ?Value) Error!void {
            const config: *const Config = @ptrCast(@alignCast(raw.?));
            return apply(ctx, config.*);
        }

        /// Start the plugin on ctx with a typed config. The config is
        /// copied into the tree's arena.
        pub fn start(ctx: *Context, config: Config) Error!Fiber {
            const stored = ctx.core.a().create(Config) catch return error.OutOfMemory;
            stored.* = config;
            return startPlugin(ctx, &view, value(stored));
        }
    };
}

const Cleanup = struct {
    ctx: *anyopaque,
    call: *const fn (ctx: *anyopaque) void,
    done: bool = false,

    fn bind(comptime T: type, data: *T, comptime f: *const fn (*T) void) Cleanup {
        const wrapper = struct {
            fn call(raw: *anyopaque) void {
                f(@ptrCast(@alignCast(raw)));
            }
        };
        return .{ .ctx = @ptrCast(data), .call = &wrapper.call };
    }
};

const Entry = struct {
    label: []const u8,
    cleanup: ?*Cleanup = null,
    node: ?*EffectNode = null,
};

const Bag = std.ArrayList(Entry);

/// A named effect scope: its entries dispose together, last in, first out.
const EffectNode = struct {
    label: []const u8,
    entries: Bag = .empty,
    disposed: bool = false,
};

/// An early disposal handle for one registration (a listener, a service or
/// an effect scope). Disposing is idempotent and also runs when the owning
/// scope later rolls back: the shared done flag is set exactly once.
pub const Disposer = struct {
    cleanup: *Cleanup,
    core: *Core,

    pub fn dispose(self: Disposer) void {
        if (self.cleanup.done) return;
        self.cleanup.done = true;
        self.core.enter();
        defer self.core.leave();
        self.cleanup.call(self.cleanup.ctx);
    }
};

/// A handle to one effect scope. Dispose it early to roll back everything
/// the scope registered; disposal is idempotent.
pub const Effect = struct {
    node: *EffectNode,
    core: *Core,

    pub fn dispose(self: Effect) void {
        self.core.enter();
        defer self.core.leave();
        self.core.disposeNode(self.node);
    }
};

/// A labeled snapshot of one effect scope for introspection.
pub const EffectMeta = struct {
    label: []const u8,
    disposed: bool,
    children: []const EffectMeta,
};

/// The registry identity of a comptime plugin type: the address of its
/// embedded view, the same key `TypedPlugin.start` registers under.
fn pluginView(comptime P: type) *const Plugin {
    if (!@hasDecl(P, "view") or @TypeOf(P.view) != Plugin)
        @compileError("cordis: registry typed operations expect a cordis.TypedPlugin(...) type, got " ++ @typeName(P));
    return &P.view;
}

/// A read view over the plugin registry of one context tree.
pub const Registry = struct {
    core: *Core,

    /// How many distinct plugins have live or pending fibers.
    pub fn size(self: Registry) usize {
        return self.core.runtimes.count();
    }

    /// Whether `plugin` has any fiber in this tree.
    pub fn has(self: Registry, plugin: *const Plugin) bool {
        return self.core.runtimes.contains(@intFromPtr(plugin));
    }

    /// The typed variant of `has`: whether the comptime plugin type `P`
    /// (a `TypedPlugin` instantiation) has any fiber in this tree.
    pub fn hasTyped(self: Registry, comptime P: type) bool {
        return self.has(pluginView(P));
    }

    /// Dispose every fiber of `plugin` and remove it from the registry.
    pub fn delete(self: Registry, plugin: *const Plugin) void {
        self.core.enter();
        defer self.core.leave();
        const key = @intFromPtr(plugin);
        const list = self.core.runtimes.getPtr(key) orelse return;
        // Copy the fiber ids out and drop the registry entry first:
        // disposing removes each id from that same list (poisoning vacated
        // slots) and frees the list with the last one, exactly like the
        // snapshot-then-dispose order of the Go and Rust registries.
        const ids = self.core.gpa.dupe(usize, list.items) catch @panic("cordis: out of memory in dispatch");
        defer self.core.gpa.free(ids);
        list.deinit(self.core.gpa);
        _ = self.core.runtimes.remove(key);
        for (ids) |id| {
            (Fiber{ .core = self.core, .id = id }).dispose();
        }
    }

    /// The typed variant of `delete`: dispose every fiber of the comptime
    /// plugin type `P` and remove it from the registry.
    pub fn deleteTyped(self: Registry, comptime P: type) void {
        self.delete(pluginView(P));
    }
};

const FiberData = struct {
    uid: i64,
    ctx: *Context,
    parent: *Context,
    config: ?Value,
    inject: []const []const u8,
    plugin: ?*const Plugin,
    state: FiberState,
    disposed: bool,
    restart_requested: bool,
    queued: bool,
    executing: bool,
    bag: ?Bag,
};

/// A handle to a fiber: one instance of a running plugin.
pub const Fiber = struct {
    core: *Core,
    id: usize,

    fn data(self: Fiber) *FiberData {
        return self.core.fibers.items[self.id].?;
    }

    /// The current lifecycle state.
    pub fn state(self: Fiber) FiberState {
        return self.data().state;
    }

    /// The framework-wide unique id: 0 for root, -1 after disposal.
    pub fn uid(self: Fiber) i64 {
        return self.data().uid;
    }

    /// The context owned by this fiber.
    pub fn context(self: Fiber) *Context {
        return self.data().ctx;
    }

    /// The plugin name, resolved through the parent chain like upstream.
    pub fn name(self: Fiber) []const u8 {
        const d = self.data();
        if (d.plugin) |p| {
            if (p.name.len > 0) return p.name;
            return (Fiber{ .core = self.core, .id = d.parent.fiber }).name();
        }
        return "root";
    }

    /// Permanently dispose the fiber. Idempotent. Disposing the root fiber
    /// restarts it instead, mirroring upstream.
    pub fn dispose(self: Fiber) void {
        self.core.enter();
        defer self.core.leave();
        const d = self.data();
        if (d.plugin == null) {
            self.core.restartRoot();
            return;
        }
        if (d.disposed) return;
        d.disposed = true;
        self.core.removeFromRuntime(self.id);
        self.core.queue(self.id) catch @panic("cordis: out of memory in dispatch");
    }

    /// Unload and reload the fiber with its current config.
    pub fn restart(self: Fiber) Error!void {
        self.core.enter();
        defer self.core.leave();
        const d = self.data();
        if (d.disposed) return Error.InactiveEffect;
        d.restart_requested = true;
        try self.core.queue(self.id);
    }

    /// Replace the fiber's config and restart it.
    pub fn update(self: Fiber, config: ?Value) Error!void {
        self.core.enter();
        defer self.core.leave();
        const d = self.data();
        if (d.disposed) return Error.InactiveEffect;
        d.config = config;
        d.restart_requested = true;
        try self.core.queue(self.id);
    }
};

/// The mutable state shared by every context of one tree.
pub const Core = struct {
    gpa: Allocator,
    arena: std.heap.ArenaAllocator,
    hooks: std.StringHashMap(std.ArrayList(Hook)),
    store: std.AutoHashMap(u64, Impl),
    keys: std.StringHashMap(u64),
    labels: std.HashMap(PairKey, u64, PairContext, 80),
    last_key: u64,
    fibers: std.ArrayList(?*FiberData),
    runtimes: std.AutoHashMap(usize, std.ArrayList(usize)),
    counter: i64,
    depth: usize,
    draining: bool,
    dirty: std.ArrayList(usize),
    errors: std.ArrayList([]const u8),

    const Impl = struct { fiber: usize, val: Value };

    /// A (name, label) shared-isolate identity, hashed by content so no
    /// string formatting can ever make two distinct pairs collide.
    const PairKey = struct { name: []const u8, label: []const u8 };
    const PairContext = struct {
        pub fn hash(_: PairContext, k: PairKey) u64 {
            var h = std.hash.Wyhash.init(0);
            h.update(k.name);
            h.update(&.{0});
            h.update(k.label);
            return h.final();
        }
        pub fn eql(_: PairContext, x: PairKey, y: PairKey) bool {
            return std.mem.eql(u8, x.name, y.name) and std.mem.eql(u8, x.label, y.label);
        }
    };

    /// The arena allocator owning every context, fiber and plugin of this
    /// tree. Exposed for plugin code that needs tree-lifetime allocations.
    pub fn a(self: *Core) Allocator {
        return self.arena.allocator();
    }

    fn enter(self: *Core) void {
        self.depth += 1;
    }

    fn leave(self: *Core) void {
        self.depth -= 1;
        if (self.depth != 0 or self.draining) return;
        self.draining = true;
        defer self.draining = false;
        while (self.dirty.items.len > 0) {
            const id = self.dirty.orderedRemove(0);
            const f = self.fibers.items[id].?;
            f.queued = false;
            self.transition(id);
        }
    }

    fn queue(self: *Core, id: usize) Error!void {
        const f = self.fibers.items[id].?;
        if (f.queued) return;
        self.dirty.append(self.gpa, id) catch return Error.OutOfMemory;
        f.queued = true;
    }

    fn nextUid(self: *Core) i64 {
        self.counter += 1;
        return self.counter;
    }

    fn rootKey(self: *Core, name: []const u8) Error!u64 {
        if (self.keys.get(name)) |key| return key;
        self.last_key += 1;
        const owned = try self.a().dupe(u8, name);
        self.keys.put(owned, self.last_key) catch return Error.OutOfMemory;
        return self.last_key;
    }

    fn freshKey(self: *Core) u64 {
        self.last_key += 1;
        return self.last_key;
    }

    fn sharedKey(self: *Core, name: []const u8, label: []const u8) Error!u64 {
        const key = PairKey{ .name = name, .label = label };
        if (self.labels.get(key)) |k| return k;
        self.last_key += 1;
        const owned = PairKey{
            .name = try self.a().dupe(u8, name),
            .label = try self.a().dupe(u8, label),
        };
        self.labels.put(owned, self.last_key) catch return Error.OutOfMemory;
        return self.last_key;
    }

    fn removeFromRuntime(self: *Core, id: usize) void {
        const d = self.fibers.items[id].?;
        const plugin = d.plugin orelse return;
        const key = @intFromPtr(plugin);
        const list = self.runtimes.getPtr(key) orelse return;
        for (list.items, 0..) |fid, i| {
            if (fid == id) {
                _ = list.orderedRemove(i);
                break;
            }
        }
        if (list.items.len == 0) {
            list.deinit(self.gpa);
            _ = self.runtimes.remove(key);
        }
    }

    /// Queue every fiber injecting `name` in the realm of `from`,
    /// mirroring ReflectService.notify upstream.
    fn notifyDependents(self: *Core, from: *Context, name: []const u8) Error!void {
        const from_key = try from.isolateKeyE(name);
        for (self.fibers.items, 0..) |slot, i| {
            const f = slot orelse continue;
            if (f.plugin == null) continue;
            var matches = false;
            for (f.inject) |dep| {
                if (std.mem.eql(u8, dep, name)) {
                    matches = true;
                    break;
                }
            }
            if (!matches) continue;
            const key = try f.ctx.isolateKeyE(name);
            if (key == from_key) try self.queue(i);
        }
    }

    /// Best-effort error log: an allocation failure drops the line instead
    /// of aborting the drain that is already reporting a plugin failure.
    fn logError(self: *Core, name: []const u8, message: []const u8) void {
        const line = std.fmt.allocPrint(self.a(), "<{s}> {s}", .{ name, message }) catch return;
        self.errors.append(self.gpa, line) catch return;
    }

    fn transition(self: *Core, id: usize) void {
        const f = self.fibers.items[id].?;
        if (f.executing) return;
        const restart = f.restart_requested;
        f.restart_requested = false;
        const disposed = f.disposed;
        const state = f.state;
        const want_active = !disposed and self.depsReady(id);

        if (disposed and state != .active) {
            f.state = .disposed;
            f.uid = -1;
            return;
        }
        if (state == .active and (restart or !want_active)) {
            f.executing = true;
            f.state = .unloading;
            self.unload(id);
            if (want_active) self.load(id);
            self.finish(id, want_active);
            return;
        }
        if ((state == .pending or state == .failed) and want_active) {
            f.executing = true;
            self.load(id);
            self.finish(id, true);
            return;
        }
    }

    fn finish(self: *Core, id: usize, activated: bool) void {
        const f = self.fibers.items[id].?;
        f.executing = false;
        if (f.disposed) {
            f.state = .disposed;
            f.uid = -1;
        } else if (f.state != .failed) {
            f.state = if (activated) .active else .pending;
        }
    }

    fn depsReady(self: *Core, id: usize) bool {
        const f = self.fibers.items[id].?;
        if (f.disposed) return false;
        for (f.inject) |name| {
            const key = f.ctx.isolateKey(name);
            const imp = self.store.get(key) orelse return false;
            const provider = self.fibers.items[imp.fiber].?;
            if (provider.state != .active) return false;
        }
        return true;
    }

    fn unload(self: *Core, id: usize) void {
        const f = self.fibers.items[id].?;
        var bag = f.bag orelse return;
        f.bag = null;
        self.runBag(&bag);
        bag.deinit(self.gpa);
    }

    fn runBag(self: *Core, bag: *Bag) void {
        var i = bag.items.len;
        while (i > 0) {
            i -= 1;
            // Iterate by pointer: the done flag must persist on the stored
            // entry so deinit never double-runs a cleanup.
            const entry = &bag.items[i];
            if (entry.node) |node| {
                self.disposeNode(node);
                continue;
            }
            if (entry.cleanup) |cleanup| {
                if (cleanup.done) continue;
                cleanup.done = true;
                self.enter();
                cleanup.call(cleanup.ctx);
                self.leave();
            }
        }
    }

    /// Dispose one effect scope's registrations, last in, first out.
    /// Idempotent; safe to call again during a parent rollback.
    fn disposeNode(self: *Core, node: *EffectNode) void {
        if (!node.disposed) {
            node.disposed = true;
            self.runBag(&node.entries);
        }
        node.entries.deinit(self.gpa);
        node.entries = .empty;
    }

    fn metaFromBag(self: *Core, bag: *Bag) []const EffectMeta {
        var list: std.ArrayList(EffectMeta) = .empty;
        for (bag.items) |*entry| {
            if (entry.node) |node| {
                const children = self.metaFromBag(&node.entries);
                list.append(self.a(), .{
                    .label = node.label,
                    .disposed = node.disposed,
                    .children = children,
                }) catch @panic("cordis: out of memory in dispatch");
            } else if (entry.cleanup) |cleanup| {
                list.append(self.a(), .{
                    .label = entry.label,
                    .disposed = cleanup.done,
                    .children = &.{},
                }) catch @panic("cordis: out of memory in dispatch");
            }
        }
        return list.toOwnedSlice(self.a()) catch @panic("cordis: out of memory in dispatch");
    }

    /// Allocate a shared cleanup on the arena so a Disposer can mark it
    /// done before the owning bag runs.
    fn bindCleanup(self: *Core, comptime T: type, data: *T, comptime f: *const fn (*T) void) Error!*Cleanup {
        const c = try self.a().create(Cleanup);
        c.* = Cleanup.bind(T, data, f);
        return c;
    }

    fn load(self: *Core, id: usize) void {
        const f = self.fibers.items[id].?;
        f.bag = Bag.empty;
        f.state = .loading;
        const plugin = f.plugin.?;
        plugin.apply(plugin, f.ctx, f.config) catch |err| {
            self.unload(id);
            self.logError(plugin.name, @errorName(err));
            f.state = .failed;
        };
    }

    fn restartRoot(self: *Core) void {
        self.unload(0);
        const f = self.fibers.items[0].?;
        f.bag = Bag.empty;
    }
};

/// A scope in the context tree. Arena allocated; the whole tree is freed by
/// deinit on the root.
pub const Context = struct {
    core: *Core,
    parent: ?*Context,
    fiber: usize,
    realm: ?struct { name: []const u8, key: u64 },
    filter: ?Filter,
    collect: ?*Bag,

    /// Create a root context with its own registry, event bus and service
    /// store. The root fiber is always active. Free with deinit.
    pub fn init(gpa: Allocator) !*Context {
        const core = try gpa.create(Core);
        errdefer gpa.destroy(core);
        core.* = .{
            .gpa = gpa,
            .arena = std.heap.ArenaAllocator.init(gpa),
            .hooks = std.StringHashMap(std.ArrayList(Hook)).init(gpa),
            .store = std.AutoHashMap(u64, Core.Impl).init(gpa),
            .keys = std.StringHashMap(u64).init(gpa),
            .labels = std.HashMap(Core.PairKey, u64, Core.PairContext, 80).init(gpa),
            .last_key = 0,
            .fibers = .empty,
            .runtimes = std.AutoHashMap(usize, std.ArrayList(usize)).init(gpa),
            .counter = 0,
            .depth = 0,
            .draining = false,
            .dirty = .empty,
            .errors = .empty,
        };
        errdefer core.arena.deinit();
        const root = try core.a().create(Context);
        root.* = .{ .core = core, .parent = null, .fiber = 0, .realm = null, .filter = null, .collect = null };
        const root_fiber = try core.a().create(FiberData);
        root_fiber.* = .{
            .uid = 0,
            .ctx = root,
            .parent = root,
            .config = null,
            .inject = &.{},
            .plugin = null,
            .state = .active,
            .disposed = false,
            .restart_requested = false,
            .queued = false,
            .executing = false,
            .bag = Bag.empty,
        };
        try core.fibers.append(gpa, root_fiber);
        return root;
    }

    /// Free the whole context tree, running any cleanups still registered.
    pub fn deinit(self: *Context) void {
        const core = self.core;
        const gpa = core.gpa;
        // Roll back everything still live, root last.
        for (core.fibers.items) |slot| {
            if (slot) |f| {
                if (f.bag) |*bag| {
                    core.runBag(bag);
                    bag.deinit(gpa);
                }
            }
        }
        var hook_it = core.hooks.iterator();
        while (hook_it.next()) |e| {
            e.value_ptr.deinit(gpa);
        }
        core.hooks.deinit();
        core.store.deinit();
        core.keys.deinit();
        core.labels.deinit();
        var rt_it = core.runtimes.iterator();
        while (rt_it.next()) |e| e.value_ptr.deinit(gpa);
        core.runtimes.deinit();
        core.dirty.deinit(gpa);
        core.errors.deinit(gpa);
        core.fibers.deinit(gpa);
        core.arena.deinit();
        gpa.destroy(core);
    }

    /// A plain child scope, mirroring ctx.extend() upstream.
    pub fn extend(self: *Context) Error!*Context {
        const child = self.core.a().create(Context) catch return error.OutOfMemory;
        child.* = .{ .core = self.core, .parent = self, .fiber = self.fiber, .realm = null, .filter = null, .collect = null };
        return child;
    }

    /// A child scope with its own service realm for `name`.
    pub fn isolate(self: *Context, name: []const u8) Error!*Context {
        const child = try self.extend();
        child.realm = .{ .name = name, .key = self.core.freshKey() };
        return child;
    }

    /// A child scope sharing a realm with every other scope created with
    /// the same label, mirroring ctx.isolate(name, label).
    pub fn isolateShared(self: *Context, name: []const u8, label: []const u8) Error!*Context {
        const child = try self.extend();
        const key = try self.core.sharedKey(name, label);
        child.realm = .{ .name = name, .key = key };
        return child;
    }

    /// A child scope with an event emission filter.
    pub fn withFilter(self: *Context, filter: Filter) Error!*Context {
        const child = try self.extend();
        child.filter = filter;
        return child;
    }

    /// A realm filter for `name`: matches listeners in the same realm as
    /// `realm_ctx`. Works with runtime event names; the filter state lives
    /// in the tree's arena.
    pub fn realmFilter(self: *Context, realm_ctx: *Context, name: []const u8) Error!Filter {
        const Holder = struct {
            realm_ctx: *Context,
            name: []const u8,

            fn call(h: *@This(), _: *const Context, listener_owner: *const Context) bool {
                return listener_owner.isolateKey(h.name) == h.realm_ctx.isolateKey(h.name);
            }
        };
        const holder = try self.core.a().create(Holder);
        const owned = try self.core.a().dupe(u8, name);
        holder.* = .{ .realm_ctx = realm_ctx, .name = owned };
        return Filter.bind(Holder, holder, Holder.call);
    }

    /// Resolve the realm key of `name` through the scope chain. The lazy
    /// root-realm assignment allocates, and this query has no error
    /// channel, so an allocation failure aborts; fallible callers use
    /// `isolateKeyE` instead.
    pub fn isolateKey(self: *const Context, name: []const u8) u64 {
        return self.isolateKeyE(name) catch @panic("cordis: out of memory in dispatch");
    }

    fn isolateKeyE(self: *const Context, name: []const u8) Error!u64 {
        var ctx: ?*const Context = self;
        while (ctx) |c| : (ctx = c.parent) {
            if (c.realm) |iso| {
                if (std.mem.eql(u8, iso.name, name)) return iso.key;
            }
        }
        // Const cast is safe: rootKey only appends lazily assigned keys.
        return @constCast(self.core).rootKey(name);
    }

    /// The fiber owning this context.
    pub fn fiberHandle(self: *Context) Fiber {
        return .{ .core = self.core, .id = self.fiber };
    }

    fn currentBag(self: *Context) ?*Bag {
        if (self.collect) |bag| return bag;
        const f = self.core.fibers.items[self.fiber].?;
        if (f.disposed) return null;
        switch (f.state) {
            .active, .loading => {},
            else => return null,
        }
        if (f.bag) |*bag| return bag;
        return null;
    }

    fn assertActive(self: *Context) Error!void {
        if (self.core.fibers.items[self.fiber].?.disposed) return Error.InactiveEffect;
    }

    /// Subscribe to the string event `name`. Prefer `onTyped` for
    /// application events; string names remain for the framework's internal
    /// namespace. The subscription is bound to this context's fiber and
    /// rolls back with it.
    pub fn onNamed(self: *Context, name: []const u8, listener: Listener) Error!Disposer {
        self.core.enter();
        defer self.core.leave();
        try self.assertActive();
        const bag = self.currentBag() orelse return Error.InactiveEffect;

        const list = blk: {
            const result = self.core.hooks.getOrPut(name) catch return error.OutOfMemory;
            if (!result.found_existing) {
                result.key_ptr.* = self.core.a().dupe(u8, name) catch return error.OutOfMemory;
                result.value_ptr.* = .empty;
            }
            break :blk result.value_ptr;
        };
        list.append(self.core.gpa, .{ .owner = self, .listener = listener, .global = false }) catch return error.OutOfMemory;

        const removal = self.core.a().create(struct {
            list: *std.ArrayList(Hook),
            listener: Listener,
        }) catch return error.OutOfMemory;
        const Removal = @TypeOf(removal.*);
        removal.* = .{ .list = list, .listener = listener };
        const cleanup = try self.core.bindCleanup(Removal, removal, struct {
            fn run(r: *Removal) void {
                for (r.list.items, 0..) |h, i| {
                    if (h.listener.ctx == r.listener.ctx and h.listener.call == r.listener.call) {
                        _ = r.list.orderedRemove(i);
                        return;
                    }
                }
            }
        }.run);
        bag.append(self.core.gpa, .{ .label = "ctx.on()", .cleanup = cleanup }) catch return error.OutOfMemory;
        return .{ .cleanup = cleanup, .core = self.core };
    }

    /// Subscribe to `name` as a global listener: exempt from every emission
    /// filter, mirroring the Global option of the Go and Rust ports.
    pub fn onGlobal(self: *Context, name: []const u8, listener: Listener) Error!Disposer {
        self.core.enter();
        defer self.core.leave();
        try self.assertActive();
        const bag = self.currentBag() orelse return Error.InactiveEffect;

        const list = blk: {
            const result = self.core.hooks.getOrPut(name) catch return error.OutOfMemory;
            if (!result.found_existing) {
                result.key_ptr.* = self.core.a().dupe(u8, name) catch return error.OutOfMemory;
                result.value_ptr.* = .empty;
            }
            break :blk result.value_ptr;
        };
        list.append(self.core.gpa, .{ .owner = self, .listener = listener, .global = true }) catch return error.OutOfMemory;

        const removal = self.core.a().create(struct {
            list: *std.ArrayList(Hook),
            listener: Listener,
        }) catch return error.OutOfMemory;
        const Removal = @TypeOf(removal.*);
        removal.* = .{ .list = list, .listener = listener };
        const cleanup = try self.core.bindCleanup(Removal, removal, struct {
            fn run(r: *Removal) void {
                for (r.list.items, 0..) |h, i| {
                    if (h.listener.ctx == r.listener.ctx and h.listener.call == r.listener.call) {
                        _ = r.list.orderedRemove(i);
                        return;
                    }
                }
            }
        }.run);
        bag.append(self.core.gpa, .{ .label = "ctx.onGlobal()", .cleanup = cleanup }) catch return error.OutOfMemory;
        return .{ .cleanup = cleanup, .core = self.core };
    }

    fn visible(self: *Context, hook: Hook) bool {
        if (hook.global) return true;
        const filter = self.filter orelse return true;
        return filter.call(filter.ctx, self, hook.owner);
    }

    /// Attach a labeled cleanup to the current effect scope: the enclosing
    /// effect body while one runs, otherwise the fiber itself. Cleanups run
    /// on rollback, last in, first out. `data` must outlive the
    /// registration (static or arena allocated) and is passed to `f`
    /// unchanged.
    pub fn attach(self: *Context, data: anytype, comptime f: *const fn (@TypeOf(data)) void) Error!void {
        self.core.enter();
        defer self.core.leave();
        try self.assertActive();
        const bag = self.currentBag() orelse return Error.InactiveEffect;
        const Data = @TypeOf(data);
        if (@typeInfo(Data) != .pointer) @compileError("cordis.attach expects a pointer to the cleanup data, got " ++ @typeName(Data));
        const Child = @typeInfo(Data).pointer.child;
        bag.append(self.core.gpa, .{
            .label = "ctx.attach()",
            .cleanup = try self.core.bindCleanup(Child, data, f),
        }) catch return error.OutOfMemory;
    }

    /// Run `f` inside a named effect scope: registrations made through the
    /// sub-context passed to `f` collect into the scope and roll back
    /// together, last in, first out, on error or early disposal.
    pub fn effect(self: *Context, label: []const u8, data: anytype, comptime f: *const fn (*Context, @TypeOf(data)) Error!void) Error!Effect {
        self.core.enter();
        defer self.core.leave();
        try self.assertActive();
        const parent = self.currentBag() orelse return Error.InactiveEffect;
        const node = self.core.a().create(EffectNode) catch return error.OutOfMemory;
        node.* = .{ .label = self.core.a().dupe(u8, label) catch return error.OutOfMemory };
        parent.append(self.core.gpa, .{ .label = node.label, .node = node }) catch return error.OutOfMemory;
        const sub = self.core.a().create(Context) catch return error.OutOfMemory;
        sub.* = self.*;
        sub.collect = &node.entries;
        f(sub, data) catch |err| {
            self.core.disposeNode(node);
            return err;
        };
        return .{ .node = node, .core = self.core };
    }

    /// A labeled introspection snapshot of this fiber's effect scopes,
    /// outermost first.
    pub fn effects(self: *Context) []const EffectMeta {
        const bag = self.currentBag() orelse return &.{};
        return self.core.metaFromBag(bag);
    }

    /// The plugin registry view of this context tree.
    pub fn registry(self: *Context) Registry {
        return .{ .core = self.core };
    }

    /// Subscribe to the event type E for exactly one delivery: after the
    /// first matching emission the subscription removes itself. Rolls back
    /// with the owning scope like `onTyped`.
    pub fn onceTyped(self: *Context, comptime E: type, data: anytype, comptime f: *const fn (@TypeOf(data), E) void) Error!void {
        self.core.enter();
        defer self.core.leave();
        try self.assertActive();
        const bag = self.currentBag() orelse return Error.InactiveEffect;
        const Data = @TypeOf(data);
        if (@typeInfo(Data) != .pointer) @compileError("cordis.onceTyped expects a pointer to the listener data, got " ++ @typeName(Data));

        const Holder = struct {
            list: *std.ArrayList(Hook),
            listener: Listener,
            data: Data,
            fired: bool = false,

            fn call(raw: *anyopaque, args: []const Value) ?Value {
                const h: *@This() = @ptrCast(@alignCast(raw));
                if (h.fired) return null;
                h.fired = true;
                for (h.list.items, 0..) |hook, i| {
                    if (hook.listener.ctx == h.listener.ctx and hook.listener.call == h.listener.call) {
                        _ = h.list.orderedRemove(i);
                        break;
                    }
                }
                const event: *const E = @ptrCast(@alignCast(args[0]));
                f(h.data, event.*);
                return null;
            }
        };

        const name = @typeName(E);
        const list = blk: {
            const result = self.core.hooks.getOrPut(name) catch return error.OutOfMemory;
            if (!result.found_existing) {
                result.key_ptr.* = self.core.a().dupe(u8, name) catch return error.OutOfMemory;
                result.value_ptr.* = .empty;
            }
            break :blk result.value_ptr;
        };
        const holder = self.core.a().create(Holder) catch return error.OutOfMemory;
        const listener = Listener{ .ctx = @ptrCast(holder), .call = &Holder.call };
        holder.* = .{ .list = list, .listener = listener, .data = data };
        list.append(self.core.gpa, .{ .owner = self, .listener = listener, .global = false }) catch return error.OutOfMemory;

        const removal = self.core.a().create(struct {
            list: *std.ArrayList(Hook),
            listener: Listener,
        }) catch return error.OutOfMemory;
        const Removal = @TypeOf(removal.*);
        removal.* = .{ .list = list, .listener = listener };
        bag.append(self.core.gpa, .{
            .label = "ctx.once()",
            .cleanup = try self.core.bindCleanup(Removal, removal, struct {
                fn run(r: *Removal) void {
                    for (r.list.items, 0..) |h, i| {
                        if (h.listener.ctx == r.listener.ctx and h.listener.call == r.listener.call) {
                            _ = r.list.orderedRemove(i);
                            return;
                        }
                    }
                }
            }.run),
        }) catch return error.OutOfMemory;
    }

    /// Deliver the string event `name` synchronously to every matching
    /// listener in registration order.
    pub fn emitNamed(self: *Context, name: []const u8, args: []const Value) void {
        const list = self.core.hooks.getPtr(name) orelse return;
        var snapshot: std.ArrayList(Hook) = .empty;
        defer snapshot.deinit(self.core.gpa);
        snapshot.appendSlice(self.core.gpa, list.items) catch @panic("cordis: out of memory in dispatch");
        for (snapshot.items) |hook| {
            if (!self.visible(hook)) continue;
            _ = hook.listener.call(hook.listener.ctx, args);
        }
    }

    /// Return the first non-null listener result, mirroring ctx.bail
    /// upstream.
    pub fn bail(self: *Context, name: []const u8, args: []const Value) ?Value {
        const list = self.core.hooks.getPtr(name) orelse return null;
        var snapshot: std.ArrayList(Hook) = .empty;
        defer snapshot.deinit(self.core.gpa);
        snapshot.appendSlice(self.core.gpa, list.items) catch @panic("cordis: out of memory in dispatch");
        for (snapshot.items) |hook| {
            if (!self.visible(hook)) continue;
            if (hook.listener.call(hook.listener.ctx, args)) |result| return result;
        }
        return null;
    }

    /// The serial dispatch mode: listeners run one at a time and the first
    /// non-null result stops the chain, mirroring ctx.serial upstream.
    /// Identical to `bail`, which is the synchronous counterpart.
    pub fn serial(self: *Context, name: []const u8, args: []const Value) ?Value {
        return self.bail(name, args);
    }

    /// A chain continuation handed to waterfall listeners: the listener
    /// calls `invoke` with the transformed arguments to run the rest of the
    /// chain; not calling it short-circuits the composition.
    pub const Next = struct {
        ctx: *Context,
        name: []const u8,
        hooks: []Hook,
        index: usize,
        terminal: *const fn (args: []const Value) ?Value,

        /// Run the remaining chain over `args`, falling through to the
        /// terminal function when every listener has run.
        pub fn invoke(self: *Next, args: []const Value) ?Value {
            if (self.index >= self.hooks.len) {
                return self.terminal(args);
            }
            return self.ctx.waterfallStep(self, args);
        }
    };

    fn waterfallStep(self: *Context, next: *Next, args: []const Value) ?Value {
        const hook = next.hooks[next.index];
        const sub = self.core.a().create(Next) catch @panic("cordis: out of memory in dispatch");
        sub.* = .{ .ctx = next.ctx, .name = next.name, .hooks = next.hooks, .index = next.index + 1, .terminal = next.terminal };
        var full: std.ArrayListUnmanaged(Value) = .empty;
        defer full.deinit(self.core.gpa);
        full.appendSlice(self.core.gpa, args) catch @panic("cordis: out of memory in dispatch");
        full.append(self.core.gpa, @ptrCast(sub)) catch @panic("cordis: out of memory in dispatch");
        if (!self.visible(hook)) {
            return sub.invoke(args);
        }
        return hook.listener.call(hook.listener.ctx, full.items);
    }

    /// The waterfall dispatch mode: every listener receives the arguments
    /// followed by a `*Next`; calling `Next.invoke` runs the rest of the
    /// chain over (possibly transformed) arguments. A listener that does
    /// not invoke next short-circuits, mirroring ctx.waterfall upstream.
    /// When no listener is registered the terminal runs unchanged.
    pub fn waterfall(self: *Context, name: []const u8, args: []const Value, terminal: *const fn (args: []const Value) ?Value) ?Value {
        const list = self.core.hooks.getPtr(name) orelse return terminal(args);
        var hooks: std.ArrayList(Hook) = .empty;
        defer hooks.deinit(self.core.gpa);
        for (list.items) |hook| {
            if (self.visible(hook)) hooks.append(self.core.gpa, hook) catch @panic("cordis: out of memory in dispatch");
        }
        if (hooks.items.len == 0) return terminal(args);
        const owned = self.core.a().dupe(Hook, hooks.items) catch @panic("cordis: out of memory in dispatch");
        const next = self.core.a().create(Next) catch @panic("cordis: out of memory in dispatch");
        next.* = .{ .ctx = self, .name = name, .hooks = owned, .index = 0, .terminal = terminal };
        return self.waterfallStep(next, args);
    }

    /// The parallel dispatch mode. This tree is single-threaded, so all
    /// matching listeners run synchronously in registration order; the
    /// mode exists for API parity with the Go and Rust ports.
    pub fn parallel(self: *Context, name: []const u8, args: []const Value) void {
        self.emitNamed(name, args);
    }

    /// Run `f` as one framework transaction: fiber transitions triggered
    /// inside are coalesced and settle after `f` returns.
    pub fn batch(self: *Context, data: anytype, comptime f: *const fn (@TypeOf(data)) void) void {
        self.core.enter();
        defer self.core.leave();
        f(data);
    }

    /// Publish `val` under the string `name` in this context's service
    /// realm. Prefer `provide` (typed); string names remain for dynamic
    /// service names. Bound to the context's fiber and withdrawn
    /// automatically when it unloads.
    pub fn provideNamed(self: *Context, name: []const u8, val: Value) Error!Disposer {
        self.core.enter();
        defer self.core.leave();
        try self.assertActive();
        const bag = self.currentBag() orelse return Error.InactiveEffect;
        const key = self.isolateKey(name);
        if (self.core.store.contains(key)) return Error.DuplicateService;
        self.core.store.put(key, .{ .fiber = self.fiber, .val = val }) catch return error.OutOfMemory;

        const removal = self.core.a().create(struct {
            core: *Core,
            ctx: *Context,
            key: u64,
            name: []const u8,
        }) catch return error.OutOfMemory;
        const Removal = @TypeOf(removal.*);
        removal.* = .{ .core = self.core, .ctx = self, .key = key, .name = name };
        const cleanup = try self.core.bindCleanup(Removal, removal, struct {
            fn run(r: *Removal) void {
                _ = r.core.store.remove(r.key);
                r.core.notifyDependents(r.ctx, r.name) catch @panic("cordis: out of memory in dispatch");
            }
        }.run);
        bag.append(self.core.gpa, .{ .label = "ctx.provide()", .cleanup = cleanup }) catch return error.OutOfMemory;

        try self.core.notifyDependents(self, name);
        return .{ .cleanup = cleanup, .core = self.core };
    }

    /// The service published under the string `name` in this context's
    /// realm, when its provider is active.
    pub fn getNamed(self: *Context, name: []const u8) ?Value {
        const key = self.isolateKey(name);
        const imp = self.core.store.get(key) orelse return null;
        const provider = self.core.fibers.items[imp.fiber].?;
        if (provider.state != .active) return null;
        return imp.val;
    }

    /// The typed variant of getNamed.
    pub fn getTypedNamed(self: *Context, comptime T: type, name: []const u8) ?*const T {
        const v = self.getNamed(name) orelse return null;
        return @ptrCast(@alignCast(v));
    }

    /// Publish `ptr`'s pointee as the service identified by its type T in
    /// this context's realm. The service name is the type identity, so
    /// providers and consumers cannot drift apart on a hand written string;
    /// the value must outlive the registration (static or arena allocated).
    /// Isolate the typed service with `isolate(@typeName(T))`.
    pub fn provide(self: *Context, ptr: anytype) Error!Disposer {
        const P = @TypeOf(ptr);
        if (@typeInfo(P) != .pointer) @compileError("cordis.provide expects a pointer, got " ++ @typeName(P));
        const T = @typeInfo(P).pointer.child;
        return self.provideNamed(@typeName(T), value(ptr));
    }

    /// The service of type T published in this context's realm, when its
    /// provider is active.
    pub fn getTyped(self: *Context, comptime T: type) ?*const T {
        return self.getTypedNamed(T, @typeName(T));
    }

    /// Subscribe to the event type E. Typed events are the primary event
    /// API: the event name derives from the type, so emitters and listeners
    /// cannot drift apart on a hand written string, and the payload arrives
    /// fully typed. `data` is passed to `f` unchanged and must outlive the
    /// subscription; the subscription is bound to this context's fiber and
    /// rolls back with it.
    pub fn onTyped(self: *Context, comptime E: type, data: anytype, comptime f: *const fn (@TypeOf(data), E) void) Error!Disposer {
        const Data = @TypeOf(data);
        if (@typeInfo(Data) != .pointer) @compileError("cordis.onTyped expects a pointer to the listener data, got " ++ @typeName(Data));
        const wrapper = struct {
            fn call(raw: *anyopaque, args: []const Value) ?Value {
                const d: Data = @ptrCast(@alignCast(raw));
                const event: *const E = @ptrCast(@alignCast(args[0]));
                f(d, event.*);
                return null;
            }
        };
        return self.onNamed(@typeName(E), .{ .ctx = @ptrCast(data), .call = &wrapper.call });
    }

    /// Deliver `event` synchronously to every listener registered for its
    /// type E, in registration order, applying this context's emission
    /// filter.
    pub fn emitTyped(self: *Context, comptime E: type, event: *const E) void {
        self.emitNamed(@typeName(E), &.{value(event)});
    }

    /// Start an anonymous plugin that runs `apply` once every service in
    /// `deps` is available, mirroring ctx.inject upstream.
    pub fn injectPlugin(self: *Context, name: []const u8, deps: []const []const u8, apply: ApplyFn) Error!Fiber {
        const plugin = self.core.a().create(Plugin) catch return error.OutOfMemory;
        plugin.* = .{ .name = name, .inject = deps, .apply = apply };
        return startPlugin(self, plugin, null);
    }

    /// Errors reported by failing plugin bodies.
    pub fn loggedErrors(self: *Context) [][]const u8 {
        return self.core.errors.items;
    }
};

fn startPlugin(ctx: *Context, plugin: *const Plugin, config: ?Value) Error!Fiber {
    const core = ctx.core;
    core.enter();
    defer core.leave();
    try ctx.assertActive();
    const parent_bag = ctx.currentBag() orelse return Error.InactiveEffect;

    const fiber_ctx = core.a().create(Context) catch return error.OutOfMemory;
    fiber_ctx.* = .{ .core = core, .parent = ctx, .fiber = undefined, .realm = null, .filter = null, .collect = null };
    const data = core.a().create(FiberData) catch return error.OutOfMemory;
    data.* = .{
        .uid = core.nextUid(),
        .ctx = fiber_ctx,
        .parent = ctx,
        .config = config,
        .inject = plugin.inject,
        .plugin = plugin,
        .state = .pending,
        .disposed = false,
        .restart_requested = false,
        .queued = false,
        .executing = false,
        .bag = null,
    };
    const id = core.fibers.items.len;
    core.fibers.append(core.gpa, data) catch return error.OutOfMemory;
    fiber_ctx.fiber = id;
    const fiber = Fiber{ .core = core, .id = id };

    // Register the fiber's disposal on the parent fiber's effect bag so
    // parent rollback cascades to child plugins.
    const holder = core.a().create(Fiber) catch return error.OutOfMemory;
    holder.* = fiber;
    parent_bag.append(core.gpa, .{
        .label = "ctx.plugin()",
        .cleanup = try core.bindCleanup(Fiber, holder, struct {
            fn run(f: *Fiber) void {
                f.dispose();
            }
        }.run),
    }) catch return error.OutOfMemory;

    const key = @intFromPtr(plugin);
    const result = core.runtimes.getOrPut(key) catch return error.OutOfMemory;
    if (!result.found_existing) result.value_ptr.* = .empty;
    result.value_ptr.append(core.gpa, id) catch return error.OutOfMemory;

    try core.queue(id);
    return fiber;
}
