//! Parity tests mirroring the Go and Rust suites.

const std = @import("std");
const cordis = @import("cordis");

const Context = cordis.Context;
const Fiber = cordis.Fiber;
const Listener = cordis.Listener;
const Plugin = cordis.Plugin;
const Value = cordis.Value;

const Counter = struct {
    n: i32 = 0,

    fn listener(self: *Counter, args: []const Value) ?Value {
        _ = args;
        self.n += 1;
        return null;
    }
};

fn onCount(ctx: *Context, name: []const u8, counter: *Counter) !void {
    _ = try ctx.onNamed(name, Listener.bind(Counter, counter, Counter.listener));
}

fn readPort(ctx: *Context) ?u32 {
    const v = ctx.getNamed("port") orelse return null;
    const p: *const u32 = @ptrCast(@alignCast(v));
    return p.*;
}

test "on, emit, dispose by fiber rollback" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    var counter = Counter{};
    try onCount(ctx, "test", &counter);
    ctx.emitNamed("test", &.{});
    try std.testing.expectEqual(1, counter.n);
}

test "bail returns first non-null result" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const S = struct {
        calls: i32 = 0,
        answer: i32 = 42,
        fn first(self: *@This(), args: []const Value) ?Value {
            _ = args;
            self.calls += 1;
            return null;
        }
        fn second(self: *@This(), args: []const Value) ?Value {
            _ = args;
            self.calls += 1;
            return cordis.value(&self.answer);
        }
        fn third(self: *@This(), args: []const Value) ?Value {
            _ = args;
            self.calls += 1;
            return null;
        }
    };
    var s = S{};
    _ = try ctx.onNamed("test", Listener.bind(S, &s, S.first));
    _ = try ctx.onNamed("test", Listener.bind(S, &s, S.second));
    _ = try ctx.onNamed("test", Listener.bind(S, &s, S.third));

    const result = ctx.bail("test", &.{});
    try std.testing.expect(result != null);
    try std.testing.expectEqual(2, s.calls); // third listener never ran
}

// Mid-dispatch disposal (upstream #242 parity, mirroring the Go suite): a
// listener disposed by an earlier listener of the same dispatch must not
// fire. Disposal splices the hook from the store, but every dispatch
// iterates a snapshot of hook pointers, so the removed tombstone is what
// the loops honor. Registration order is invocation order: the disposer
// registers first, the victim second.

const Disposal = struct {
    victim: ?cordis.Disposer = null,
    victim_called: bool = false,

    fn disposer(self: *@This(), args: []const Value) ?Value {
        _ = args;
        if (self.victim) |v| v.dispose();
        return null;
    }

    fn onVictim(self: *@This(), args: []const Value) ?Value {
        _ = args;
        self.victim_called = true;
        return null;
    }

    fn onVictimBail(self: *@This(), args: []const Value) ?Value {
        _ = args;
        self.victim_called = true;
        return cordis.value(self);
    }
};

test "emit skips a listener disposed mid-dispatch" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    var s = Disposal{};
    _ = try ctx.onNamed("test", Listener.bind(Disposal, &s, Disposal.disposer));
    s.victim = try ctx.onNamed("test", Listener.bind(Disposal, &s, Disposal.onVictim));

    ctx.emitNamed("test", &.{});

    try std.testing.expect(!s.victim_called);
}

test "parallel skips a listener disposed mid-dispatch" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    var s = Disposal{};
    _ = try ctx.onNamed("test", Listener.bind(Disposal, &s, Disposal.disposer));
    s.victim = try ctx.onNamed("test", Listener.bind(Disposal, &s, Disposal.onVictim));

    ctx.parallel("test", &.{});

    try std.testing.expect(!s.victim_called);
}

test "serial skips a listener disposed mid-dispatch" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    var s = Disposal{};
    _ = try ctx.onNamed("test", Listener.bind(Disposal, &s, Disposal.disposer));
    s.victim = try ctx.onNamed("test", Listener.bind(Disposal, &s, Disposal.onVictimBail));

    try std.testing.expect(ctx.serial("test", &.{}) == null);
    try std.testing.expect(!s.victim_called);
}

test "bail skips a listener disposed mid-dispatch" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    var s = Disposal{};
    _ = try ctx.onNamed("test", Listener.bind(Disposal, &s, Disposal.disposer));
    s.victim = try ctx.onNamed("test", Listener.bind(Disposal, &s, Disposal.onVictimBail));

    try std.testing.expect(ctx.bail("test", &.{}) == null);
    try std.testing.expect(!s.victim_called);
}

const WaterfallDisposal = struct {
    ctx: *Context,
    victim: ?cordis.Disposer = null,
    victim_called: bool = false,

    fn nextArg(args: []const Value) *cordis.Context.Next {
        return @ptrCast(@alignCast(@constCast(args[args.len - 1])));
    }

    fn disposer(self: *@This(), args: []const Value) ?Value {
        if (self.victim) |v| v.dispose();
        return nextArg(args).invoke(args[0 .. args.len - 1]);
    }

    fn onVictim(self: *@This(), args: []const Value) ?Value {
        _ = args;
        self.victim_called = true;
        return null;
    }

    fn third(self: *@This(), args: []const Value) ?Value {
        const next = nextArg(args);
        // Arena allocated: the pointer flows out through the terminal
        // return, so it must outlive this listener's frame.
        const out = self.ctx.core.a().create(i32) catch return null;
        out.* = 3;
        return next.invoke(&.{cordis.value(out)});
    }
};

