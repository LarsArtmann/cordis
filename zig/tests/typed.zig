//! Tests for the native API forms: comptime plugins with typed configs,
//! type-keyed services and typed events.

const std = @import("std");
const cordis = @import("cordis");

const Context = cordis.Context;
const Fiber = cordis.Fiber;

const Database = struct {
    dsn: []const u8,
};

const UserCreated = struct {
    id: i32,
};

const UserDeleted = struct {
    id: i32,
};

test "typed service round trip" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const db = Database{ .dsn = "postgres://localhost" };
    try std.testing.expect(ctx.getTyped(Database) == null);

    _ = try ctx.provide(&db);
    try std.testing.expectEqualStrings("postgres://localhost", ctx.getTyped(Database).?.dsn);
    try std.testing.expect(ctx.getTyped(u32) == null);
}

test "typed service duplicate and realm isolation" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const root_db = Database{ .dsn = "root" };
    const iso_db = Database{ .dsn = "isolated" };

    _ = try ctx.provide(&root_db);
    try std.testing.expectError(cordis.Error.DuplicateService, ctx.provide(&iso_db));

    const isolated = ctx.isolate(@typeName(Database));
    _ = try isolated.provide(&iso_db);
    try std.testing.expectEqualStrings("root", ctx.getTyped(Database).?.dsn);
    try std.testing.expectEqualStrings("isolated", isolated.getTyped(Database).?.dsn);
}

test "typed events dispatch by type" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const Sink = struct {
        received: std.ArrayListUnmanaged([]const u8) = .empty,

        fn onCreated(self: *@This(), e: UserCreated) void {
            self.received.append(std.testing.allocator, std.fmt.allocPrint(std.testing.allocator, "created {d}", .{e.id}) catch @panic("oom")) catch @panic("oom");
        }

        fn onDeleted(self: *@This(), e: UserDeleted) void {
            self.received.append(std.testing.allocator, std.fmt.allocPrint(std.testing.allocator, "deleted {d}", .{e.id}) catch @panic("oom")) catch @panic("oom");
        }
    };
    var sink = Sink{};
    defer {
        for (sink.received.items) |item| std.testing.allocator.free(item);
        sink.received.deinit(std.testing.allocator);
    }

    _ = try ctx.onTyped(UserCreated, &sink, Sink.onCreated);
    _ = try ctx.onTyped(UserDeleted, &sink, Sink.onDeleted);

    ctx.emitTyped(UserCreated, &.{ .id = 1 });
    ctx.emitTyped(UserDeleted, &.{ .id = 2 });
    ctx.emitTyped(UserCreated, &.{ .id = 3 });

    try std.testing.expectEqual(@as(usize, 3), sink.received.items.len);
    try std.testing.expectEqualStrings("created 1", sink.received.items[0]);
    try std.testing.expectEqualStrings("deleted 2", sink.received.items[1]);
    try std.testing.expectEqualStrings("created 3", sink.received.items[2]);
}

test "typed events roll back with the fiber" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const Sink = struct {
        n: i32 = 0,
        fn onEvent(self: *@This(), _: UserCreated) void {
            self.n += 1;
        }
    };
    var sink = Sink{};

    const S = struct {
        var holder: *Sink = undefined;
        fn apply(p: *const cordis.Plugin, c: *Context, _: ?cordis.Value) cordis.Error!void {
            _ = p;
            _ = try c.onTyped(UserCreated, holder, Sink.onEvent);
        }
    };
    S.holder = &sink;

    const p = cordis.Plugin{ .name = "listener", .apply = S.apply };
    const fiber = try p.start(ctx, null);
    ctx.emitTyped(UserCreated, &.{ .id = 1 });
    try std.testing.expectEqual(@as(i32, 1), sink.n);

    fiber.dispose();
    ctx.emitTyped(UserCreated, &.{ .id = 2 });
    try std.testing.expectEqual(@as(i32, 1), sink.n);
}

