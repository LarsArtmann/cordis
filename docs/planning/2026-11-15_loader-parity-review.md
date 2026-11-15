# Loader parity review, rc.13 (T20)

Review of the eight upstream loader fixes absorbed by the rc.13 sync
(#329/#271/#267/#239/#210/#246/#241/#179, plus #212) against `go/loader`,
reconciled with the concurrent session's `entry_options`/`entry_lifecycle`
work (`0c42ee9`). Companion to the program plan's T20.

## Mapping

| PR | Upstream change | Go status |
| --- | --- | --- |
| #329 | isolation boundaries resolved through per-(name,label) symbol isolate maps instead of GlobalRealm objects | **Already the Go design.** Go realms are uint64 keys (`sharedKey(name, label)` synthetic key, root fallback, parent-chain walks) — the same shape #329 adopted. No action. |
| #271 | cross-tree entry moves transfer ownership (source commits a removal, target a creation; store adopts entry + members) | **No surface yet.** Go `Tree.Move` resolves groups within one tree; a Go Tree is single-tree and there is no include/multi-tree composition. Revisit if/when a multi-tree loader lands; until then Remove+Create is the embedder-side equivalent. |
| #267 | empty string is a valid isolate label | **Satisfied by construction.** `buildContext`'s switch sends `""` (any non-nil non-bool) down the labeled shared-realm path; Go has no JS truthiness. Micro-edge documented below. |
| #239 | validate entry options, locate entry field errors | **Partial by design.** `validateIsolateLabels` (0c42ee9) + Update option-diffing cover the Go-relevant subset; stdlib-JSON config has no richer shape to validate, and field-error *locations* are yaml machinery (see #210). |
| #210 | trace config validation issues to source locations | **N/A.** The Go loader decodes stdlib JSON (`cordis.json`) with no position grammar; errors name the entry and field instead of a file position. Documented divergence. |
| #246 | locate evaluation errors of entry options | **N/A**, same substrate as #210. |
| #241 | remove `cordis:` builtin specifier mechanism | **Go never had the generic mechanism.** It keeps a harmless config-name alias: entries may name the group plugin `"group"` or `"cordis:group"` (tree.go). Upstream's name is `@cordisjs/group`. Keep the alias (removing it breaks existing Go configs for zero gain); document. |
| #179 | evaluate `!!js` expressions in `disabled` | **N/A.** No js-expression machinery exists in Go config; `disabled` is a plain bool. Documented divergence. |
| #212 | accept a drive-letter path for plugin import | **N/A.** Go resolves plugin names through the Resolver registry (AOT); there are no dynamic file imports to path-normalize. |

## Notes

- Micro-edge on #267: upstream keys a shared realm for a `false` label
  (post-fix, only `undefined` means "not isolated"); Go's switch treats
  `bool` (both `true` and `false`) as fresh-unlabeled. An entry writing
  `isolate: {svc: false}` gets a shared realm upstream, a fresh one in
  Go. Nobody sane writes this; recorded rather than fixed.
- Watch-golden (`go/loader/watch_golden_test.go`) ran green through the
  T17 sweep; the reviewed commits change no Go loader semantics.
- Rust/Zig: no loader ports exist (by scope decision); nothing to map.

Decided 2026-11-15 during the rc.13 sync execution session
(`docs/status/2026-11-15_08-18_rc13-sync-execution-session.md`).