fn terminalFirstArg(args: []const Value) ?Value {
    if (args.len == 0) return null;
    return args[0];
}

test "waterfall continues past a listener disposed mid-dispatch" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    var s = WaterfallDisposal{ .ctx = ctx };
    _ = try ctx.onNamed("test", Listener.bind(WaterfallDisposal, &s, WaterfallDisposal.disposer));
    s.victim = try ctx.onNamed("test", Listener.bind(WaterfallDisposal, &s, WaterfallDisposal.onVictim));
    _ = try ctx.onNamed("test", Listener.bind(WaterfallDisposal, &s, WaterfallDisposal.third));

    const result = ctx.waterfall("test", &.{}, &terminalFirstArg);

    try std.testing.expect(result != null);
    try std.testing.expectEqual(@as(i32, 3), @as(*const i32, @ptrCast(@alignCast(result.?))).*);
    try std.testing.expect(!s.victim_called);
}

test "waterfall reaches the terminal when every listener was disposed mid-dispatch" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    var s = WaterfallDisposal{ .ctx = ctx };
    _ = try ctx.onNamed("test", Listener.bind(WaterfallDisposal, &s, WaterfallDisposal.disposer));
    s.victim = try ctx.onNamed("test", Listener.bind(WaterfallDisposal, &s, WaterfallDisposal.onVictim));

    const Terminal = struct {
        fn run(_: []const Value) ?Value {
            return cordis.value(&reached);
        }

        var reached: u8 = 7;
    };

    const result = ctx.waterfall("test", &.{}, &Terminal.run);

    try std.testing.expect(result != null);
    try std.testing.expect(!s.victim_called);
}

// Pins the mechanism itself: disposal tombstones the hook before splicing,
// so an already-captured dispatch snapshot observes it through the shared
// pointer.
test "disposal sets the removed tombstone on the hook" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    var counter = Counter{};
    const d = try ctx.onNamed("test", Listener.bind(Counter, &counter, Counter.listener));
    const hooks = ctx.core.hooks.getPtr("test").?.items;
    try std.testing.expectEqual(@as(usize, 1), hooks.len);
    const hook = hooks[0];
    try std.testing.expect(!hook.removed);

    d.dispose();

    try std.testing.expect(hook.removed);
    try std.testing.expectEqual(@as(usize, 0), ctx.core.hooks.getPtr("test").?.items.len);
}

test "plugin lifecycle: apply, restart, dispose" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const S = struct {
        applies: i32 = 0,
        fn apply(p: *const Plugin, c: *Context, config: ?Value) cordis.Error!void {
            _ = p;
            _ = c;
            _ = config;
            const self: *@This() = @ptrCast(@alignCast(registry));
            self.applies += 1;
        }
        var registry: *@This() = undefined;
    };
    var s = S{};
    S.registry = &s;

    const p = Plugin{ .name = "greeter", .apply = S.apply };
    const fiber = try p.start(ctx, null);
    try std.testing.expectEqual(1, s.applies);
    try std.testing.expectEqual(cordis.FiberState.active, fiber.state());
    try std.testing.expectEqualStrings("greeter", fiber.name());

    try fiber.restart();
    try std.testing.expectEqual(2, s.applies);

    fiber.dispose();
    try std.testing.expectEqual(cordis.FiberState.disposed, fiber.state());
    try std.testing.expectEqual(-1, fiber.uid());
    fiber.dispose(); // idempotent
}

test "plugin error rolls back partial effects" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const S = struct {
        calls: i32 = 0,
        fn listener(self: *@This(), args: []const Value) ?Value {
            _ = args;
            self.calls += 1;
            return null;
        }
        fn faulty(p: *const Plugin, c: *Context, config: ?Value) cordis.Error!void {
            _ = p;
            _ = config;
            const self: *@This() = @ptrCast(@alignCast(registry));
            _ = try c.onNamed("custom-event", Listener.bind(@This(), self, listener));
            return cordis.Error.PluginFailed;
        }
        fn healthy(p: *const Plugin, c: *Context, config: ?Value) cordis.Error!void {
            _ = p;
            _ = config;
            const self: *@This() = @ptrCast(@alignCast(registry));
            _ = try c.onNamed("custom-event", Listener.bind(@This(), self, listener));
        }
        var registry: *@This() = undefined;
    };
    var s = S{};
    S.registry = &s;

    const faulty = Plugin{ .name = "faulty", .apply = S.faulty };
    const healthy = Plugin{ .name = "healthy", .apply = S.healthy };
    const faulty_fiber = try faulty.start(ctx, null);
    _ = try healthy.start(ctx, null);

    try std.testing.expectEqual(cordis.FiberState.failed, faulty_fiber.state());
    try std.testing.expectEqual(1, (try ctx.loggedErrors()).len);
    ctx.emitNamed("custom-event", &.{});
    try std.testing.expectEqual(1, s.calls); // only the healthy listener
}

