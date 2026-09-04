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

/// Errors surfaced by the framework.
pub const Error = error{
    /// An effect, listener, service or plugin was registered on a context
    /// whose fiber is no longer active.
    InactiveEffect,
    /// A service was provided twice in the same realm.
    DuplicateService,
    /// A plugin body failed.
    PluginFailed,
    /// Out of memory.
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
/// mirroring Context.filter upstream.
pub const FilterFn = *const fn (emitter: *const Context, listener_owner: *const Context) bool;

const Hook = struct {
    owner: *Context,
    listener: Listener,
    global: bool,
};

/// A plugin body.
pub const ApplyFn = *const fn (ctx: *Context, config: ?Value) Error!void;

/// A unit of composable behavior: a name, injected dependencies and an
/// apply function. The Plugin value address is its registry identity, so
/// plugins must have a stable address (static or arena allocated).
pub const Plugin = struct {
    name: []const u8,
    inject: []const []const u8 = &.{},
    apply: ApplyFn,

    /// Start this plugin on ctx with an optional config value.
    pub fn start(self: *const Plugin, ctx: *Context, config: ?Value) Error!Fiber {
        return startPlugin(ctx, self, config);
    }
};

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
    cleanup: ?Cleanup,
};

const Bag = std.ArrayList(Entry);

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
        self.core.queue(self.id);
    }

    /// Unload and reload the fiber with its current config.
    pub fn restart(self: Fiber) Error!void {
        self.core.enter();
        defer self.core.leave();
        const d = self.data();
        if (d.disposed) return Error.InactiveEffect;
        d.restart_requested = true;
        self.core.queue(self.id);
    }

    /// Replace the fiber's config and restart it.
    pub fn update(self: Fiber, config: ?Value) Error!void {
        self.core.enter();
        defer self.core.leave();
        const d = self.data();
        if (d.disposed) return Error.InactiveEffect;
        d.config = config;
        d.restart_requested = true;
        self.core.queue(self.id);
    }
};

