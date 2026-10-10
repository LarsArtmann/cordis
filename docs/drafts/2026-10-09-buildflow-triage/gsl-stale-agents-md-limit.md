TL;DR: `internal/rules/agent_config_rule.go:15` still has `maxAgentsMdLines = 377` — BuildFlow's doctor ratcheted the same budget to **220 on 2026-10-06** (`discovery/doctor/checks/agents_md.go`, "Lowered from 377 to 220"). Same file, two different budgets in one pipeline run: preflight warns at 220, the structure linter errors at 377. Ask: sync the constant (220) or make both read one shared source of truth.

## Problem

One buildflow run on cordis, 2026-10-09 (buildflow `1b99ae2`, gsl at cdc3c4f-era tree):

```
WARN  preflight warn [docs/agents-md-size] AGENTS.md has 399 lines (max: 220, excess: 179)
...
ERROR AGENTS.md:399 AGENTS.md has 399 lines (maximum: 377, excess: 22)
```

Two thresholds for the identical check confuse consumers: after trimming to 376 lines the error clears while the warning persists, and nothing tells you which number is current. BuildFlow's constant carries the history in-source:

```go
// maxAgentsMdLines is the ratchet for AGENTS.md's line budget. Lowered from
// 377 to 220 on 2026-10-06 ... (discovery/doctor/checks/agents_md.go:15-19)
const maxAgentsMdLines = 220
```

vs `agent_config_rule.go:15`: `maxAgentsMdLines = 377`.

## Suggested fix

Set gsl's default to 220 to match the doctor ratchet (and note the date), unless the intent is that the SDK tool deliberately keeps a looser budget — if so, a comment saying so would prevent the next "which limit is real" investigation. Longer term, both tools drifting on the same policy number is the pattern that wants one canonical home.

💘 Generated with Crush

Assisted-By: Crush:glm-5.3-flash
