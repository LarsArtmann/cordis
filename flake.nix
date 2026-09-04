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
          path: type:
          !builtins.elem (baseNameOf path) [
            ".git"
            "node_modules"
            "target"
            ".zig-cache"
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
          go = pkgs.runCommand "cordis-go-tests" {
            nativeBuildInputs = [
              pkgs.go
              pkgs.gcc
            ];
          } ''
            export GOCACHE="$TMPDIR/go-build"
            export HOME="$TMPDIR"
            cp -r ${source}/go cordis-go
            cp -r ${source}/golden golden
            chmod -R u+w cordis-go
            cd cordis-go
            go vet ./...
            go test -race -count=1 ./...
            touch $out
          '';

          rust = pkgs.runCommand "cordis-rust-tests" {
            nativeBuildInputs = with pkgs; [
              rustc
              cargo
              clippy
              gcc
            ];
          } ''
            export CARGO_HOME="$TMPDIR/cargo"
            export CARGO_TARGET_DIR="$TMPDIR/target"
            cp -r ${source}/rust cordis-rust
            cp -r ${source}/golden golden
            chmod -R u+w cordis-rust
            cd cordis-rust
            cargo clippy --offline --all-targets -- --deny warnings
            cargo test --offline
            touch $out
          '';

          zig = pkgs.runCommand "cordis-zig-tests" {
            nativeBuildInputs = [ pkgs.zig ];
          } ''
            cp -r ${source} cordis
            chmod -R u+w cordis
            cd cordis/zig
            zig build test --summary all --cache-dir "$TMPDIR/zig-cache" --global-cache-dir "$TMPDIR/zig-global-cache"
            touch $out
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
              go
              gopls
              golangci-lint
              rustc
              cargo
              clippy
              zig
              nodejs_24
              yarn-berry
            ];
            # Some machines ship broken cache locations via the environment;
            # point Go at a writable one inside the shell.
            GOCACHE = "${builtins.getEnv "HOME"}/.cache/cordis/go-build";
          };
        }
      );

      apps = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          mkTest =
            name: script:
            let
              app = pkgs.writeShellApplication {
                inherit name;
                runtimeInputs = with pkgs; [
                  go
                  rustc
                  cargo
                  zig
                ];
                text = script;
              };
            in
            {
              type = "app";
              program = "${app}/bin/${name}";
            };
        in
        {
          test-go = mkTest "test-go" ''
            export GOCACHE="''${GOCACHE_OVERRIDE:-$(mktemp -d)/go-build}"
            cd go && go vet ./... && go test -race -count=1 ./...
          '';
          test-rust = mkTest "test-rust" ''
            export CARGO_HOME="''${CARGO_HOME_OVERRIDE:-$HOME/.cache/cordis/cargo}"
            mkdir -p "$CARGO_HOME"
            cd rust && cargo clippy --all-targets -- --deny warnings && cargo test
          '';
          test-zig = mkTest "test-zig" ''
            cd zig && zig build test --summary all
          '';
          test = mkTest "test" ''
            export GOCACHE="''${GOCACHE_OVERRIDE:-$(mktemp -d)/go-build}"
            export CARGO_HOME="''${CARGO_HOME_OVERRIDE:-$HOME/.cache/cordis/cargo}"
            mkdir -p "$CARGO_HOME"
            set -e
            echo "== Go =="
            (cd go && go vet ./... && go test -race -count=1 ./...)
            echo "== Rust =="
            (cd rust && cargo clippy --all-targets -- --deny warnings && cargo test)
            echo "== Zig =="
            (cd zig && zig build test --summary all)
          '';
        }
      );
    };
}
