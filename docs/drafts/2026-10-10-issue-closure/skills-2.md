Implemented — closing.

`buildflow/SKILL.md` "The standard loop" now ends with:

> 7. **Close the loop — file or fix, never absorb.** Every problem encountered in a run (step failure, false positive, stale doc, misleading message) MUST end the session in exactly one state: (a) fixed in the owning repo, (b) filed as a GitHub issue there — diagnosis verified at source level per **verify-before-filing**, duplicates searched open+closed — or (c) recorded as a deliberate non-fix with rationale in the consumer repo. "Mentioned in the run summary" is not a state. This also catches stale workarounds: re-verify documented limitations against the current binary before re-documenting them.

Mirrored in "Failure triage" (whatever the symptom, close the loop; same re-verify-before-workaround gate). The re-verify clause is this issue's item 5 turned policy: the "platform mismatch" limitation sat documented as open in cordis for a month after it was fixed upstream.

💘 Generated with Crush

Assisted-By: Crush:glm-5.3-flash
