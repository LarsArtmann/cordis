#!/usr/bin/env bash
# Panic allowlist gate for the Go, Rust and Zig ports: production code
# must stay panic-free except for the reviewed entries below. Each entry
# pins the exact number of panic-pattern lines in one production file
# together with the reason that site has no error channel. A new panic
# fails the gate until the allowlist grows with a rationale; removing one
# fails until the entry shrinks (stale entries are drift). Reviewed
# 2026-09-10 alongside the panic-free typed-errors sweep.
#
# Rationales:
# - go/typed.go: Must* lookup sugar (explicit opt-in) and the typed-event
#   dispatch guards; listener callbacks (func(E)) have no error channel
#   in any port.
# - go/loader/resolver.go: MustRegister sugar, same contract as Must*.
# - rust/src/events.rs: typed-event dispatch guards; Fn(&E) listeners
#   have no error channel.
# - zig/src/cordis.zig: dispatch-path allocations and channel-less void
#   queries (emit/bail/serial/waterfall, effects(), Registry.delete,
#   isolateKey, Fiber.dispose) abort with the distinct greppable message
#   "cordis: out of memory in dispatch"; the error log drops its line
#   instead of aborting.
#
# Patterns: Go `panic(`, Rust `panic!`/`expect`/`unwrap`/`unreachable!`/
# `todo!`/`unimplemented!` (doc-comment doctests excluded), Zig `@panic`.
# Test files are exempt everywhere (`*_test.go`, `rust/tests`,
# `zig/tests`, Rust doc comments).
set -euo pipefail
cd "$(dirname "$0")/.."

expected() {
  cat <<'EOF'
go/loader/resolver.go 1
go/typed.go 6
rust/src/events.rs 2
zig/src/cordis.zig 15
EOF
}

actual() {
  {
    grep -rn 'panic(' go --include='*.go' --exclude='*_test.go' || true
    grep -rnE 'panic!\(|\.expect\(|\.unwrap\(|unreachable!\(|todo!\(|unimplemented!\(' rust/src || true
    grep -rn '@panic' zig/src || true
  } | grep -Ev '^[^:]+:[0-9]+:[[:space:]]*///' | cut -d: -f1 | sort | uniq -c | awk '{print $2, $1}'
}

exp="$(expected | sort)"
act="$(actual)"

if [ "$exp" = "$act" ]; then
  echo "panic allowlist: OK — reviewed production panic sites:"
  echo "$act" | sed 's/^/  /'
  exit 0
fi

echo "panic allowlist gate FAILED: production panic sites diverge from the reviewed allowlist." >&2
echo "--- expected vs actual ---" >&2
diff -u <(echo "$exp") <(echo "$act") | sed -n '3,$p' >&2 || true
cat >&2 <<'EOS'
Every production panic must be a reviewed, channel-less site. Update
scripts/panic-allowlist.sh: add an entry with a rationale for a new
panic, or shrink the entry (and remove the code) when one disappeared.
EOS
exit 1