/// The mutable state shared by every context of one tree.
pub const Core = struct {
    gpa: Allocator,
    arena: std.heap.ArenaAllocator,
    hooks: std.StringHashMap(std.ArrayList(Hook)),
    store: std.AutoHashMap(u64, Impl),
    keys: std.StringHashMap(u64),
    last_key: u64,
    fibers: std.ArrayList(?*FiberData),
    runtimes: std.AutoHashMap(usize, std.ArrayList(usize)),
    counter: i64,
    depth: usize,
    draining: bool,
    dirty: std.ArrayList(usize),
    errors: std.ArrayList([]const u8),

    const Impl = struct { fiber: usize, val: Value };

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

    fn queue(self: *Core, id: usize) void {
        const f = self.fibers.items[id].?;
        if (f.queued) return;
        f.queued = true;
        self.dirty.append(self.gpa, id) catch @panic("OOM");
    }

    fn nextUid(self: *Core) i64 {
        self.counter += 1;
        return self.counter;
    }

    fn rootKey(self: *Core, name: []const u8) u64 {
        if (self.keys.get(name)) |key| return key;
        self.last_key += 1;
        const owned = self.a().dupe(u8, name) catch @panic("OOM");
        self.keys.put(owned, self.last_key) catch @panic("OOM");
        return self.last_key;
    }

    fn freshKey(self: *Core) u64 {
        self.last_key += 1;
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
    fn notifyDependents(self: *Core, from: *Context, name: []const u8) void {
        const from_key = from.isolateKey(name);
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
            if (f.ctx.isolateKey(name) == from_key) self.queue(i);
        }
    }

    fn logError(self: *Core, name: []const u8, message: []const u8) void {
        const line = std.fmt.allocPrint(self.a(), "<{s}> {s}", .{ name, message }) catch @panic("OOM");
        self.errors.append(self.gpa, line) catch @panic("OOM");
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
            if (entry.cleanup) |*cleanup| {
                if (cleanup.done) continue;
                cleanup.done = true;
                self.enter();
                cleanup.call(cleanup.ctx);
                self.leave();
            }
        }
    }

    fn load(self: *Core, id: usize) void {
        const f = self.fibers.items[id].?;
        f.bag = Bag.empty;
        f.state = .loading;
        const plugin = f.plugin.?;
        plugin.apply(f.ctx, f.config) catch |err| {
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
    filter: ?FilterFn,
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
    pub fn extend(self: *Context) *Context {
        const child = self.core.a().create(Context) catch @panic("OOM");
        child.* = .{ .core = self.core, .parent = self, .fiber = self.fiber, .realm = null, .filter = null, .collect = null };
        return child;
    }

    /// A child scope with its own service realm for `name`.
    pub fn isolate(self: *Context, name: []const u8) *Context {
        const child = self.extend();
        child.realm = .{ .name = name, .key = self.core.freshKey() };
        return child;
    }

    /// A child scope sharing a realm with every other scope created with
    /// the same label, mirroring ctx.isolate(name, label).
    pub fn isolateShared(self: *Context, name: []const u8, label: []const u8) *Context {
        const synthetic = std.fmt.allocPrint(self.core.a(), "{s}\x00{s}", .{ name, label }) catch @panic("OOM");
        const child = self.extend();
        child.realm = .{ .name = name, .key = self.core.rootKey(synthetic) };
        return child;
    }

    /// A child scope with an event emission filter.
    pub fn withFilter(self: *Context, filter: FilterFn) *Context {
        const child = self.extend();
        child.filter = filter;
        return child;
    }

    /// A static realm filter for `name`: matches listeners in the same
    /// realm as `realm_ctx`. Bind with `realmFilter(&ctx, "foo")`.
    pub fn realmFilter(realm_ctx: *Context, comptime name: []const u8) FilterFn {
        _ = realm_ctx;
        return struct {
            fn filter(emitter: *const Context, listener_owner: *const Context) bool {
                return emitter.isolateKey(name) == listener_owner.isolateKey(name);
            }
        }.filter;
    }

    /// Resolve the realm key of `name` through the scope chain.
    pub fn isolateKey(self: *const Context, name: []const u8) u64 {
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

    /// Subscribe to `name`. The subscription is bound to this context's
    /// fiber and rolls back with it.
    pub fn on(self: *Context, name: []const u8, listener: Listener) Error!void {
        self.core.enter();
        defer self.core.leave();
        try self.assertActive();
        const bag = self.currentBag() orelse return Error.InactiveEffect;

        const list = blk: {
            const result = self.core.hooks.getOrPut(name) catch return Error.OutOfMemory;
            if (!result.found_existing) {
                result.key_ptr.* = self.core.a().dupe(u8, name) catch return Error.OutOfMemory;
                result.value_ptr.* = .empty;
            }
            break :blk result.value_ptr;
        };
        list.append(self.core.gpa, .{ .owner = self, .listener = listener, .global = false }) catch return Error.OutOfMemory;

        const removal = self.core.a().create(struct {
            list: *std.ArrayList(Hook),
            listener: Listener,
        }) catch return Error.OutOfMemory;
        const Removal = @TypeOf(removal.*);
        removal.* = .{ .list = list, .listener = listener };
        bag.append(self.core.gpa, .{
            .label = "ctx.on()",
            .cleanup = Cleanup.bind(Removal, removal, struct {
                fn run(r: *Removal) void {
                    for (r.list.items, 0..) |h, i| {
                        if (h.listener.ctx == r.listener.ctx and h.listener.call == r.listener.call) {
                            _ = r.list.orderedRemove(i);
                            return;
                        }
                    }
                }
            }.run),
        }) catch return Error.OutOfMemory;
    }

    fn visible(self: *Context, hook: Hook) bool {
        if (hook.global) return true;
        const filter = self.filter orelse return true;
        return filter(self, hook.owner);
    }

    /// Deliver `name` synchronously to every matching listener in
    /// registration order.
    pub fn emit(self: *Context, name: []const u8, args: []const Value) void {
        const list = self.core.hooks.getPtr(name) orelse return;
        var snapshot: std.ArrayList(Hook) = .empty;
        defer snapshot.deinit(self.core.gpa);
        snapshot.appendSlice(self.core.gpa, list.items) catch @panic("OOM");
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
        snapshot.appendSlice(self.core.gpa, list.items) catch @panic("OOM");
        for (snapshot.items) |hook| {
            if (!self.visible(hook)) continue;
            if (hook.listener.call(hook.listener.ctx, args)) |result| return result;
        }
        return null;
    }

    /// Publish `val` under `name` in this context's service realm. Bound to
    /// the context's fiber and withdrawn automatically when it unloads.
    pub fn provide(self: *Context, name: []const u8, val: Value) Error!void {
        self.core.enter();
        defer self.core.leave();
        try self.assertActive();
        const bag = self.currentBag() orelse return Error.InactiveEffect;
        const key = self.isolateKey(name);
        if (self.core.store.contains(key)) return Error.DuplicateService;
        self.core.store.put(key, .{ .fiber = self.fiber, .val = val }) catch return Error.OutOfMemory;

        const removal = self.core.a().create(struct {
            core: *Core,
            ctx: *Context,
            key: u64,
            name: []const u8,
        }) catch return Error.OutOfMemory;
        const Removal = @TypeOf(removal.*);
        removal.* = .{ .core = self.core, .ctx = self, .key = key, .name = name };
        bag.append(self.core.gpa, .{
            .label = "ctx.provide()",
            .cleanup = Cleanup.bind(Removal, removal, struct {
                fn run(r: *Removal) void {
                    _ = r.core.store.remove(r.key);
                    r.core.notifyDependents(r.ctx, r.name);
                }
            }.run),
        }) catch return Error.OutOfMemory;

        self.core.notifyDependents(self, name);
    }

    /// The service published under `name` in this context's realm, when its
    /// provider is active.
    pub fn get(self: *Context, name: []const u8) ?Value {
        const key = self.isolateKey(name);
        const imp = self.core.store.get(key) orelse return null;
        const provider = self.core.fibers.items[imp.fiber].?;
        if (provider.state != .active) return null;
        return imp.val;
    }

    /// The typed variant of get.
    pub fn getTyped(self: *Context, comptime T: type, name: []const u8) ?*const T {
        const v = self.get(name) orelse return null;
        return @ptrCast(@alignCast(v));
    }

    /// Start an anonymous plugin that runs `apply` once every service in
    /// `deps` is available, mirroring ctx.inject upstream.
    pub fn injectPlugin(self: *Context, name: []const u8, deps: []const []const u8, apply: ApplyFn) Error!Fiber {
        const plugin = self.core.a().create(Plugin) catch return Error.OutOfMemory;
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

    const fiber_ctx = core.a().create(Context) catch return Error.OutOfMemory;
    fiber_ctx.* = .{ .core = core, .parent = ctx, .fiber = undefined, .realm = null, .filter = null, .collect = null };
    const data = core.a().create(FiberData) catch return Error.OutOfMemory;
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
    core.fibers.append(core.gpa, data) catch return Error.OutOfMemory;
    fiber_ctx.fiber = id;
    const fiber = Fiber{ .core = core, .id = id };

    // Register the fiber's disposal on the parent fiber's effect bag so
    // parent rollback cascades to child plugins.
    const holder = core.a().create(Fiber) catch return Error.OutOfMemory;
    holder.* = fiber;
    parent_bag.append(core.gpa, .{
        .label = "ctx.plugin()",
        .cleanup = Cleanup.bind(Fiber, holder, struct {
            fn run(f: *Fiber) void {
                f.dispose();
            }
        }.run),
    }) catch return Error.OutOfMemory;

    const key = @intFromPtr(plugin);
    const result = core.runtimes.getOrPut(key) catch return Error.OutOfMemory;
    if (!result.found_existing) result.value_ptr.* = .empty;
    result.value_ptr.append(core.gpa, id) catch return Error.OutOfMemory;

    core.queue(id);
    return fiber;
}
