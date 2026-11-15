//! Example smoke app for the Zig port: a typed `Greeter` service, a typed
//! `Greeted` event, and the synchronous drain contract (the listener has
//! already run when `emitTyped` returns). Build with `zig build` and run
//! `zig-out/bin/greeter`.

const std = @import("std");
const cordis = @import("cordis");

const Context = cordis.Context;

/// Provided under its type identity; consumers look it up with
/// `getTyped(Greeter)`.
const Greeter = struct {
    name: []const u8,
};

/// The typed event emitted after a greeting renders.
const Greeted = struct {
    line: []const u8,
};

const Printer = struct {
    fn on(_: *Printer, event: Greeted) void {
        std.debug.print("{s}\n", .{event.line});
    }
};

pub fn main() !void {
    const ctx = try Context.init(std.heap.page_allocator);
    defer ctx.deinit();

    var greeter = Greeter{ .name = "cordis" };
    _ = try ctx.provide(&greeter);

    var printer = Printer{};
    _ = try ctx.onTyped(Greeted, &printer, Printer.on);

    const g = ctx.getTyped(Greeter) orelse return error.MissingGreeter;
    var buf: [64]u8 = undefined;
    const line = try std.fmt.bufPrint(&buf, "Hello, world, from {s}!", .{g.name});
    ctx.emitTyped(Greeted, &Greeted{ .line = line });
}