test "nested plugins cascade on dispose" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const S = struct {
        calls: i32 = 0,
        inner: Plugin = undefined,
        fn listener(self: *@This(), args: []const Value) ?Value {
            _ = args;
            self.calls += 1;
            return null;
        }
        fn applyInner(p: *const Plugin, c: *Context, config: ?Value) cordis.Error!void {
            _ = p;
            _ = config;
            const self: *@This() = @ptrCast(@alignCast(registry));
            _ = try c.onNamed("custom-event", Listener.bind(@This(), self, listener));
        }
        fn applyOuter(p: *const Plugin, c: *Context, config: ?Value) cordis.Error!void {
            _ = p;
            _ = config;
            const self: *@This() = @ptrCast(@alignCast(registry));
            _ = try c.onNamed("custom-event", Listener.bind(@This(), self, listener));
            _ = try self.inner.start(c, null);
        }
        var registry: *@This() = undefined;
    };
    var s = S{};
    S.registry = &s;
    s.inner = Plugin{ .name = "inner", .apply = S.applyInner };
    const outer = Plugin{ .name = "outer", .apply = S.applyOuter };

    const fiber = try outer.start(ctx, null);
    ctx.emitNamed("custom-event", &.{});
    try std.testing.expectEqual(2, s.calls);

    fiber.dispose();
    ctx.emitNamed("custom-event", &.{});
    try std.testing.expectEqual(2, s.calls); // both rolled back
}

test "inject reactivity: pending, active, unload, reload" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const S = struct {
        applies: i32 = 0,
        cleanups: i32 = 0,
        fn apply(p: *const Plugin, c: *Context, config: ?Value) cordis.Error!void {
            _ = p;
            _ = c;
            _ = config;
            const self: *@This() = @ptrCast(@alignCast(registry));
            self.applies += 1;
        }
        fn provideFoo(p: *const Plugin, c: *Context, config: ?Value) cordis.Error!void {
            _ = p;
            _ = config;
            const self: *@This() = @ptrCast(@alignCast(registry));
            const v: i32 = 1;
            const stored = c.core.a().create(i32) catch @panic("cordis: out of memory");
            stored.* = v;
            _ = self;
            _ = try c.provideNamed("foo", cordis.value(stored));
        }
        var registry: *@This() = undefined;
    };
    var s = S{};
    S.registry = &s;

    const deps = [_][]const u8{"foo"};
    const consumer = Plugin{ .name = "consumer", .inject = &deps, .apply = S.apply };
    const fiber = try consumer.start(ctx, null);
    try std.testing.expectEqual(cordis.FiberState.pending, fiber.state());
    try std.testing.expectEqual(0, s.applies);

    const provider = Plugin{ .name = "provider", .apply = S.provideFoo };
    const provider_fiber = try provider.start(ctx, null);
    try std.testing.expectEqual(cordis.FiberState.active, fiber.state());
    try std.testing.expectEqual(1, s.applies);

    provider_fiber.dispose();
    try std.testing.expectEqual(cordis.FiberState.pending, fiber.state());

    _ = try provider.start(ctx, null);
    try std.testing.expectEqual(cordis.FiberState.active, fiber.state());
    try std.testing.expectEqual(2, s.applies);
}

test "isolation realms" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const iso1 = try ctx.isolate("foo");
    const iso2 = try ctx.isolate("foo");

    const S = struct {
        calls: i32 = 0,
        fn apply(p: *const Plugin, c: *Context, config: ?Value) cordis.Error!void {
            _ = p;
            _ = c;
            _ = config;
            const self: *@This() = @ptrCast(@alignCast(registry));
            self.calls += 1;
        }
        var registry: *@This() = undefined;
    };
    var s = S{};
    S.registry = &s;

    const deps = [_][]const u8{"foo"};
    const watcher = Plugin{ .name = "watcher", .inject = &deps, .apply = S.apply };
    for ([_]*Context{ ctx, iso1, iso2 }) |scope| {
        _ = try watcher.start(scope, null);
    }

    const v1: i32 = 100;
    _ = try ctx.provideNamed("foo", cordis.value(&v1));
    try std.testing.expectEqual(1, s.calls);
    try std.testing.expect(iso1.getNamed("foo") == null);

    const v2: i32 = 200;
    _ = try iso1.provideNamed("foo", cordis.value(&v2));
    try std.testing.expectEqual(2, s.calls);
    try std.testing.expect(iso2.getNamed("foo") == null);
    try std.testing.expectEqual(100, ctx.getTypedNamed(i32, "foo").?.*);
}

test "shared isolation label shares the realm" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const iso1 = try ctx.isolateShared("foo", "shared");
    const iso2 = try ctx.isolateShared("foo", "shared");

    const v: i32 = 200;
    _ = try iso1.provideNamed("foo", cordis.value(&v));
    try std.testing.expectEqual(200, iso2.getTypedNamed(i32, "foo").?.*);
    try std.testing.expect(ctx.getNamed("foo") == null);
}

test "shared isolation labels are collision free" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    // With the previous "{name}\x00{label}" synthetic key these two distinct
    // pairs collapsed into one realm.
    const a = try ctx.isolateShared("foo\x00bar", "baz");
    const b = try ctx.isolateShared("foo", "bar\x00baz");

    const v: i32 = 1;
    _ = try a.provideNamed("foo", cordis.value(&v));
    try std.testing.expect(b.getNamed("foo") == null);

    const a2 = try ctx.isolateShared("foo\x00bar", "baz");
    try std.testing.expectEqual(1, a2.getTypedNamed(i32, "foo").?.*);
}

