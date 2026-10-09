{
  description = "Cordis: a meta-framework of spatiotemporal composability (TypeScript original + Go, Rust and Zig ports)";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs =
    { nixpkgs, ... }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "aarch64-darwin"
      ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
      # The live working tree (not the committed git tree), so checks see
      # uncommitted port changes too. Vendored/build directories are
      # excluded to keep the store copy small and deterministic.
      source = builtins.path {
        path = ./.;
        name = "cordis-source";
        filter =
          path: _type:
          !builtins.elem (baseNameOf path) [
            ".git"
            "node_modules"
            "target"
            ".zig-cache"
            "zig-out"
            "dist"
            ".turbo"
          ];
      };
    in
    {
      checks = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          go =
            pkgs.runCommand "cordis-go-tests"
              {
                nativeBuildInputs = [
                  pkgs.go_1_27
                  pkgs.gcc
                ];
              }
              ''
                export GOCACHE="$TMPDIR/go-build"
                export HOME="$TMPDIR"
                cp -r ${source}/go cordis-go
                cp -r ${source}/golden golden
                chmod -R u+w cordis-go
                cd cordis-go
                go vet ./...
                go test -race -count=3 ./...
                touch $out
              '';

          rust =
            pkgs.runCommand "cordis-rust-tests"
              {
                nativeBuildInputs = with pkgs; [
                  rustc
                  cargo
                  clippy
                  gcc
                ];
              }
              ''
                export CARGO_HOME="$TMPDIR/cargo"
                export CARGO_TARGET_DIR="$TMPDIR/target"
                cp -r ${source}/rust cordis-rust
                cp -r ${source}/golden golden
                chmod -R u+w cordis-rust
                cd cordis-rust
                cargo clippy --offline --all-targets
                cargo clippy --offline --all-targets --features thread-safe
                cargo test --offline
                touch $out
              '';

          zig =
            pkgs.runCommand "cordis-zig-tests"
              {
                nativeBuildInputs = [ pkgs.zig ];
              }
              ''
                cp -r ${source} cordis
                chmod -R u+w cordis
                cd cordis/zig
                zig fmt --check build.zig src tests
                zig build test --summary all --cache-dir "$TMPDIR/zig-cache" --global-cache-dir "$TMPDIR/zig-global-cache"
                # Doc emission gate: fails on code the compiler rejects; the
                # library compile walks every public decl.
                zig build docs --summary all --cache-dir "$TMPDIR/zig-cache" --global-cache-dir "$TMPDIR/zig-global-cache"
                touch $out
              '';

          markdown =
            pkgs.runCommand "cordis-markdown-lint"
              {
                nativeBuildInputs = [ pkgs.markdownlint-cli ];
              }
              ''
                cp -r ${source} cordis
                cd cordis
                markdownlint --config .markdownlint.jsonc --ignore-path .markdownlintignore .
                touch $out
              '';

          panic-allowlist =
            pkgs.runCommand "cordis-panic-allowlist"
              {
                nativeBuildInputs = with pkgs; [
                  bash
                  coreutils
                  diffutils
                  gawk
                  gnugrep
                ];
              }
              ''
                cp -r ${source} cordis
                cd cordis
                bash scripts/panic-allowlist.sh
                touch $out
              '';
        }
      );

      # `nix fmt` invokes the formatter with a directory argument, which the
      # bare nixfmt binary cannot parse; expand directories to files first.
      formatter = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        pkgs.writeShellApplication {
          name = "nixfmt";
          meta = {
            description = "Format Nix files (expands directories before invoking nixfmt)";
            license = pkgs.lib.licenses.mit;
            homepage = "https://github.com/LarsArtmann/cordis";
          };
          runtimeInputs = [ pkgs.nixfmt ];
          text = ''
            for target in "$@"; do
              if [ -d "$target" ]; then
                find "$target" -type f -name '*.nix' -not -path '*/node_modules/*' -exec nixfmt {} +
              else
                nixfmt "$target"
              fi
            done
          '';
        }
      );

      devShells = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          default = pkgs.mkShell {
            packages = with pkgs; [
              go_1_27
              gopls
              golangci-lint
              gcc
              rustc
              cargo
              clippy
              zig
              nodejs_24
              yarn-berry
            ];
            # Some machines ship broken cache locations via the environment;
            # resolve a writable one at shell startup (getEnv at eval time
            # returns "" under pure evaluation).
            shellHook = ''
              export GOCACHE="$HOME/.cache/cordis/go-build"
            '';
          };
        }
      );

      apps = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          mkTest =
            name: desc: script:
            let
              app = pkgs.writeShellApplication {
                inherit name;
                runtimeInputs = with pkgs; [
                  go_1_27
                  rustc
                  cargo
                  zig
                  markdownlint-cli
                  bash
                  diffutils
                  gawk
                  gnugrep
                ];
                text = script;
              };
            in
            {
              type = "app";
              program = "${app}/bin/${name}";
              meta = {
                # desc, not `description = description;`: nixfmt would
                # normalize that to `inherit description;`, which
                # buildflow's flake-meta-checker cannot parse.
                description = desc;
                license = nixpkgs.lib.licenses.mit;
                homepage = "https://github.com/LarsArtmann/cordis";
              };
            };
        in
        {
          test-go = mkTest "test-go" "Run the Go port test suite (go vet plus race-enabled tests)" ''
            export GOCACHE="''${GOCACHE_OVERRIDE:-$(mktemp -d)/go-build}"
            cd go && go vet ./... && go test -race -count=3 ./...
          '';
          test-rust =
            mkTest "test-rust" "Run the Rust port test suite (clippy on both feature variants plus tests)"
              ''
                export CARGO_HOME="''${CARGO_HOME_OVERRIDE:-$HOME/.cache/cordis/cargo}"
                mkdir -p "$CARGO_HOME"
                cd rust && cargo clippy --all-targets && cargo clippy --all-targets --features thread-safe && cargo test
              '';
          test-zig = mkTest "test-zig" "Run the Zig port test suite (leak-checked)" ''
            cd zig && zig build test --summary all
          '';
          test-markdown = mkTest "test-markdown" "Run markdownlint over the repository" ''
            markdownlint --config .markdownlint.jsonc --ignore-path .markdownlintignore .
          '';
          test-panic-allowlist =
            mkTest "test-panic-allowlist" "Verify the panic allowlist against all three ports"
              ''
                bash scripts/panic-allowlist.sh
              '';
          test = mkTest "test" "Run every port test suite: Go, Rust, Zig, markdown, panic allowlist" ''
            export GOCACHE="''${GOCACHE_OVERRIDE:-$(mktemp -d)/go-build}"
            export CARGO_HOME="''${CARGO_HOME_OVERRIDE:-$HOME/.cache/cordis/cargo}"
            mkdir -p "$CARGO_HOME"
            set -e
            echo "== Go =="
            (cd go && go vet ./... && go test -race -count=3 ./...)
            echo "== Rust =="
            (cd rust && cargo clippy --all-targets && cargo clippy --all-targets --features thread-safe && cargo test)
            echo "== Zig =="
            (cd zig && zig build test --summary all)
            echo "== Markdown =="
            markdownlint --config .markdownlint.jsonc --ignore-path .markdownlintignore .
            echo "== Panic allowlist =="
            bash scripts/panic-allowlist.sh
          '';
        }
      );
    };
}
