# AI Assistant Rules (PART 0, 1)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Guess requirements, values, or behavior; inspect the governing spec or ask when it is genuinely silent.
- Implement before reading the task's relevant AI.md PART and project intent in IDEA.md.
- Modify AI.md; it is the read-only implementation authority. Resolve conflicts in lower-priority files.
- Skip required implementation sections, leave stubs, or replace complete work with TODOs.
- Claim a check passed without running it and inspecting its result.
- Use host execution for project development; use the prescribed isolated/container workflow.

## CRITICAL - ALWAYS DO
- Treat SPEC.md (when present) as the project override, then AI.md, then global guidance; IDEA.md defines product intent.
- Read only relevant spec sections before work, revisit those sections during work, and check compliance before reporting completion.
- Search the repository and relevant docs before asking questions or creating duplicate mechanisms.
- Translate every human-readable string and keep user-facing docs synchronized with implementation.
- Preserve existing user changes; make complete, scoped changes and report limitations truthfully.
- Follow the complete implementation and compliance checklist in AI.md PART 0 and PART 1.

## KEY DECISIONS (pre-answered)
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Source of implementation requirements? | SPEC.md overrides, otherwise AI.md; IDEA.md supplies product behavior | PART 0 |
| May AI.md be edited during project work? | No; it is read-only | PART 0 |
| How are optional PARTs 34–36 activated? | Project activation is defined by the spec's SPEC.md/IDEA.md rules; verify before implementation | PART 0 |
| What if the spec appears contradictory? | Verify both passages and log a verified internal defect in the designated tracker; do not silently choose | PART 0 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| `{project_dir}` | Git repository root, resolved with `git rev-parse --show-toplevel` |
| AI.md | Read-only implementation specification |
| IDEA.md | Project-specific WHAT: scope, users, flows, constraints |
| SPEC.md | Project-specific rule override when present |

## QUICK REFERENCE
- Before changes: inspect relevant rule file, AI.md PART, IDEA.md, and existing implementation.
- During changes: keep edits scoped, preserve work, and implement complete behavior.
- Before reporting: run only the checks authorized by the task and report exact commands/results.

---
For complete details, see AI.md PART 0, 1