test "realm filtered events" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const isolated = try ctx.isolate("foo");
    var root_calls = Counter{};
    var iso_calls = Counter{};
    try onCount(ctx, "custom-event", &root_calls);
    try onCount(isolated, "custom-event", &iso_calls);

    const emitter = try isolated.withFilter(try isolated.realmFilter(isolated, "foo"));
    emitter.emitNamed("custom-event", &.{});
    try std.testing.expectEqual(0, root_calls.n);
    try std.testing.expectEqual(1, iso_calls.n);

    ctx.emitNamed("custom-event", &.{});
    try std.testing.expectEqual(1, root_calls.n);
    try std.testing.expectEqual(2, iso_calls.n);
}

test "update reinvokes with the new config" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const S = struct {
        last: i32 = 0,
        fn apply(p: *const Plugin, c: *Context, config: ?Value) cordis.Error!void {
            _ = p;
            _ = c;
            const self: *@This() = @ptrCast(@alignCast(registry));
            self.last = @as(*const i32, @ptrCast(@alignCast(config.?))).*;
        }
        var registry: *@This() = undefined;
    };
    var s = S{};
    S.registry = &s;

    const p = Plugin{ .name = "p", .apply = S.apply };
    const one: i32 = 1;
    const two: i32 = 2;
    const fiber = try p.start(ctx, cordis.value(&one));
    try std.testing.expectEqual(1, s.last);
    try fiber.update(cordis.value(&two));
    try std.testing.expectEqual(2, s.last);
    try std.testing.expectEqual(cordis.FiberState.active, fiber.state());
}

test "status events emission order" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const Recorder = struct {
        lines: std.ArrayList([]const u8) = .empty,

        fn listener(self: *Self(), args: []const Value) ?Value {
            const change: *const cordis.StatusChange = @ptrCast(@alignCast(args[0]));
            const line = std.fmt.allocPrint(
                std.heap.page_allocator,
                "{s}:{s}->{s}",
                .{ change.name, @tagName(change.old), @tagName(change.new) },
            ) catch return null;
            self.lines.append(std.heap.page_allocator, line) catch {};
            return null;
        }
        fn Self() type {
            return @This();
        }
    };
    var recorder = Recorder{};
    _ = try ctx.onNamed(cordis.event_status, Listener.bind(Recorder, &recorder, Recorder.listener));

    const applies = struct {
        var count: i32 = 0;
        fn apply(_: *const Plugin, _: *Context, _: ?Value) cordis.Error!void {
            count += 1;
        }
    };
    const p = try ctx.injectPlugin("svc", &.{}, applies.apply);
    try std.testing.expectEqualStrings("svc:pending->loading", recorder.lines.items[0]);
    try std.testing.expectEqualStrings("svc:loading->active", recorder.lines.items[1]);

    recorder.lines.clearRetainingCapacity();
    try p.restart();
    try std.testing.expectEqual(@as(usize, 3), recorder.lines.items.len);
    try std.testing.expectEqualStrings("svc:active->unloading", recorder.lines.items[0]);
    try std.testing.expectEqualStrings("svc:unloading->loading", recorder.lines.items[1]);
    try std.testing.expectEqualStrings("svc:loading->active", recorder.lines.items[2]);

    recorder.lines.clearRetainingCapacity();
    p.dispose();
    try std.testing.expectEqual(@as(usize, 2), recorder.lines.items.len);
    try std.testing.expectEqualStrings("svc:active->unloading", recorder.lines.items[0]);
    try std.testing.expectEqualStrings("svc:unloading->disposed", recorder.lines.items[1]);
    try std.testing.expectEqual(cordis.FiberState.disposed, p.state());
    for (recorder.lines.items) |line| std.heap.page_allocator.free(line);
}

test "plugin events fire on create and dispose, never for root" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const Seen = struct {
        states: std.ArrayList(cordis.FiberState) = .empty,
        fn listener(self: *@This(), args: []const Value) ?Value {
            const fiber: *const Fiber = @ptrCast(@alignCast(args[0]));
            self.states.append(std.heap.page_allocator, fiber.state()) catch {};
            return null;
        }
    };
    var seen = Seen{};
    _ = try ctx.onNamed(cordis.event_plugin, Listener.bind(Seen, &seen, Seen.listener));

    const noop = struct {
        fn apply(_: *const Plugin, _: *Context, _: ?Value) cordis.Error!void {}
    };
    const fiber = try ctx.injectPlugin("p", &.{}, noop.apply);
    try std.testing.expectEqual(@as(usize, 1), seen.states.items.len);
    try std.testing.expectEqual(cordis.FiberState.pending, seen.states.items[0]);

    fiber.dispose();
    try std.testing.expectEqual(@as(usize, 2), seen.states.items.len);
    try std.testing.expectEqual(cordis.FiberState.active, seen.states.items[1]);

    // Root disposal is a restart and fires no plugin event.
    ctx.fiberHandle().dispose();
    try std.testing.expectEqual(@as(usize, 2), seen.states.items.len);
    for (seen.states.items) |_| {}
}

