//! Cross-language golden scenario runner: executes the scenario described
//! by golden/scenario.txt (path injected via build options) and asserts the
//! emitted trace matches golden/expected.txt exactly. The Go and Rust ports
//! ship structurally identical runners; see golden/README.md.

const std = @import("std");
const cordis = @import("cordis");

// The shared scenario and its expected trace, embedded at build time from
// ../golden/ so the runner needs no runtime file access.
const golden_data = @import("golden_data");

const Context = cordis.Context;
const Fiber = cordis.Fiber;

const ChildSpec = struct {
    name: []const u8,
    deps: []const []const u8 = &.{},
    config: i32 = 0,
};

const Runner = struct {
    gpa: std.mem.Allocator,
    trace: std.ArrayListUnmanaged([]const u8) = .empty,
    fibers: std.StringHashMapUnmanaged(Fiber) = .empty,
    plugins: std.StringHashMapUnmanaged(*const cordis.Plugin) = .empty,
    children: std.StringHashMapUnmanaged([]const ChildSpec) = .empty,

    fn log(self: *Runner, comptime format: []const u8, args: anytype) void {
        const line = std.fmt.allocPrint(self.gpa, format, args) catch @panic("cordis: out of memory");
        self.trace.append(self.gpa, line) catch @panic("cordis: out of memory");
    }

    fn applyPlugin(self: *Runner, c: *Context, config: i32, body: *const PluginBody) cordis.Error!void {
        const name = body.name;
        self.log("apply {s} config={d}", .{ name, config });
        var i: usize = 1;
        const count: usize = if (body.lifo) 3 else 1;
        while (i <= count) : (i += 1) {
            const label = if (body.lifo)
                std.fmt.allocPrint(self.gpa, "{s}#{d}", .{ name, i }) catch @panic("cordis: out of memory")
            else
                name;
            const entry = self.gpa.create(CleanupEntry) catch @panic("cordis: out of memory");
            entry.* = .{ .runner = self, .label = label };
            try c.attach(entry, CleanupEntry.run);
        }
        for (body.children) |spec| {
            startLogical(self, c, spec.name, .{ .deps = spec.deps, .config = spec.config }) catch @panic("golden: child start failed");
        }
    }

    fn applyProvider(c: *Context, service: []const u8) cordis.Error!void {
        const stored = c.core.a().create(i32) catch @panic("cordis: out of memory");
        stored.* = 1;
        _ = try c.provideNamed(service, cordis.value(stored));
    }
};

const CleanupEntry = struct {
    runner: *Runner,
    label: []const u8,

    fn run(entry: *CleanupEntry) void {
        entry.runner.log("cleanup {s}", .{entry.label});
    }
};

// Runtime plugin plumbing: each logical plugin owns a PluginBody stored in
// the tree arena and referenced through Plugin.data, so applies always see
// their own name even when dependency reactivation defers them.
const PluginBody = struct {
    runner: *Runner,
    name: []const u8,
    lifo: bool = false,
    children: []const ChildSpec = &.{},
};

const ProviderBody = struct {
    service: []const u8,
};

fn genericApply(plugin: *const cordis.Plugin, c: *Context, raw: ?cordis.Value) cordis.Error!void {
    const body: *const PluginBody = @ptrCast(@alignCast(plugin.data.?));
    const config: i32 = if (raw) |v| @as(*const i32, @ptrCast(@alignCast(v))).* else 0;
    return body.runner.applyPlugin(c, config, body);
}

fn providerApply(plugin: *const cordis.Plugin, c: *Context, _: ?cordis.Value) cordis.Error!void {
    const body: *const ProviderBody = @ptrCast(@alignCast(plugin.data.?));
    return Runner.applyProvider(c, body.service);
}

const Params = struct {
    deps: []const []const u8 = &.{},
    realm: []const u8 = "",
    config: i32 = 0,
    lifo: bool = false,
};

fn parseParams(gpa: std.mem.Allocator, tokens: []const []const u8) Params {
    var p = Params{};
    for (tokens) |tok| {
        if (std.mem.startsWith(u8, tok, "inject=")) {
            const rest = tok["inject=".len..];
            var deps: std.ArrayListUnmanaged([]const u8) = .empty;
            var it = std.mem.splitScalar(u8, rest, ',');
            while (it.next()) |dep| {
                if (dep.len > 0) deps.append(gpa, dep) catch @panic("cordis: out of memory");
            }
            p.deps = deps.toOwnedSlice(gpa) catch @panic("cordis: out of memory");
        } else if (std.mem.startsWith(u8, tok, "realm=")) {
            p.realm = tok["realm=".len..];
        } else if (std.mem.startsWith(u8, tok, "config=")) {
            p.config = std.fmt.parseInt(i32, tok["config=".len..], 10) catch @panic("bad config");
        } else if (std.mem.eql(u8, tok, "lifo")) {
            p.lifo = true;
        }
    }
    return p;
}

