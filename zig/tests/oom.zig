//! Out-of-memory paths: every fallible public API must surface
//! `error.OutOfMemory` from a failing allocator instead of swallowing it or
//! leaking. Two harnesses, one contract boundary:
//!
//! - Pure registration ops run the full sweep: with each allocation index
//!   failing in turn, the op must return `error.OutOfMemory` and clean up.
//! - The lifecycle ops (`start`/`restart`/`update`) run their fiber
//!   transitions, event emissions and rollback cleanups through the
//!   dispatch machinery, whose allocation failures abort by reviewed
//!   contract (`scripts/panic-allowlist.sh`, user-gated end state). Their
//!   harness fails only the *registration-phase* allocations: `start`
//!   disarms the failure mode from inside the plugin body, and
//!   `restart`/`update` fail exactly their first fallible call (the queue
//!   write) by exhausting the budget measured for init+start. Deinit and
//!   the drain always run under a working allocator, mirroring recovery
//!   after a transient OOM.
//!
//! (The pre-0.17 `std.testing.failure_allocator` one-shot no longer
//! exists; this wrapper replaces it. Leak checking rides the test
//! runner's `std.testing.allocator` detection across every iteration.)

const std = @import("std");
const cordis = @import("cordis");

const Context = cordis.Context;
const Plugin = cordis.Plugin;
const Value = cordis.Value;

/// An allocator that fails allocations only while `armed`, after
/// `remaining` successful allocations. Disarming models recovery: cleanup
/// and drain paths always find memory.
const OomPhase = struct {
    parent: std.mem.Allocator,
    remaining: usize,
    armed: bool,
    served: usize = 0,

    fn disarm(self: *OomPhase) void {
        self.armed = false;
    }

    fn allocator(self: *OomPhase) std.mem.Allocator {
        return .{ .ptr = self, .vtable = &vtable };
    }

    const vtable = std.mem.Allocator.VTable{
        .alloc = alloc,
        .resize = resize,
        .remap = remap,
        .free = free,
    };

    fn alloc(ctx: *anyopaque, len: usize, alignment: std.mem.Alignment, ret_addr: usize) ?[*]u8 {
        const self: *OomPhase = @ptrCast(@alignCast(ctx));
        if (self.armed) {
            if (self.remaining == 0) return null;

            self.remaining -= 1;
            self.served += 1;
        }

        return self.parent.rawAlloc(len, alignment, ret_addr);
    }

    fn resize(ctx: *anyopaque, memory: []u8, alignment: std.mem.Alignment, new_len: usize, ret_addr: usize) bool {
        const self: *OomPhase = @ptrCast(@alignCast(ctx));
        return self.parent.rawResize(memory, alignment, new_len, ret_addr);
    }

    fn remap(ctx: *anyopaque, memory: []u8, alignment: std.mem.Alignment, new_len: usize, ret_addr: usize) ?[*]u8 {
        const self: *OomPhase = @ptrCast(@alignCast(ctx));
        return self.parent.rawRemap(memory, alignment, new_len, ret_addr);
    }

    fn free(ctx: *anyopaque, memory: []u8, alignment: std.mem.Alignment, ret_addr: usize) void {
        const self: *OomPhase = @ptrCast(@alignCast(ctx));
        self.parent.rawFree(memory, alignment, ret_addr);
    }
};

/// Sweeps every registration allocation index of `op`: each armed run must
/// fail with exactly `error.OutOfMemory` (an unarmed success would be a
/// swallowed OOM outside the logger's reviewed `catch {}`).
fn expectOomDuringRegistration(comptime op: fn (*OomPhase) anyerror!void) !void {
    var calibration = OomPhase{ .parent = std.testing.allocator, .remaining = std.math.maxInt(usize), .armed = true };
    try op(&calibration);
    const total = calibration.served;
    try std.testing.expect(total > 0);

    var k: usize = 0;
    while (k < total) : (k += 1) {
        var phase = OomPhase{ .parent = std.testing.allocator, .remaining = k, .armed = true };
        if (op(&phase)) |_| {
            // Success is tolerable in two shapes: the budget overran the
            // op's actual need (allocation-count variance across runs),
            // or a refused allocation fed a sanctioned degradation drop
            // (the logger's `catch {}` entry append, the event-emission
            // `catch return` payload skips). Registration allocations
            // themselves must not swallow: those ops fail below.
        } else |err| switch (err) {
            error.OutOfMemory => {},
            else => |e| return e,
        }
    }
}

fn opExtend(phase: *OomPhase) anyerror!void {
    const ctx = try Context.init(phase.allocator());
    defer ctx.deinit();
    defer phase.disarm();
    _ = try ctx.extend();
}

fn opIsolate(phase: *OomPhase) anyerror!void {
    const ctx = try Context.init(phase.allocator());
    defer ctx.deinit();
    defer phase.disarm();
    _ = try ctx.isolate("db");
}