test "update runs the interception waterfall and veto works" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const svc = struct {
        fn apply(_: *const Plugin, c: *Context, config: ?Value) cordis.Error!void {
            const port: *const u32 = @ptrCast(@alignCast(config.?));
            _ = try c.provideNamed("port", cordis.value(port));
        }
    };
    const svc_plugin = try ctx.core.a().create(Plugin);
    svc_plugin.* = .{ .name = "svc", .apply = svc.apply };
    var initial: u32 = 8080;
    const fiber = try svc_plugin.start(ctx, cordis.value(&initial));
    try std.testing.expectEqual(@as(?u32, 8080), readPort(ctx));

    // Rewrite: the listener passes a new config through next.
    const Rewriter = struct {
        fn listener(_: *u8, args: []const Value) ?Value {
            const next: *Context.Next = @ptrCast(@alignCast(@constCast(args[3])));
            const updating: *const Fiber = @ptrCast(@alignCast(args[0]));
            const rewritten = updating.core.a().create(u32) catch return null;
            rewritten.* = 9090;
            return next.invoke(&.{ args[0], cordis.value(rewritten) });
        }
    };
    var dummy: u8 = 0;
    _ = try ctx.onNamed(cordis.event_update, Listener.bind(u8, &dummy, Rewriter.listener));
    var updated: u32 = 7070;
    try fiber.update(cordis.value(&updated));
    try std.testing.expectEqual(@as(?u32, 9090), readPort(ctx));

    // Veto: a listener that never invokes next leaves the config untouched.
    const Vetoer = struct {
        fn listener(_: *u8, _: []const Value) ?Value {
            return null;
        }
    };
    var veto_dummy: u8 = 0;
    _ = try ctx.onNamed(cordis.event_update, Listener.bind(u8, &veto_dummy, Vetoer.listener));
    var vetoed_config: u32 = 6060;
    try fiber.update(cordis.value(&vetoed_config));
    try std.testing.expectEqual(@as(?u32, 9090), readPort(ctx));
}

test "root fiber update is rejected and restart rolls back" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    try std.testing.expectError(cordis.Error.RootUpdate, ctx.fiberHandle().update(cordis.value(&@as(u32, 1))));

    var ran: i32 = 0;
    const Cleanup = struct {
        fn run(counter: *i32) void {
            counter.* += 1;
        }
    };
    try ctx.attach(&ran, Cleanup.run);
    // Restarting the root used to fall into the plugin load path; it must
    // roll back in place.
    try ctx.fiberHandle().restart();
    try std.testing.expectEqual(@as(i32, 1), ran);
}

test "intercept and intercepted" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    var level: []const u8 = "info";
    const scoped = try ctx.intercept("logger", cordis.value(&level));
    try std.testing.expect(scoped.intercepted("logger") != null);
    try std.testing.expect(ctx.intercepted("logger") == null);
    const v = scoped.intercepted("logger").?;
    const read: *const []const u8 = @ptrCast(@alignCast(v));
    try std.testing.expectEqualStrings("info", read.*);
}

test "logger intercept resolves names and gates levels" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    try std.testing.expectEqualStrings("root", ctx.logger(null).getName());
    try std.testing.expectEqualStrings("custom", ctx.logger("custom").getName());

    const li = cordis.LoggerIntercept{ .name = "scoped", .level = .warn };
    const scoped = try ctx.intercept("logger", cordis.value(&li));
    try std.testing.expectEqualStrings("scoped", scoped.logger(null).getName());
    try std.testing.expectEqualStrings("explicit", scoped.logger("explicit").getName());

    scoped.logger(null).info(&.{"hidden"});
    scoped.logger(null).warn(&.{"shown"});
    const buf = ctx.loggerBuffer();
    try std.testing.expectEqual(@as(usize, 1), buf.len);
    try std.testing.expectEqualStrings("shown", buf[0].args[0]);
}

test "logger buffer, exporters and level filters" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    ctx.setLoggerBufferSize(2);
    const log = ctx.logger("test");
    log.info(&.{"one"});
    log.info(&.{"two"});
    log.info(&.{"three"});
    try std.testing.expectEqual(@as(usize, 2), ctx.loggerBuffer().len);
    try std.testing.expectEqualStrings("two", ctx.loggerBuffer()[0].args[0]);
    try std.testing.expectEqualStrings("three", ctx.loggerBuffer()[1].args[0]);

    const Collector = struct {
        lines: std.ArrayList([]const u8) = .empty,
        fn export_one(self: *@This(), message: *const cordis.Message) void {
            const line = cordis.formatMessage(std.heap.page_allocator, message) catch return;
            self.lines.append(std.heap.page_allocator, line) catch {};
        }
    };
    var collector = Collector{};
    const remove = try ctx.addExporter(cordis.Exporter.bind(Collector, &collector, Collector.export_one), null);

    var quiet = std.StringHashMap(cordis.Level).init(std.testing.allocator);
    try quiet.put("quiet", .err);
    const quiet_remove = try ctx.addExporter(
        cordis.Exporter.bind(Collector, &collector, Collector.export_one),
        quiet,
    );
    quiet_remove.dispose();

    ctx.logger("test").info(&.{"hello"});
    try std.testing.expectEqual(@as(usize, 1), collector.lines.items.len);
    remove.dispose();
    ctx.logger("test").info(&.{"world"});
    try std.testing.expectEqual(@as(usize, 1), collector.lines.items.len);
    for (collector.lines.items) |line| std.heap.page_allocator.free(line);
}