test "comptime plugin with typed config and inject" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const State = struct {
        var applies: i32 = 0;
        var last_config: i32 = 0;

        fn apply(c: *Context, config: i32) cordis.Error!void {
            _ = c;
            applies += 1;
            last_config = config;
        }
    };

    const deps = [_][]const u8{@typeName(Database)};
    const Consumer = cordis.TypedPlugin("consumer", i32, State.apply, &deps);

    const fiber = try Consumer.start(ctx, 7);
    try std.testing.expectEqual(cordis.FiberState.pending, fiber.state());
    try std.testing.expectEqual(@as(i32, 0), State.applies);

    const db = Database{ .dsn = "live" };
    _ = try ctx.provide(&db);
    try std.testing.expectEqual(cordis.FiberState.active, fiber.state());
    try std.testing.expectEqual(@as(i32, 7), State.last_config);

    // Two starts of one plugin type share one registry identity.
    const second = try Consumer.start(ctx, 9);
    try std.testing.expectEqual(cordis.FiberState.active, second.state());
    try std.testing.expectEqual(@as(i32, 9), State.last_config);

    // The config copy lives in the arena: update through the fiber handle.
    const ten: i32 = 10;
    try fiber.update(cordis.value(&ten));
    try std.testing.expectEqual(@as(i32, 10), State.last_config);
}

test "comptime plugin error fails the fiber" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const faulty = cordis.TypedPlugin("faulty", void, struct {
        fn apply(_: *Context, _: void) cordis.Error!void {
            return cordis.Error.PluginFailed;
        }
    }.apply, &.{});

    const fiber = try faulty.start(ctx, {});
    try std.testing.expectEqual(cordis.FiberState.failed, fiber.state());
    try std.testing.expectEqual(@as(usize, 1), ctx.loggedErrors().len);
}

test "effect scope collects, introspects and rolls back" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const Sink = struct {
        n: i32 = 0,
        fn onEvent(self: *@This(), _: UserCreated) void {
            self.n += 1;
        }
    };
    var sink = Sink{};

    const Ctx = struct {
        fn inner(c: *Context, s: *Sink) cordis.Error!void {
            _ = try c.onTyped(UserCreated, s, Sink.onEvent);
        }
    };

    const eff = try ctx.effect("user-listeners", &sink, Ctx.inner);
    ctx.emitTyped(UserCreated, &.{ .id = 1 });
    try std.testing.expectEqual(@as(i32, 1), sink.n);
    try std.testing.expectEqual(@as(usize, 1), ctx.effects().len);
    try std.testing.expectEqualStrings("user-listeners", ctx.effects()[0].label);
    try std.testing.expect(!ctx.effects()[0].disposed);

    eff.dispose();
    ctx.emitTyped(UserCreated, &.{ .id = 2 });
    try std.testing.expectEqual(@as(i32, 1), sink.n);
    try std.testing.expect(ctx.effects()[0].disposed);

    // Disposing twice is a no-op.
    eff.dispose();
    try std.testing.expectEqual(@as(i32, 1), sink.n);
}

test "effect scope error rolls back its registrations" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const db = Database{ .dsn = "x" };
    const Ctx = struct {
        fn inner(c: *Context, d: *const Database) cordis.Error!void {
            _ = try c.provide(d);
            return cordis.Error.PluginFailed;
        }
    };

    try std.testing.expectError(cordis.Error.PluginFailed, ctx.effect("boom", &db, Ctx.inner));
    try std.testing.expect(ctx.getTyped(Database) == null);
}

test "onceTyped delivers exactly one event" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const Sink = struct {
        n: i32 = 0,
        fn onEvent(self: *@This(), _: UserCreated) void {
            self.n += 1;
        }
    };
    var sink = Sink{};

    try ctx.onceTyped(UserCreated, &sink, Sink.onEvent);
    ctx.emitTyped(UserCreated, &.{ .id = 1 });
    ctx.emitTyped(UserCreated, &.{ .id = 2 });
    try std.testing.expectEqual(@as(i32, 1), sink.n);

    // A different event type does not trigger the once listener; it is
    // already detached anyway.
    ctx.emitTyped(UserDeleted, &.{ .id = 3 });
    try std.testing.expectEqual(@as(i32, 1), sink.n);
}

