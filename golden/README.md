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

## Scenarios

| Scenario | Spec | Expected trace | Pins |
| --- | --- | --- | --- |
| #1 lifecycle | `scenario.txt` | `expected.txt` | fiber lifecycle, inject reactivity, LIFO rollback, isolation realms, root restart |
| #2 events | `scenario-events.txt` | `expected-events.txt` | dispatch order, realm filters, global listeners |
| #3 cascade | `scenario-cascade.txt` | `expected-cascade.txt` | nested plugin disposal order, registry delete, registry size |

Regenerate an expected file with
`GOLDEN_UPDATE=1 go test -run 'TestGolden.*' ./...` from `go/`, then
re-verify the Rust and Zig runners.

The trace must equal the scenario's expected file byte for byte in every language. A
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