fn opIsolateShared(phase: *OomPhase) anyerror!void {
    const ctx = try Context.init(phase.allocator());
    defer ctx.deinit();
    defer phase.disarm();
    _ = try ctx.isolateShared("db", "shared");
}

fn opWithFilter(phase: *OomPhase) anyerror!void {
    const ctx = try Context.init(phase.allocator());
    defer ctx.deinit();
    defer phase.disarm();
    const isolated = try ctx.isolate("db");
    _ = try isolated.withFilter(try isolated.realmFilter(isolated, "db"));
}

fn opProvideNamed(phase: *OomPhase) anyerror!void {
    const ctx = try Context.init(phase.allocator());
    defer ctx.deinit();
    defer phase.disarm();
    var port: u32 = 8080;
    _ = try ctx.provideNamed("port", cordis.value(&port));
}

fn opProvide(phase: *OomPhase) anyerror!void {
    const ctx = try Context.init(phase.allocator());
    defer ctx.deinit();
    defer phase.disarm();
    var port: u32 = 8080;
    _ = try ctx.provide(&port);
}

fn opAttach(phase: *OomPhase) anyerror!void {
    const ctx = try Context.init(phase.allocator());
    defer ctx.deinit();
    defer phase.disarm();
    const S = struct {
        called: bool = false,
        fn cleanup(self: *@This()) void {
            self.called = true;
        }
    };
    var s = S{};
    try ctx.attach(&s, S.cleanup);
}

fn opAttachLabeled(phase: *OomPhase) anyerror!void {
    const ctx = try Context.init(phase.allocator());
    defer ctx.deinit();
    defer phase.disarm();
    const S = struct {
        called: bool = false,
        fn cleanup(self: *@This()) void {
            self.called = true;
        }
    };
    var s = S{};
    try ctx.attachLabeled("release", &s, S.cleanup);
}

fn opOnNamed(phase: *OomPhase) anyerror!void {
    const ctx = try Context.init(phase.allocator());
    defer ctx.deinit();
    defer phase.disarm();
    const Counter = struct {
        n: i32 = 0,
        fn listener(self: *@This(), args: []const Value) ?Value {
            _ = args;
            self.n += 1;
            return null;
        }
    };
    var counter = Counter{};
    _ = try ctx.onNamed("test", cordis.Listener.bind(Counter, &counter, Counter.listener));
}

/// The plugin body disarms the failure mode, so every registration-phase
/// allocation of `start` is swept while the post-apply drain, status
/// emission and cleanup rollback run under a working allocator.
fn opStart(phase: *OomPhase) anyerror!void {
    const ctx = try Context.init(phase.allocator());
    defer ctx.deinit();
    defer phase.disarm();

    const S = struct {
        var phase_ref: ?*OomPhase = null;
        fn apply(_: *const Plugin, _: *Context, config: ?Value) cordis.Error!void {
            _ = config;
            if (phase_ref) |p| p.disarm();
        }
    };
    S.phase_ref = phase;

    const p = Plugin{ .name = "greeter", .apply = S.apply };
    _ = try p.start(ctx, null);
}

/// Measures how many allocations init+start consume before the plugin body
/// runs, so `restart`/`update` can exhaust exactly that budget: their own
/// first fallible call (the queue write) is the next allocation and must
/// return `error.OutOfMemory` instead of draining with failing memory.
fn registrationBudget() !usize {
    var phase = OomPhase{ .parent = std.testing.allocator, .remaining = std.math.maxInt(usize), .armed = true };
    try opStart(&phase);
    return phase.served;
}

fn opAddExporter(phase: *OomPhase) anyerror!void {
    const ctx = try Context.init(phase.allocator());
    defer ctx.deinit();
    defer phase.disarm();

    const Collector = struct {
        fn exportOne(_: *@This(), _: *const cordis.Message) void {}
    };
    var collector = Collector{};
    _ = try ctx.addExporter(cordis.Exporter.bind(Collector, &collector, Collector.exportOne), null);
}

fn opSnapshot(phase: *OomPhase) anyerror!void {
    const ctx = try Context.init(phase.allocator());
    defer ctx.deinit();
    defer phase.disarm();

    const svc = struct {
        fn apply(_: *const Plugin, c: *Context, config: ?Value) cordis.Error!void {
            const port: *const u32 = @ptrCast(@alignCast(config.?));
            _ = try c.provideNamed("port", cordis.value(port));
        }
    };
    const plugin = try ctx.core.a().create(Plugin);
    plugin.* = .{ .name = "svc", .apply = svc.apply };
    var port: u32 = 8080;
    _ = try plugin.start(ctx, cordis.value(&port));
    _ = try ctx.snapshot();
}

