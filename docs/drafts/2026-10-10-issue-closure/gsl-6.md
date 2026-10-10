Fixed — the constant is synced to the doctor's ratchet, and deliberate exceptions now go through the suppression mechanism instead of a divergent constant.

**Fix:** `internal/rules/agent_config_rule.go` — `maxAgentsMdLines` 377 → 220, with a comment pointing at BuildFlow's `discovery/doctor/checks/agents_md.go` (lowered 377 → 220 on 2026-10-06 when the gotcha register moved out of AGENTS.md). Behavioral pin: a 221-line AGENTS.md must report "maximum: 220" (fails on the old constant, passes on the new). Suite green.

**Consumer policy:** the two repos that deliberately keep ~376-line AGENTS.md files (this repo and cordis) now carry a `.go-structure-linter.yaml` suppressing `agent-config` with a required reason and an expiry that forces re-review — dogfooding the suppression feature instead of letting the budget drift per repo. This repo's own run is green again (`total=0` with the 376-line AGENTS.md in place).

💘 Generated with Crush

Assisted-By: Crush:glm-5.3-flash
