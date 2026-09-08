// Package cordis anchors the repository root as a Go module so that
// repo-level Go tooling (go generate/test/vet ./...) has a module to
// resolve. The real ports live in the nested go/ module (flagship) next to
// the upstream TypeScript sources in packages/; this module deliberately
// contains no buildable code beyond this stub.
package cordis