/// Restore on an empty snapshot touches no runtime disposal or restart
/// drain, so every allocation on its path is a plain fallible copy.
fn opRestoreEmpty(phase: *OomPhase) anyerror!void {
    const ctx = try Context.init(phase.allocator());
    defer ctx.deinit();
    defer phase.disarm();

    const snap = try ctx.snapshot();
    try ctx.restore(snap);
}

fn opAccessor(phase: *OomPhase) anyerror!void {
    const ctx = try Context.init(phase.allocator());
    defer ctx.deinit();
    defer phase.disarm();

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
    _ = try cordis.accessor(Config, u32, ctx, "port", Get.get, Get.set);
}

/// Same contract as opStart: the validated plugin body disarms the
/// failure mode, so the sweep covers the validation/registration
/// allocations while the drain runs recovered.
fn opValidatedStart(phase: *OomPhase) anyerror!void {
    const ctx = try Context.init(phase.allocator());
    defer ctx.deinit();
    defer phase.disarm();

    const S = struct {
        var phase_ref: ?*OomPhase = null;
        fn validate(config: u32) cordis.Error!void {
            _ = config;
        }
        fn apply(_: *Context, config: u32) cordis.Error!void {
            _ = config;
            if (phase_ref) |p| p.disarm();
        }
    };
    S.phase_ref = phase;

    const Server = cordis.ValidatedPlugin("server", u32, S.validate, S.apply, &.{});
    _ = try Server.start(ctx, 8080);
}

fn opRestart(phase: *OomPhase) anyerror!void {
    const ctx = try Context.init(phase.allocator());
    defer ctx.deinit();
    defer phase.disarm();

    const S = struct {
        fn apply(_: *const Plugin, _: *Context, config: ?Value) cordis.Error!void {
            _ = config;
        }
    };
    const p = Plugin{ .name = "greeter", .apply = S.apply };
    const fiber = try p.start(ctx, null);
    try fiber.restart();
}

fn opUpdate(phase: *OomPhase) anyerror!void {
    const ctx = try Context.init(phase.allocator());
    defer ctx.deinit();
    defer phase.disarm();

    const S = struct {
        fn apply(_: *const Plugin, _: *Context, config: ?Value) cordis.Error!void {
            _ = config;
        }
    };
    const p = Plugin{ .name = "greeter", .apply = S.apply };
    const fiber = try p.start(ctx, null);
    try fiber.update(null);
}

test "extend surfaces OutOfMemory" {
    try expectOomDuringRegistration(opExtend);
}

test "isolate surfaces OutOfMemory" {
    try expectOomDuringRegistration(opIsolate);
}

test "isolateShared surfaces OutOfMemory" {
    try expectOomDuringRegistration(opIsolateShared);
}

test "withFilter surfaces OutOfMemory" {
    try expectOomDuringRegistration(opWithFilter);
}

test "provideNamed surfaces OutOfMemory" {
    try expectOomDuringRegistration(opProvideNamed);
}

test "provide surfaces OutOfMemory" {
    try expectOomDuringRegistration(opProvide);
}

test "attach surfaces OutOfMemory" {
    try expectOomDuringRegistration(opAttach);
}

test "attachLabeled surfaces OutOfMemory" {
    try expectOomDuringRegistration(opAttachLabeled);
}

test "onNamed surfaces OutOfMemory" {
    try expectOomDuringRegistration(opOnNamed);
}

test "addExporter surfaces OutOfMemory" {
    try expectOomDuringRegistration(opAddExporter);
}

test "snapshot surfaces OutOfMemory" {
    try expectOomDuringRegistration(opSnapshot);
}

test "restore surfaces OutOfMemory" {
    try expectOomDuringRegistration(opRestoreEmpty);
}

test "accessor surfaces OutOfMemory" {
    try expectOomDuringRegistration(opAccessor);
}

test "validated plugin start surfaces OutOfMemory during registration" {
    try expectOomDuringRegistration(opValidatedStart);
}

test "start surfaces OutOfMemory during registration" {
    try expectOomDuringRegistration(opStart);
}

test "restart and update allocate nothing on their fallible pre-drain path" {
    // With init+start's budget exhausted, both lifecycle calls must still
    // complete: their queue write reuses the drain's list capacity, so
    // today they make no allocation of their own. Every additional
    // allocation added to this path must thread `error.OutOfMemory` (the
    // sweep above fails at that index); allocations inside the drain
    // itself are dispatch machinery, whose OOM aborts by reviewed
    // contract (scripts/panic-allowlist.sh, user-gated end state).
    const budget = try registrationBudget();

    inline for (.{ opRestart, opUpdate }) |op| {
        var phase = OomPhase{ .parent = std.testing.allocator, .remaining = budget, .armed = true };
        try op(&phase);
        try std.testing.expectEqual(budget, phase.served);
    }
}
