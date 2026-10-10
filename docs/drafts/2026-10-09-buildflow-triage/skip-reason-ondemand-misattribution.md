TL;DR: on-demand tools (markdown-lint, gitleaks, codespell) show up in the summary as `skipped by build mode 'full'` — wrong attribution: full mode doesn't blocklist them, `ShouldSkipOnDemand` does, and the accurate message ("is an on-demand tool: run with `buildflow -s <tool>`") already exists but never reaches the summary. Ask: use `SkipReasonForStep`'s message for build-mode skips.

## Problem

Run summary on cordis, buildflow `1b99ae2`, 2026-10-09:

```
⊘ markdown-lint  (skipped by build mode 'full')
⊘ gitleaks       (skipped by build mode 'full')
⊘ codespell      (skipped by build mode 'full')
```

Reading that, you conclude full mode blocklists markdown-lint — it doesn't. The actual cause is the on-demand group, and this cost me real time in cordis: I treated "full mode skips markdown-lint" as a pipeline bug and went hunting for a mode-profile regression, when the true statement is "markdown-lint is on-demand by user preference (2026-09-13) in every mode".

## Root cause

Two code paths disagree:

- `domain/config/build_mode.go:88-95` — `ShouldSkipOnDemand` (gitleaks, codespell, markdown-lint) and `SkipReasonForStep` (L129-132) already produces the precise reason: `"<step> is an on-demand tool: run with`buildflow -s <step>`"`.
- `execution/filtered_tools_1.go:94` — `detectBuildModeSkips` hardcodes `reason := "skipped by build mode '" + string(mode) + "'"` for everything it reports, discarding the specific message.

## Suggested fix

In `detectBuildModeSkips`, prefer `mode.SkipReasonForStep(name)` when non-empty and keep the generic string only as fallback. One-line-ish change; makes the summary self-explanatory and stops sending users after a nonexistent mode-profile bug.

💘 Generated with Crush

Assisted-By: Crush:glm-5.3-flash
