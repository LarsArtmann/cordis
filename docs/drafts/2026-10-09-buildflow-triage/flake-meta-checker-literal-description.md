TL;DR: `description = desc;` inside a `meta` block is reported as "missing the description attribute" (severity error, trips the findings gate) because the checker only recognizes string literals. Ask: count any `description =` binding (and `inherit description;`) as present — downgrade "present but unverifiable" instead of erroring.

## Problem

Ran buildflow on a flake whose apps set `meta = { description = desc; ... }` (parameterized so nixfmt doesn't normalize it to `inherit description;`). The checker reports:

```
flake.nix:217: meta block is missing the description attribute (error)
```

...which fails the default findings gate (`--fail-on=error`), so the run exits non-zero on a false positive.

## Root cause

`modules/flake-meta-checker` is regex-scanned:

- `check_meta_attributes.go:178` — `descValueRe = regexp.MustCompile(`^\s_description\s_=\s*"([^"]*)"`)` requires a string **literal**; `description = desc;` (identifier) never matches.
- `meta_block.go:65` — `metaAttrRe =`^\s*(\w+)\s*=`` can't see `inherit description;` either (no `=`).

Reproduced 2026-10-09 on cordis (`flake.nix:217`), buildflow `1b99ae2`, file state at repo cdc3c4f. After renaming the binding to a literal the finding cleared; nixfmt converting `description = description;` back to `inherit description;` re-broke it — both shapes are idiomatic Nix.

## Suggested fix

1. `description =` with a non-literal value → attribute present, unverifiable value: no error (optionally info "could not verify description is a string").
2. `inherit <attr>;` lines inside a `meta { }` block → count each inherited attr as present.

Alternative: evaluate with `nix eval` when available, keep the regex as the fallback — but the cheap regex fix closes the false-gate case.

💘 Generated with Crush

Assisted-By: Crush:glm-5.3-flash
