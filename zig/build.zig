const std = @import("std");

pub fn build(b: *std.Build) void {
    const target = b.standardTargetOptions(.{});
    const optimize = b.standardOptimizeOption(.{});

    const mod = b.addModule("cordis", .{
        .root_source_file = b.path("src/cordis.zig"),
        .target = target,
        .optimize = optimize,
    });

    const tests = b.addTest(.{
        .root_module = b.createModule(.{
            .root_source_file = b.path("tests/parity.zig"),
            .target = target,
            .optimize = optimize,
            .imports = &.{.{ .name = "cordis", .module = mod }},
        }),
    });

    const run_tests = b.addRunArtifact(tests);
    const test_step = b.step("test", "Run the cordis Zig test suite");
    test_step.dependOn(&run_tests.step);
}
