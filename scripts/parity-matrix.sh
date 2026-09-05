#!/usr/bin/env bash
# Emit an approximate parity matrix: which behavior areas have tests in
# each port. Name-based matching is approximate by nature; treat this as a
# navigation aid, not a guarantee.
# Usage: nix run nixpkgs#bash -- scripts/parity-matrix.sh
set -euo pipefail
cd "$(dirname "$0")/.."

echo "| Area | Go | Rust | Zig |"
echo "| ---- | -- | ---- | --- |"

list_go() { (cd go && GOCACHE=/tmp/gocache go test -list '.*' . ./group ./timer ./loader ./hmr 2>/dev/null | grep -E '^(Test|Benchmark)'); }
list_rust() { (cd rust && cargo test -- --list 2>/dev/null | grep -E ': test$' | cut -d: -f1); }
list_zig() { grep -hoE 'test "[^"]+"' zig/tests/*.zig zig/src/*.zig 2>/dev/null | sed 's/.*test "//;s/"//' || true; }

GO="$(list_go | sort -u)"
RUST="$(list_rust | sort -u)"
ZIG="$(list_zig | sort -u)"

row_for() {
  local area="$1" go_pat="$2" rust_pat="$3" zig_pat="$4"
  local row="| $area |"
  grep -qiE "$go_pat" <<<"$GO" && row="$row x |" || row="$row - |"
  grep -qiE "$rust_pat" <<<"$RUST" && row="$row x |" || row="$row - |"
  grep -qiE "$zig_pat" <<<"$ZIG" && row="$row x |" || row="$row - |"
  echo "$row"
}

row_for lifecycle 'FiberLifecycle|Start.*Dispose|plugin_lifecycle' 'lifecycle|plugin_lifecycle' 'lifecycle'
row_for effect 'Effect' 'effect' 'effect'
row_for event 'Event' 'event' 'event'
row_for service 'Provide|Service' 'provide|service' 'provide|service'
row_for isolate 'Isolate' 'isolate|shared' 'isolate'
row_for inject 'Inject' 'inject|dependent' 'inject'
row_for registry 'Registry' 'registry' 'registry'
row_for waterfall 'Waterfall' 'waterfall' 'waterfall'
row_for parallel 'Parallel' 'parallel' 'parallel'
row_for serial 'Serial' 'serial' 'serial'
row_for dispose 'Dispose' 'dispose' 'dispose'
row_for golden 'Golden' 'golden' 'golden'
row_for snapshot 'Snapshot' 'snapshot' 'snapshot'
row_for status 'Status' 'status|Status' 'status'
row_for accessor 'Accessor' 'accessor' 'accessor'
row_for mixin 'Mixin' 'mixin' 'mixin'
row_for loader/watch 'ConfigFile|Watch|Reload|Entry' 'watch|reload|entry' 'watch|reload'
row_for hmr 'Swap|Hmr' 'swap|hmr' 'swap'
row_for timer 'Interval|Debounce|Throttle' 'timer' 'timer'
row_for batch 'Batch' 'batch' 'batch'
