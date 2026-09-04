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

    try ctx.provide(&db);
    try std.testing.expectEqualStrings("postgres://localhost", ctx.getTyped(Database).?.dsn);
    try std.testing.expect(ctx.getTyped(u32) == null);
}

test "typed service duplicate and realm isolation" {
    const ctx = try Context.init(std.testing.allocator);
    defer ctx.deinit();

    const root_db = Database{ .dsn = "root" };
    const iso_db = Database{ .dsn = "isolated" };

    try ctx.provide(&root_db);
    try std.testing.expectError(cordis.Error.DuplicateService, ctx.provide(&iso_db));

    const isolated = ctx.isolate(@typeName(Database));
    try isolated.provide(&iso_db);
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

    try ctx.onTyped(UserCreated, &sink, Sink.onCreated);
    try ctx.onTyped(UserDeleted, &sink, Sink.onDeleted);

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
            try c.onTyped(UserCreated, holder, Sink.onEvent);
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
    try ctx.provide(&db);
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
