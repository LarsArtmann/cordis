Fixed in this repo — closing.

**Fix:** `pkg/provider/provider.go` — the Trigger now declares the detector's real read set instead of `toolsdk.OnGoModule()`'s `**/*.go`-only Files:

```go
Files: []string{
    "**/*.go",
    "AGENTS.md", "README.md", "LICENSE", ".gitignore", ".env",
    ".go-structure-linter.yaml", ".go-structure-linter.yml",
    ".structure-linter.yaml", ".structure-linter.yml",
},
```

Language stays `go` and Requires stays `**/go.mod`/`**/go.work`, so activation is unchanged in Go repos — but BuildFlow's result cache now keys on the content hash of those files, so AGENTS.md edits invalidate the cache. Directory-structure-only changes stay unkeyed (stated in a comment; tree changes almost always touch a declared file too).

**Tests:** `TestTriggerFilesDeclareReadSet` pins the contract; `GOEXPERIMENT=jsonv2 go test -race ./internal/rules/ ./pkg/provider/` green.

**Live:** verified in the cordis pipeline through BuildFlow's local replace (`go-structure-linter:detect` green with the widened trigger). Pinned/flake builds pick this up at the next toolsdk version bump + BuildFlow repin; BuildFlow's local `replace` picks it up immediately.

💘 Generated with Crush

Assisted-By: Crush:glm-5.3-flash
