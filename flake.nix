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
    in
    {
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
