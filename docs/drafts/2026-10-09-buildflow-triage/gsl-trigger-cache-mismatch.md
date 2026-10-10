TL;DR: the provider declares `Trigger: toolsdk.OnGoModule()` → `Files: ["**/*.go"]`, but the linter actually reads AGENTS.md, README.md, LICENSE and directory structure. BuildFlow's result cache keys detector output on the declared input files, so editing AGENTS.md does not change the cache key — stale findings get served until some .go file changes or the entry expires. Ask: declare the real read scope in the trigger.

## Problem

Reproduced 2026-10-09 on cordis (buildflow `1b99ae2`):

1. AGENTS.md at 381 lines → structure-linter reports "has 381 lines" (error).
2. Edited AGENTS.md down to 376 lines (verified `wc -l`).
3. `buildflow -s go-structure-linter` → **still reports 381 lines**, summary shows `cache: 1 hits, 0 misses (100% hit rate)`.
4. `BUILDFLOW_NO_RESULT_CACHE=1 buildflow -s go-structure-linter` → correct fresh result (376/377).

## Root cause

- `pkg/provider/provider.go:90` — `Trigger: toolsdk.OnGoModule()`.
- go-finding `toolsdk/triggers.go` — `OnGoModule()` sets `Files: []string{"**/*.go"}`.
- BuildFlow keys detector cache entries on `computeInputContentHash(rootDir, inputs)` — SHA-256 over the files matched by the trigger's `Files` patterns (execution/result_cache_1.go). AGENTS.md is not a `*.go` file → its content never enters the key → the cached FindingReport survives edits.

Same class as the gotcha-BuildFlow-files#155 caveat (github-actions-pinning depending on remote state the cache doesn't key on): detection depends on state outside the declared input set.

The tool also reports on directory presence/absence (`assets/`, `internal/`, `examples/` suggestions) and non-Go files (README install section, .gitignore hygiene) — none of that state is representable in the current key.

## Suggested fix

Widen `Trigger.Files` to the actual read set, e.g. `["**/*.go", "AGENTS.md", "README.md", ".gitignore"]` (and consider `**/*` + exclude patterns if the directory rules should re-run on any tree change — with the cost tradeoff stated). Until then, consumers must document `BUILDFLOW_NO_RESULT_CACHE=1` or `--no-result-cache-for go-structure-linter` as the workaround for stale AGENTS.md findings (cordis AGENTS.md now carries this).

💘 Generated with Crush

Assisted-By: Crush:glm-5.3-flash
