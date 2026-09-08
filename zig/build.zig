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
    _ = golden_files.addCopyFile(b.path("../golden/scenario-dispatch.txt"), "scenario-dispatch.txt");
    _ = golden_files.addCopyFile(b.path("../golden/expected-dispatch.txt"), "expected-dispatch.txt");
    _ = golden_files.addCopyFile(b.path("../golden/scenario-cascade.txt"), "scenario-cascade.txt");
    _ = golden_files.addCopyFile(b.path("../golden/expected-cascade.txt"), "expected-cascade.txt");
    const golden_data = golden_files.add("golden_data.zig",
        \\pub const scenario = @embedFile("scenario.txt");
        \\pub const expected = @embedFile("expected.txt");
        \\pub const scenario_events = @embedFile("scenario-events.txt");
        \\pub const expected_events = @embedFile("expected-events.txt");
        \\pub const scenario_dispatch = @embedFile("scenario-dispatch.txt");
        \\pub const expected_dispatch = @embedFile("expected-dispatch.txt");
        \\pub const scenario_cascade = @embedFile("scenario-cascade.txt");
        \\pub const expected_cascade = @embedFile("expected-cascade.txt");
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

    // Doc emission: `zig build docs` writes the generated module docs to
    // zig-out/docs. The build runner has no -femit-docs flag in 0.16, so
    // the pass rides on a library compile that analyzes every public decl.
    const docs_step = b.step("docs", "Emit the cordis module documentation");
    const lib = b.addLibrary(.{
        .linkage = .static,
        .name = "cordis",
        .root_module = b.createModule(.{
            .root_source_file = b.path("src/cordis.zig"),
            .target = target,
            .optimize = optimize,
        }),
    });
    const docs_install = b.addInstallDirectory(.{
        .source_dir = lib.getEmittedDocs(),
        .install_dir = .{ .custom = "docs" },
        .install_subdir = "",
    });
    docs_step.dependOn(&docs_install.step);
}