test "registry view counts and deletes plugins" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const State = struct {
        fn apply(_: *Context, _: void) cordis.Error!void {}
    };
    const A = cordis.TypedPlugin("a", void, State.apply, &.{});
    const B = cordis.TypedPlugin("b", void, State.apply, &.{});

    const reg = ctx.registry();
    try std.testing.expectEqual(@as(usize, 0), reg.size());

    const fa = try A.start(ctx, {});
    _ = try B.start(ctx, {});
    try std.testing.expectEqual(@as(usize, 2), reg.size());
    try std.testing.expect(reg.has(&A.view));
    try std.testing.expect(reg.has(&B.view));

    reg.delete(&A.view);
    try std.testing.expectEqual(cordis.FiberState.disposed, fa.state());
    try std.testing.expect(!reg.has(&A.view));
    try std.testing.expectEqual(@as(usize, 1), reg.size());
}

const UserRenamed = struct {
    from: []const u8,
    to: []const u8,
};

test "serial stops at the first non-null result" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const Probe = struct {
        ran_second: bool = false,
        first: ?cordis.Value = null,
        second: ?cordis.Value = null,

        fn onFirst(self: *@This(), _: []const cordis.Value) ?cordis.Value {
            return self.first;
        }
        fn onSecond(self: *@This(), _: []const cordis.Value) ?cordis.Value {
            self.ran_second = true;
            return self.second;
        }
    };
    var probe = Probe{};
    _ = try ctx.onNamed("probe", cordis.Listener.bind(Probe, &probe, Probe.onFirst));
    _ = try ctx.onNamed("probe", cordis.Listener.bind(Probe, &probe, Probe.onSecond));
    try std.testing.expect(ctx.serial("probe", &.{}) == null);
    try std.testing.expect(probe.ran_second);

    const marker = ctx.core.a().create(i32) catch unreachable;
    marker.* = 1;
    probe.first = marker;
    probe.ran_second = false;
    const got = ctx.serial("probe", &.{});
    try std.testing.expect(got != null);
    try std.testing.expectEqual(@intFromPtr(marker), @intFromPtr(got.?));
    try std.testing.expect(!probe.ran_second);
}

test "waterfall composes listeners around a terminal" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const ListenerState = struct {
        fn call(raw: *anyopaque, args: []const cordis.Value) ?cordis.Value {
            _ = raw;
            const next: *cordis.Context.Next = @constCast(@ptrCast(@alignCast(args[args.len - 1])));
            const n: *i32 = @constCast(@ptrCast(@alignCast(args[0])));
            n.* *= 2;
            return next.invoke(args[0 .. args.len - 1]);
        }
    };
    var dummy: u8 = 0;
    _ = try ctx.onNamed("wf", .{ .ctx = @ptrCast(&dummy), .call = &ListenerState.call });

    const Terminal = struct {
        fn run(args: []const cordis.Value) ?cordis.Value {
            const n: *i32 = @constCast(@ptrCast(@alignCast(args[0])));
            n.* += 1;
            return args[0];
        }
    };

    const n = try ctx.core.a().create(i32);
    n.* = 20;
    const result = ctx.waterfall("wf", &.{cordis.value(n)}, &Terminal.run);
    try std.testing.expectEqual(@as(i32, 41), @as(*const i32, @ptrCast(@alignCast(result.?))).*);
}

test "waterfall without listeners runs the terminal" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();
    const Terminal = struct {
        fn run(_: []const cordis.Value) ?cordis.Value {
            return null;
        }
    };
    try std.testing.expect(ctx.waterfall("none", &.{}, &Terminal.run) == null);
}

test "parallel delivers to every listener and batch coalesces" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const Counter = struct {
        n: usize = 0,
        fn onEvent(self: *@This(), _: UserCreated) void {
            self.n += 1;
        }
    };
    var a = Counter{};
    var b = Counter{};
    _ = try ctx.onTyped(UserCreated, &a, Counter.onEvent);
    _ = try ctx.onTyped(UserCreated, &b, Counter.onEvent);

    const Batcher = struct {
        ctx: *Context,
        fn run(self: *@This()) void {
            self.ctx.emitTyped(UserCreated, &.{ .id = 1 });
        }
    };
    var batcher = Batcher{ .ctx = ctx };
    ctx.batch(&batcher, Batcher.run);

    try std.testing.expectEqual(@as(usize, 1), a.n);
    try std.testing.expectEqual(@as(usize, 1), b.n);
}
