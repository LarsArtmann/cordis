TL;DR: during a real triage session (cordis, 2026-10-09, buildflow `1b99ae2`) six distinct tool problems surfaced and **zero** became GitHub issues until Lars explicitly ordered it — and one of the six was already fixed upstream while consumers still document it as open. Ask: amend `buildflow/SKILL.md`'s "The standard loop" with a mandatory close-the-loop step: every encountered problem ends the session either fixed, filed (with source-level verification), or documented as a deliberate non-fix — never merely observed.

## Problem

The skill's standard loop ends at fix → drill in → recover. Nothing obliges the agent to externalize tool defects, so they die in session logs. Session evidence, one run:

1. `flake-meta-checker` false positive (`description = <identifier>;` reported missing, error severity) — real bug, unfixed.
2. Cargo tools run at repo root (subdir crates always fail) — real gap, documented in cordis AGENTS.md since 2026-09-08 as "fix belongs in buildflow" and never filed.
3. go-structure-linter agents-md budget stale at 377 vs BuildFlow doctor's 220 (ratcheted 2026-10-06) — real drift, never filed.
4. go-structure-linter + result cache: stale findings served after AGENTS.md edits (trigger declares `**/*.go`, tool reads markdown) — real bug, needed `BUILDFLOW_NO_RESULT_CACHE=1` to work around.
5. "nix steps build cross-system checks → platform mismatch" — documented as an open buildflow limitation in cordis AGENTS.md since 2026-09-08, but **already fixed upstream** (`ParseFlakeShowJSONForSystem`, feedback 2026-09-06). A mandatory re-verify-and-file step would have updated that stale doc months earlier.
6. One suspected bug was the session's own jq error (wrong JSON key) — the verify-before-filing gate exists precisely for this and belongs in the mandated path.

## Proposed change

Append to "The standard loop" and mirror in "Failure triage":

> 8. **Close the loop — file or fix, never absorb.** Every problem encountered in a run (step failure, false positive, stale doc, misleading message) MUST end the session in exactly one state: (a) fixed in the owning repo, (b) filed as a GitHub issue there — diagnosis verified at source level per `verify-before-filing`, duplicates searched open+closed — or (c) recorded as a deliberate non-fix with rationale in the consumer repo. "Mentioned in the run summary" is not a state.

Consumers of this skill run dozens of tools over dozens of repos; unfiled defects are silent fleet-wide regressions. The step costs minutes and is the difference between a bug report and folklore.

💘 Generated with Crush

Assisted-By: Crush:glm-5.3-flash