fn stateName(state: cordis.FiberState) []const u8 {
    return switch (state) {
        .pending => "PENDING",
        .loading => "LOADING",
        .active => "ACTIVE",
        .failed => "FAILED",
        .disposed => "DISPOSED",
        .unloading => "UNLOADING",
    };
}

fn getOrCreatePlugin(r: *Runner, ctx: *Context, name: []const u8, params: Params) *const cordis.Plugin {
    if (r.plugins.get(name)) |p| return p;
    const body = ctx.core.a().create(PluginBody) catch @panic("cordis: out of memory");
    body.* = .{
        .runner = r,
        .name = name,
        .lifo = params.lifo,
        .children = r.children.get(name) orelse &.{},
    };
    const p = ctx.core.a().create(cordis.Plugin) catch @panic("cordis: out of memory");
    p.* = .{ .name = name, .inject = params.deps, .apply = genericApply, .data = body };
    r.plugins.put(r.gpa, name, p) catch @panic("cordis: out of memory");
    return p;
}

fn startLogical(r: *Runner, ctx: *Context, name: []const u8, params: Params) !void {
    const p = getOrCreatePlugin(r, ctx, name, params);
    const config = ctx.core.a().create(i32) catch @panic("cordis: out of memory");
    config.* = params.config;
    const fiber = try p.start(ctx, cordis.value(config));
    try r.fibers.put(r.gpa, name, fiber);
}

fn startProvider(r: *Runner, ctx: *Context, service: []const u8) !void {
    const body = ctx.core.a().create(ProviderBody) catch @panic("cordis: out of memory");
    body.* = .{ .service = service };
    const name = std.fmt.allocPrint(ctx.core.a(), "provider:{s}", .{service}) catch @panic("cordis: out of memory");
    const p = ctx.core.a().create(cordis.Plugin) catch @panic("cordis: out of memory");
    p.* = .{ .name = name, .apply = providerApply, .data = body };
    const fiber = try p.start(ctx, null);
    const key = std.fmt.allocPrint(r.gpa, "provider:{s}", .{service}) catch @panic("cordis: out of memory");
    try r.fibers.put(r.gpa, key, fiber);
}

fn readScenarioLines(gpa: std.mem.Allocator, text: []const u8) ![][]const u8 {
    var lines: std.ArrayListUnmanaged([]const u8) = .empty;
    var it = std.mem.splitScalar(u8, text, '\n');
    while (it.next()) |raw| {
        const line = std.mem.trim(u8, raw, " \t\r");
        if (line.len == 0 or line[0] == '#') continue;
        try lines.append(gpa, line);
    }
    return lines.toOwnedSlice(gpa);
}

