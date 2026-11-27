# Project Rules (PART 2, 3, 4)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Never place a Dockerfile at the repository root; use `docker/`.
- Never replace project identity with a guessed value; read IDEA.md project variables.
- Never add attribution or generated-by credits to project output.

## CRITICAL - ALWAYS DO
- Keep `{project_name}` mutable and `{internal_name}` / `{internal_org}` stable for on-disk identity.
- Follow the required source, test, documentation, and platform directory layout in AI.md.
- Use the project license and attribution requirements exactly as specified.

## KEY DECISIONS
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Where are source files? | `src/` for Go source | PART 3 |
| Where is the Dockerfile? | `docker/Dockerfile` | PART 3, 27 |
| Which identity names persist on disk? | `internal_name`, `internal_org` | PART 4 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| `{project_name}` | User-facing project name, may change |
| `{internal_name}` | Frozen on-disk identifier |
| `{internal_org}` | Frozen organization identifier |

## QUICK REFERENCE
Resolve placeholders from IDEA.md; preserve required project structure, license, and attribution rules.

---
For complete details, see AI.md PART 2, 3, 4
