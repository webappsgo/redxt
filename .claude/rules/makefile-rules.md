# Makefile Rules (PART 26)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Never use Makefile targets in CI/CD workflows.
- Never add targets outside the six targets defined by PART 26.
- Never tag, push, or publish as a side effect of `release`.

## CRITICAL - ALWAYS DO
- Keep exactly `build`, `test`, `release`, `docker`, `clean`, and `help`.
- Make `help` the default goal.
- Build debug artifacts with symbols and all required platforms; release artifacts are optimized and checksummed.
- Keep Docker builds local and non-publishing.

## KEY DECISIONS
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| CI target mechanism | Explicit commands, no Makefile | PART 28 |
| Default target | `help` | PART 26 |
| Docker target | Host platform, local load, no push | PART 26 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| Debug build | Symbols and DWARF retained for local work |
| Release build | Optimized cross-platform artifacts and checksums |

## QUICK REFERENCE
`test` runs the specified quality checks, Go coverage collection, and `tests/run_tests.sh`.

---
For complete details, see AI.md PART 26