test "plugin errors flow through the logger" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const faulty = struct {
        fn apply(_: *const Plugin, _: *Context, _: ?Value) cordis.Error!void {
            return cordis.Error.PluginFailed;
        }
    };
    const fiber = try ctx.injectPlugin("faulty", &.{}, faulty.apply);
    try std.testing.expectEqual(cordis.FiberState.failed, fiber.state());
    const errors = try ctx.loggedErrors();
    try std.testing.expectEqual(@as(usize, 1), errors.len);
    try std.testing.expectEqualStrings("<faulty> PluginFailed", errors[0]);
}

test "config validation rejects starts before any fiber exists" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const Config = struct { port: u32 };
    const V = struct {
        fn validate(c: Config) cordis.Error!void {
            if (c.port == 0) return cordis.Error.Validation;
        }
        fn apply(_: *Context, _: Config) cordis.Error!void {}
    };
    const Server = cordis.ValidatedPlugin("server", Config, V.validate, V.apply, &.{});

    try std.testing.expectError(cordis.Error.Validation, Server.start(ctx, .{ .port = 0 }));
    try std.testing.expectEqual(@as(usize, 0), ctx.registry().size());
    const fiber = try Server.start(ctx, .{ .port = 8080 });
    try std.testing.expectEqual(cordis.FiberState.active, fiber.state());
}

test "snapshot and restore roundtrip" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const svc = struct {
        var starts: i32 = 0;
        fn apply(_: *const Plugin, c: *Context, config: ?Value) cordis.Error!void {
            starts += 1;
            const port: *const u32 = @ptrCast(@alignCast(config.?));
            _ = try c.provideNamed("port", cordis.value(port));
        }
    };
    const plugin = try ctx.core.a().create(Plugin);
    plugin.* = .{ .name = "svc", .apply = svc.apply };
    var port: u32 = 8080;
    _ = try plugin.start(ctx, cordis.value(&port));

    const snap = try ctx.snapshot();
    try std.testing.expectEqual(@as(usize, 1), snap.runtimes.len);
    try std.testing.expectEqualStrings("svc", snap.runtimes[0].name);
    try std.testing.expectEqual(@as(usize, 1), snap.runtimes[0].fibers.len);

    ctx.registry().delete(plugin);
    try std.testing.expectEqual(@as(usize, 0), ctx.registry().size());
    try ctx.restore(snap);
    try std.testing.expectEqual(@as(i32, 2), svc.starts);
    try std.testing.expectEqual(@as(?u32, 8080), readPort(ctx));

    // Restoring an older snapshot disposes runtimes that appeared since.
    const after = try ctx.snapshot();
    const extra = try ctx.core.a().create(Plugin);
    const noop = struct {
        fn apply(_: *const Plugin, _: *Context, _: ?Value) cordis.Error!void {}
    };
    extra.* = .{ .name = "extra", .apply = noop.apply };
    _ = try extra.start(ctx, null);
    try std.testing.expectEqual(@as(usize, 2), ctx.registry().size());
    try ctx.restore(after);
    const now = try ctx.snapshot();
    try std.testing.expectEqual(@as(usize, 1), now.runtimes.len);
    try std.testing.expectEqualStrings("svc", now.runtimes[0].name);
    try std.testing.expectEqual(cordis.FiberState.active, now.runtimes[0].fibers[0].state);
}

test "accessor derives, follows lifecycle and writes back" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const Config = struct { port: u32 };
    var source = Config{ .port = 8080 };
    _ = try ctx.provide(&source);

    const Get = struct {
        fn get(_: *Context, s: *const Config) cordis.Error!u32 {
            return s.port;
        }
        fn set(_: *Context, s: *const Config, v: u32) cordis.Error!void {
            const mutable: *Config = @constCast(s);
            mutable.port = v;
        }
    };
    const result = try cordis.accessor(Config, u32, ctx, "port", Get.get, Get.set);
    try std.testing.expectEqual(@as(?u32, 8080), readPort(ctx));

    try result.member.set(9090);
    try std.testing.expectEqual(@as(u32, 9090), source.port);
    try std.testing.expectEqual(@as(?u32, 9090), readPort(ctx));

    // The derived service follows the accessor's lifecycle: disposing the
    // accessor withdraws it, while the source service stays.
    result.fiber.dispose();
    try std.testing.expect(readPort(ctx) == null);
    try std.testing.expect(ctx.getTyped(Config) != null);
}

