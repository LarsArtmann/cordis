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
    try ctx.onNamed(name, Listener.bind(Counter, counter, Counter.listener));
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
    try ctx.onNamed("test", Listener.bind(S, &s, S.first));
    try ctx.onNamed("test", Listener.bind(S, &s, S.second));
    try ctx.onNamed("test", Listener.bind(S, &s, S.third));

    const result = ctx.bail("test", &.{});
    try std.testing.expect(result != null);
    try std.testing.expectEqual(2, s.calls); // third listener never ran
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
            try c.onNamed("custom-event", Listener.bind(@This(), self, listener));
            return cordis.Error.PluginFailed;
        }
        fn healthy(p: *const Plugin, c: *Context, config: ?Value) cordis.Error!void {
            _ = p;
            _ = config;
            const self: *@This() = @ptrCast(@alignCast(registry));
            try c.onNamed("custom-event", Listener.bind(@This(), self, listener));
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
    try std.testing.expectEqual(1, ctx.loggedErrors().len);
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
            try c.onNamed("custom-event", Listener.bind(@This(), self, listener));
        }
        fn applyOuter(p: *const Plugin, c: *Context, config: ?Value) cordis.Error!void {
            _ = p;
            _ = config;
            const self: *@This() = @ptrCast(@alignCast(registry));
            try c.onNamed("custom-event", Listener.bind(@This(), self, listener));
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
            try c.provideNamed("foo", cordis.value(stored));
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

    const iso1 = ctx.isolate("foo");
    const iso2 = ctx.isolate("foo");

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
    try ctx.provideNamed("foo", cordis.value(&v1));
    try std.testing.expectEqual(1, s.calls);
    try std.testing.expect(iso1.getNamed("foo") == null);

    const v2: i32 = 200;
    try iso1.provideNamed("foo", cordis.value(&v2));
    try std.testing.expectEqual(2, s.calls);
    try std.testing.expect(iso2.getNamed("foo") == null);
    try std.testing.expectEqual(100, ctx.getTypedNamed(i32, "foo").?.*);
}

test "shared isolation label shares the realm" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const iso1 = ctx.isolateShared("foo", "shared");
    const iso2 = ctx.isolateShared("foo", "shared");

    const v: i32 = 200;
    try iso1.provideNamed("foo", cordis.value(&v));
    try std.testing.expectEqual(200, iso2.getTypedNamed(i32, "foo").?.*);
    try std.testing.expect(ctx.getNamed("foo") == null);
}

test "realm filtered events" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const isolated = ctx.isolate("foo");
    var root_calls = Counter{};
    var iso_calls = Counter{};
    try onCount(ctx, "custom-event", &root_calls);
    try onCount(isolated, "custom-event", &iso_calls);

    const emitter = isolated.withFilter(Context.realmFilter(isolated, "foo"));
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