test "golden scenario events" {
    const gpa = std.testing.allocator;
    var arena = std.heap.ArenaAllocator.init(gpa);
    defer arena.deinit();
    const alloc = arena.allocator();

    const scenario = try readScenarioLines(alloc, golden_data.scenario_events);
    const expected = try readScenarioLines(alloc, golden_data.expected_events);

    const ctx = try Context.init(gpa);
    defer ctx.deinit();

    var r = Runner{ .gpa = alloc };

    const Sink = struct {
        runner: *Runner,
        prefix: []const u8,

        fn fire(self: *@This(), args: []const cordis.Value) ?cordis.Value {
            const payload: i32 = if (args.len > 0)
                @as(*const i32, @ptrCast(@alignCast(args[0]))).*
            else
                0;
            self.runner.log("{s}{d}", .{ self.prefix, payload });
            return null;
        }
    };

    for (scenario) |line| {
        var tokens = std.mem.tokenizeAny(u8, line, " \t");
        const op = tokens.next().?;
        const event = tokens.next().?;
        var payload: i32 = 0;
        var realm: []const u8 = "";
        while (tokens.next()) |tok| {
            if (std.mem.startsWith(u8, tok, "payload=")) {
                payload = std.fmt.parseInt(i32, tok["payload=".len..], 10) catch @panic("bad payload");
            } else if (std.mem.startsWith(u8, tok, "realm=")) {
                realm = tok["realm=".len..];
            }
        }

        if (std.mem.eql(u8, op, "on") or std.mem.eql(u8, op, "on-isolated")) {
            var scope: *Context = ctx;
            var prefix: []const u8 = undefined;
            if (realm.len > 0) {
                scope = ctx.isolateShared(event, realm);
                prefix = std.fmt.allocPrint(alloc, "fired {s} realm={s} payload=", .{ event, realm }) catch @panic("cordis: out of memory");
            } else {
                prefix = std.fmt.allocPrint(alloc, "fired {s} root payload=", .{event}) catch @panic("cordis: out of memory");
            }
            const sink = alloc.create(Sink) catch @panic("cordis: out of memory");
            sink.* = .{ .runner = &r, .prefix = prefix };
            _ = try scope.onNamed(event, cordis.Listener.bind(Sink, sink, Sink.fire));
        } else if (std.mem.eql(u8, op, "on-global")) {
            const prefix = std.fmt.allocPrint(alloc, "fired {s} global payload=", .{event}) catch @panic("cordis: out of memory");
            const sink = alloc.create(Sink) catch @panic("cordis: out of memory");
            sink.* = .{ .runner = &r, .prefix = prefix };
            _ = try ctx.onGlobal(event, cordis.Listener.bind(Sink, sink, Sink.fire));
        } else if (std.mem.eql(u8, op, "emit")) {
            const stored = try ctx.core.a().create(i32);
            stored.* = payload;
            ctx.emitNamed(event, &.{cordis.value(stored)});
        } else if (std.mem.eql(u8, op, "emit-filtered")) {
            const scope = ctx.isolateShared(event, realm);
            const emitter = scope.withFilter(scope.realmFilter(scope, event));
            const stored = try ctx.core.a().create(i32);
            stored.* = payload;
            emitter.emitNamed(event, &.{cordis.value(stored)});
        } else {
            std.debug.print("unknown op {s}\n", .{op});
            return error.GoldenUnknownOp;
        }
    }

    if (r.trace.items.len != expected.len) {
        std.debug.print("trace length {d} != expected {d}\ntrace:\n", .{ r.trace.items.len, expected.len });
        for (r.trace.items) |entry| std.debug.print("{s}\n", .{entry});
        return error.GoldenLengthMismatch;
    }
    for (r.trace.items, expected, 0..) |got, want, i| {
        if (!std.mem.eql(u8, got, want)) {
            std.debug.print("trace divergence at line {d}: expected '{s}', got '{s}'\n", .{ i + 1, want, got });
            return error.GoldenDivergence;
        }
    }
}

