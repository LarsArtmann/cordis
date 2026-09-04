# Golden scenario

One spec, three runners. `scenario.txt` describes a scripted session against
the cordis core (fiber lifecycle, inject reactivity, LIFO rollback,
isolation realms, root restart cascades). Each port ships a runner that
executes the script and emits a canonical trace:

| Runner | Location               |
| ------ | ---------------------- |
| Go     | `go/golden_test.go`    |
| Rust   | `rust/tests/golden.rs` |
| Zig    | `zig/tests/golden.zig` |

The trace must equal `expected.txt` byte for byte in every language. A
divergence means one port's semantics drifted; fix the port, never the
golden file, unless the scenario itself changes (in which case regenerate
`expected.txt` with every runner and commit all three results).

Trace lines are fully deterministic: no timestamps, no uids, states are
spelled `PENDING`, `ACTIVE`, `DISPOSED`, and every line is produced by
framework-observable behavior only (plugin bodies, cleanups, service
lifecycle ops, state assertions).

To regenerate after intentionally changing the scenario:

```sh
GOLDEN_UPDATE=1 (cd go && go test -run TestGoldenScenario -count=1 ./...)
(cd rust && cargo test --test golden)
(cd zig && zig build test)
```

Then verify the Rust and Zig traces still match the regenerated file.