test "mixin exposes one member and read-only accessors reject writes" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const Logger = struct { level: []const u8 = "info" };
    var logger = Logger{};
    _ = try ctx.provide(&logger);

    const Get = struct {
        fn get(s: *const Logger) []const u8 {
            return s.level;
        }
        fn set(s: *const Logger, v: []const u8) void {
            const mutable: *Logger = @constCast(s);
            mutable.level = v;
        }
    };
    const result = try cordis.mixin(Logger, []const u8, ctx, "level", Get.get, Get.set);
    const level = ctx.getTypedNamed([]const u8, "level").?;
    try std.testing.expectEqualStrings("info", level.*);
    try result.member.set("debug");
    try std.testing.expectEqualStrings("debug", ctx.getTypedNamed([]const u8, "level").?.*);

    const RoGet = struct {
        fn get(s: *const Logger) []const u8 {
            return s.level;
        }
    };
    const ro = try cordis.mixin(Logger, []const u8, ctx, "level2", RoGet.get, null);
    try std.testing.expectError(cordis.Error.ReadOnlyAccessor, ro.member.set("x"));
}

test "registration labels carry their names" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const noop_listener = struct {
        fn call(_: *u8, _: []const Value) ?Value {
            return null;
        }
    };
    var dummy: u8 = 0;
    _ = try ctx.onNamed("custom-event", Listener.bind(u8, &dummy, noop_listener.call));
    _ = try ctx.provideNamed("port", cordis.value(&@as(u32, 1)));
    try ctx.attachLabeled("my-cleanup", &dummy, struct {
        fn run(_: *u8) void {}
    }.run);

    const effects = ctx.effects();
    try std.testing.expectEqualStrings("ctx.on(custom-event)", effects[0].label);
    try std.testing.expectEqualStrings("ctx.provide(port)", effects[1].label);
    try std.testing.expectEqualStrings("my-cleanup", effects[2].label);
}

// ---------------------------------------------------------------------------
// Parity battery for the upstream rc.13 core fixes, mirroring the Go suite's
// parity_battery_test.go. Each test pins an upstream spec scenario against
// this port; a red result means the port drifted and the port gets fixed,
// not the test.
//
// Upstream #205 (Parallel labels its aggregate error with the failing event
// name) has no Zig counterpart to pin: `Context.parallel` returns void and
// Zig listeners (`Listener.call`) have no error channel, so the port never
// constructs a joined listener error whose message could carry the label —
// and the dispatch surface is shared by all five modes, so an error
// reporting parallel would be a listener ABI redesign, not a parity fix.

// #56 (7d0a90d): internal/update hooks must be disposed with the fiber, so
// reloads never accumulate them. Upstream leaked them because the hooks
// lived in a per-fiber list outside the effect scope; this port routes
// event_update through the ordinary on machinery, which should make the
// leak impossible. This pins it: every update fires exactly one hook.
test "event update hooks do not accumulate across reloads (#56)" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const Hooks = struct {
        calls: [8]i32 = undefined,
        len: usize = 0,

        // The update waterfall delivers [fiber, config, no_save] plus a
        // *Next; the listener records the config and passes it through.
        fn listener(self: *@This(), args: []const Value) ?Value {
            const config: *const i32 = @ptrCast(@alignCast(args[1]));
            self.calls[self.len] = config.*;
            self.len += 1;
            const next: *Context.Next = @ptrCast(@alignCast(@constCast(args[3])));
            return next.invoke(args[0..3]);
        }

        fn apply(_: *const Plugin, c: *Context, _: ?Value) cordis.Error!void {
            const self: *@This() = @ptrCast(@alignCast(registry));
            _ = try c.onNamed(cordis.event_update, Listener.bind(@This(), self, listener));
        }

        var registry: *@This() = undefined;
    };
    var hooks = Hooks{};
    Hooks.registry = &hooks;

    const p = Plugin{ .name = "p", .apply = Hooks.apply };
    const fiber = try p.start(ctx, null);

    for ([_]i32{ 1, 2, 3 }) |n| {
        const config = try ctx.core.a().create(i32);
        config.* = n;
        try fiber.update(cordis.value(config));
    }

    try std.testing.expectEqual(@as(usize, 3), hooks.len);
    try std.testing.expectEqualSlices(i32, &.{ 1, 2, 3 }, hooks.calls[0..hooks.len]);
}

// #133 (5dac695): a pending fiber that is disposed must settle to DISPOSED,
// not linger in PENDING.
test "disposed pending fiber settles DISPOSED (#133)" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const Body = struct {
        var ran: bool = false;
        fn apply(_: *const Plugin, _: *Context, _: ?Value) cordis.Error!void {
            ran = true;
        }
    };
    const deps = [_][]const u8{"never-provided"};
    const fiber = try ctx.injectPlugin("pending", &deps, Body.apply);

    try std.testing.expectEqual(cordis.FiberState.pending, fiber.state());

    fiber.dispose();

    try std.testing.expectEqual(cordis.FiberState.disposed, fiber.state());
    try std.testing.expect(!Body.ran); // the body must never run
}

