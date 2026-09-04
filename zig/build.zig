const std = @import("std");

pub fn build(b: *std.Build) void {
    const target = b.standardTargetOptions(.{});
    const optimize = b.standardOptimizeOption(.{});

    const mod = b.addModule("cordis", .{
        .root_source_file = b.path("src/cordis.zig"),
        .target = target,
        .optimize = optimize,
    });

    const test_step = b.step("test", "Run the cordis Zig test suite");

    for ([_][]const u8{ "tests/parity.zig", "tests/typed.zig" }) |path| {
        const tests = b.addTest(.{
            .root_module = b.createModule(.{
                .root_source_file = b.path(path),
                .target = target,
                .optimize = optimize,
                .imports = &.{.{ .name = "cordis", .module = mod }},
            }),
        });
        test_step.dependOn(&b.addRunArtifact(tests).step);
    }

    // The shared golden scenario and its expected trace are copied into the
    // build cache and embedded, so the runner needs no runtime file access.
    const golden_files = b.addWriteFiles();
    _ = golden_files.addCopyFile(b.path("../golden/scenario.txt"), "scenario.txt");
    _ = golden_files.addCopyFile(b.path("../golden/expected.txt"), "expected.txt");
    _ = golden_files.addCopyFile(b.path("../golden/scenario-events.txt"), "scenario-events.txt");
    _ = golden_files.addCopyFile(b.path("../golden/expected-events.txt"), "expected-events.txt");
    const golden_data = golden_files.add("golden_data.zig",
        \\pub const scenario = @embedFile("scenario.txt");
        \\pub const expected = @embedFile("expected.txt");
        \\pub const scenario_events = @embedFile("scenario-events.txt");
        \\pub const expected_events = @embedFile("expected-events.txt");
        \\
    );
    const golden_data_mod = b.createModule(.{ .root_source_file = golden_data });

    const golden_mod = b.createModule(.{
        .root_source_file = b.path("tests/golden.zig"),
        .target = target,
        .optimize = optimize,
        .imports = &.{
            .{ .name = "cordis", .module = mod },
            .{ .name = "golden_data", .module = golden_data_mod },
        },
    });
    const golden = b.addTest(.{ .root_module = golden_mod });
    test_step.dependOn(&b.addRunArtifact(golden).step);
}
