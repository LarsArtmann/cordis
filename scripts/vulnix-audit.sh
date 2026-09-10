#!/usr/bin/env bash
# Scoped vulnix CVE audit of this flake's own derivations (see
# docs/status/2026-09-10_09-08_vulnix-environment-audit.md for the full
# reviewed run and triage methodology).
#
# Modes:
#   scripts/vulnix-audit.sh direct   (default)
#       Scan only the packages the flake itself selects: the direct
#       build inputs of every check derivation, the default devShell
#       and the formatter. The apps' runtimeInputs are a subset of
#       those. This is the low-noise scope recommended after the
#       2026-09-08 buildflow run flagged 153 whole-store-closure
#       findings; expect single-digit package counts.
#   scripts/vulnix-audit.sh closure
#       Scan the full build closure of the same derivations
#       (thorough but noisy: bootstrap toolchains, nixpkgs packaging
#       internals, stdenv).
#
# Exit codes are vulnix's: 0 clean, 2 vulnerabilities found.
# Caveat (from the 2026-09-10 review): vulnix matches NVD entries by
# product name + version range only, so expect product-collision false
# positives (MediaWiki "Cargo" vs cargo, "ecies/go" vs the Go
# toolchain, ...). Verify every critical finding against the NVD entry
# before acting.
#
# Requires: nix with flakes, jq. vulnix is pulled from the registry's
# nixpkgs (the scanner version hardly matters; NVD data drives the
# result). First run downloads the NVD archive (~380 MB cache in
# ~/.cache/vulnix) and needs network access.
set -euo pipefail
cd "$(dirname "$0")/.."

mode="${1:-direct}"
case "$mode" in
  direct | closure) ;;
  *)
    echo "usage: $0 [direct|closure]" >&2
    exit 64
    ;;
esac

system="$(nix eval --impure --expr 'builtins.currentSystem' --raw)"
# ^out: the package has extra outputs (man); we only need bin/.
vulnix_bin="$(nix build 'nixpkgs#vulnix^out' --no-link --print-out-paths)/bin/vulnix"

mapfile -t drvs < <(
  for check in go rust zig markdown panic-allowlist; do
    nix eval --raw ".#checks.$system.$check.drvPath" && echo
  done
  nix eval --raw ".#devShells.$system.default.drvPath" && echo
  nix eval --raw ".#formatter.$system.drvPath" && echo
)

flags=()
if [ "$mode" = direct ]; then
  flags+=(--no-requisites)
  mapfile -t drvs < <(
    for drv in "${drvs[@]}"; do
      nix derivation show "$drv" |
        jq -r '.derivations[].inputs.drvs | keys[] | "/nix/store/" + .'
    done | sort -u
  )
fi

echo "== vulnix $mode scan: ${#drvs[@]} derivations =="
exec "$vulnix_bin" "${flags[@]}" "${drvs[@]}"
