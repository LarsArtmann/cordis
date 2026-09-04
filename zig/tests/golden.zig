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

const Runner = struct {
    gpa: std.mem.Allocator,
    trace: std.ArrayListUnmanaged([]const u8) = .empty,
    fibers: std.StringHashMapUnmanaged(Fiber) = .empty,

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
    }

    fn applyProvider(c: *Context, service: []const u8) cordis.Error!void {
        const stored = c.core.a().create(i32) catch @panic("cordis: out of memory");
        stored.* = 1;
        try c.provideNamed(service, cordis.value(stored));
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

fn startLogical(r: *Runner, ctx: *Context, name: []const u8, params: Params) !void {
    const body = ctx.core.a().create(PluginBody) catch @panic("cordis: out of memory");
    body.* = .{ .runner = r, .name = name, .lifo = params.lifo };
    const p = ctx.core.a().create(cordis.Plugin) catch @panic("cordis: out of memory");
    p.* = .{ .name = name, .inject = params.deps, .apply = genericApply, .data = body };
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