// #209 (bc9a344), scenario 1: an isolated child scope must not inherit the
// injected service view across the isolation boundary. A plain child plugin
// still resolves the outer realm; an isolated child must not — the fresh
// realm for the name stops the lookup walk at the boundary.
test "isolation boundary stops the injected view (#209)" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    var outer: []const u8 = "outer";
    _ = try ctx.provideNamed("foo", cordis.value(&outer));

    const S = struct {
        results: [2][]const u8 = undefined,
        len: usize = 0,

        fn record(self: *@This(), value: []const u8) void {
            self.results[self.len] = value;
            self.len += 1;
        }

        fn lookUp(self: *@This(), c: *Context) void {
            const v = c.getNamed("foo") orelse {
                self.record("cannot get property \"foo\" without inject");
                return;
            };
            const value: *const []const u8 = @ptrCast(@alignCast(v));
            self.record(value.*);
        }

        fn lookupApply(_: *const Plugin, c: *Context, _: ?Value) cordis.Error!void {
            const self: *@This() = @ptrCast(@alignCast(registry));
            self.lookUp(c);
        }

        fn injectBody(_: *const Plugin, c: *Context, _: ?Value) cordis.Error!void {
            // A plain child plugin still resolves the outer realm.
            const child = try c.core.a().create(Plugin);
            child.* = .{ .name = "child", .apply = lookupApply };
            _ = try child.start(c, null);

            // An isolated child must not.
            const iso = try c.isolate("foo");
            const isolated = try c.core.a().create(Plugin);
            isolated.* = .{ .name = "isolated", .apply = lookupApply };
            _ = try isolated.start(iso, null);
        }

        var registry: *@This() = undefined;
    };
    var s = S{};
    S.registry = &s;

    const deps = [_][]const u8{"foo"};
    const fiber = try ctx.injectPlugin("inject", &deps, S.injectBody);
    try std.testing.expectEqual(cordis.FiberState.active, fiber.state());

    try std.testing.expectEqual(@as(usize, 2), s.len);
    try std.testing.expectEqualStrings("outer", s.results[0]);
    try std.testing.expectEqualStrings("cannot get property \"foo\" without inject", s.results[1]);
}

// #191 (147e4f2): after an isolated provider unloads, providing the same
// service in the same isolated scope again must succeed — no stale impl
// left behind by the isolate move.
test "re-provide after isolated provider unload succeeds (#191)" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const iso = try ctx.isolate("svc");

    const S = struct {
        fn apply(_: *const Plugin, c: *Context, _: ?Value) cordis.Error!void {
            const stored = try c.core.a().create([]const u8);
            stored.* = "value";
            _ = try c.provideNamed("svc", cordis.value(stored));
        }
    };
    const provider = try ctx.core.a().create(Plugin);
    provider.* = .{ .name = "provider", .apply = S.apply };

    const first = try provider.start(iso, null);
    first.dispose();

    try std.testing.expect(iso.getNamed("svc") == null);

    const second = try provider.start(iso, null); // must not DuplicateService

    const v = second.context().getNamed("svc") orelse return error.TestUnexpectedResult;
    const value: *const []const u8 = @ptrCast(@alignCast(v));
    try std.testing.expectEqualStrings("value", value.*);
}

// #196 (9cad5e8): a provider's earlier effects (the resources backing a
// service) are released only after its dependents have unloaded — the
// dependent's teardown observes a working service value. This port
// implements it inside the provide cleanup: withdrawal first notifies and
// settles dependents (Core.settlePending), then the provider's LIFO
// cleanups continue.
test "provider resources outlive dependent teardown (#196)" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const S = struct {
        cell: []const u8 = "value",
        order: [4][]const u8 = undefined,
        len: usize = 0,
        observed: ?*const []const u8 = null,
        ok: bool = true,

        fn record(self: *@This(), line: []const u8) void {
            self.order[self.len] = line;
            self.len += 1;
        }

        fn releaseResource(self: *@This()) void {
            self.record("resource-released");
            self.cell = "";
        }

        fn teardownDependent(self: *@This()) void {
            self.record("dependent-teardown");
            const captured = self.observed orelse {
                self.ok = false;
                return;
            };
            if (!std.mem.eql(u8, captured.*, "value")) self.ok = false;
        }

        fn providerApply(_: *const Plugin, c: *Context, _: ?Value) cordis.Error!void {
            const self: *@This() = @ptrCast(@alignCast(registry));
            try c.attachLabeled("resource", self, releaseResource);
            _ = try c.provideNamed("svc", cordis.value(&self.cell));
        }

        fn dependentApply(_: *const Plugin, c: *Context, _: ?Value) cordis.Error!void {
            const self: *@This() = @ptrCast(@alignCast(registry));
            const v = c.getNamed("svc") orelse return cordis.Error.MissingService;
            self.observed = @ptrCast(@alignCast(v));
            try c.attachLabeled("dependent", self, teardownDependent);
        }

        var registry: *@This() = undefined;
    };
    var s = S{};
    S.registry = &s;

    const provider = Plugin{ .name = "provider", .apply = S.providerApply };
    const provider_fiber = try provider.start(ctx, null);

    const deps = [_][]const u8{"svc"};
    const dependent = try ctx.injectPlugin("dependent", &deps, S.dependentApply);
    try std.testing.expectEqual(cordis.FiberState.active, dependent.state());
    try std.testing.expect(s.observed != null);

    provider_fiber.dispose();

    try std.testing.expectEqual(@as(usize, 2), s.len);
    try std.testing.expectEqualStrings("dependent-teardown", s.order[0]);
    try std.testing.expectEqualStrings("resource-released", s.order[1]);
    try std.testing.expect(s.ok); // the service value still worked at teardown
    try std.testing.expect(ctx.getNamed("svc") == null);
}