test "golden scenario" {
    const gpa = std.testing.allocator;
    var arena = std.heap.ArenaAllocator.init(gpa);
    defer arena.deinit();
    const alloc = arena.allocator();

    const scenario = try readScenarioLines(alloc, golden_data.scenario);
    const expected = try readScenarioLines(alloc, golden_data.expected);

    const ctx = try Context.init(gpa);
    defer ctx.deinit();

    var r = Runner{ .gpa = alloc };

    for (scenario) |line| {
        var tokens = std.mem.tokenizeAny(u8, line, " \t");
        const op = tokens.next().?;
        var args: std.ArrayListUnmanaged([]const u8) = .empty;
        while (tokens.next()) |tok| try args.append(alloc, tok);

        if (std.mem.eql(u8, op, "provide") or std.mem.eql(u8, op, "provide-in-realm")) {
            const params = parseParams(alloc, args.items);
            var scope: *Context = ctx;
            if (params.realm.len > 0) scope = ctx.isolateShared(args.items[0], params.realm);
            try startProvider(&r, scope, args.items[0]);
            r.log("provided {s}", .{args.items[0]});
        } else if (std.mem.eql(u8, op, "withdraw") or std.mem.eql(u8, op, "withdraw-in-realm")) {
            const key = try std.fmt.allocPrint(alloc, "provider:{s}", .{args.items[0]});
            const fiber = r.fibers.get(key).?;
            fiber.dispose();
            r.log("withdrawn {s}", .{args.items[0]});
        } else if (std.mem.eql(u8, op, "start") or std.mem.eql(u8, op, "start-isolated")) {
            const params = parseParams(alloc, args.items);
            var scope: *Context = ctx;
            if (std.mem.eql(u8, op, "start-isolated")) {
                for (params.deps) |dep| scope = scope.isolateShared(dep, params.realm);
            }
            try startLogical(&r, scope, args.items[0], params);
        } else if (std.mem.eql(u8, op, "update")) {
            const fiber = r.fibers.get(args.items[0]).?;
            const config = try std.fmt.parseInt(i32, args.items[1], 10);
            const stored = try ctx.core.a().create(i32);
            stored.* = config;
            try fiber.update(cordis.value(stored));
        } else if (std.mem.eql(u8, op, "restart")) {
            try r.fibers.get(args.items[0]).?.restart();
        } else if (std.mem.eql(u8, op, "dispose")) {
            r.fibers.get(args.items[0]).?.dispose();
            r.log("disposed {s}", .{args.items[0]});
        } else if (std.mem.eql(u8, op, "restart-root")) {
            ctx.fiberHandle().dispose();
            r.log("root-restarted", .{});
        } else if (std.mem.eql(u8, op, "spawn")) {
            var parent: []const u8 = "";
            for (args.items) |tok| {
                if (std.mem.startsWith(u8, tok, "parent=")) parent = tok["parent=".len..];
            }
            const params = parseParams(alloc, args.items[1..]);
            const spec = alloc.create(ChildSpec) catch @panic("cordis: out of memory");
            spec.* = .{ .name = args.items[0], .deps = params.deps, .config = params.config };
            const existing = r.children.get(parent) orelse &.{};
            var list: std.ArrayListUnmanaged(ChildSpec) = .empty;
            list.appendSlice(alloc, existing) catch @panic("cordis: out of memory");
            list.append(alloc, spec.*) catch @panic("cordis: out of memory");
            r.children.put(alloc, parent, list.toOwnedSlice(alloc) catch @panic("cordis: out of memory")) catch @panic("cordis: out of memory");
        } else if (std.mem.eql(u8, op, "delete")) {
            const p = r.plugins.get(args.items[0]).?;
            ctx.registry().delete(p);
            r.log("deleted {s}", .{args.items[0]});
        } else if (std.mem.eql(u8, op, "expect-registry-size")) {
            const want = try std.fmt.parseInt(usize, args.items[0], 10);
            const got = ctx.registry().size();
            if (got != want) {
                std.debug.print("expected registry size {d}, got {d}\n", .{ want, got });
                return error.GoldenRegistrySizeMismatch;
            }
            r.log("registry-size {d}", .{want});
        } else if (std.mem.eql(u8, op, "expect-state")) {
            const fiber = r.fibers.get(args.items[0]).?;
            const got = stateName(fiber.state());
            if (!std.mem.eql(u8, got, args.items[1])) {
                std.debug.print("expected {s} {s}, got {s}\ntrace:\n", .{ args.items[0], args.items[1], got });
                for (r.trace.items) |entry| std.debug.print("{s}\n", .{entry});
                return error.GoldenStateMismatch;
            }
            r.log("state {s} {s}", .{ args.items[0], args.items[1] });
        } else {
            std.debug.print("unknown op {s}\n", .{op});
            return error.GoldenUnknownOp;
        }
    }

    if (r.trace.items.len != expected.len) {
        std.debug.print("trace length {d} != expected {d}\ntrace:\n", .{ r.trace.items.len, expected.len });
        for (r.trace.items) |entry| std.debug.print("{s}\n", .{entry});
        return error.GoldenLengthMismatch;
    }
    for (r.trace.items, expected, 0..) |got, want, i| {
        if (!std.mem.eql(u8, got, want)) {
            std.debug.print("trace divergence at line {d}: expected '{s}', got '{s}'\n", .{ i + 1, want, got });
            return error.GoldenDivergence;
        }
    }
}

test "golden dsl parseParams" {
    const alloc = std.testing.allocator;
    var arena = std.heap.ArenaAllocator.init(alloc);
    defer arena.deinit();
    const a = arena.allocator();

    const tokens = [_][]const u8{ "inject=a,,b", "realm=tenant", "config=7", "lifo", "ignored" };
    const p = parseParams(a, &tokens);
    try std.testing.expectEqual(@as(usize, 2), p.deps.len);
    try std.testing.expectEqualStrings("a", p.deps[0]);
    try std.testing.expectEqualStrings("b", p.deps[1]);
    try std.testing.expectEqualStrings("tenant", p.realm);
    try std.testing.expectEqual(@as(i32, 7), p.config);
    try std.testing.expect(p.lifo);

    const empty = parseParams(a, &.{});
    try std.testing.expectEqual(@as(usize, 0), empty.deps.len);
    try std.testing.expectEqual(@as(i32, 0), empty.config);
    try std.testing.expect(!empty.lifo);
}

test "golden dsl scenario line parsing" {
    const alloc = std.testing.allocator;
    var arena = std.heap.ArenaAllocator.init(alloc);
    defer arena.deinit();
    const a = arena.allocator();

    const lines = try readScenarioLines(a, "# comment\n\nstart worker inject=a config=1\n");
    try std.testing.expectEqual(@as(usize, 1), lines.len);
    try std.testing.expectEqualStrings("start worker inject=a config=1", lines[0]);

    var it = std.mem.tokenizeAny(u8, lines[0], " \t");
    try std.testing.expectEqualStrings("start", it.next().?);
    try std.testing.expectEqualStrings("worker", it.next().?);
}
