# Makefile Rules (PART 26)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Use Makefile targets in CI/CD; workflows must show explicit commands and flags.
- Run Go compilation, testing, or tooling directly on the host.
- Guess repository identity, build metadata, cache paths, or output destinations when the spec provides a source of truth.
- Omit required platforms or leave temporary/build outputs in the source tree when the Makefile specifies isolated paths.

## CRITICAL - ALWAYS DO
- Keep the Makefile aligned with PART 26's local development targets and Docker-only Go toolchain.
- Use `casjaysdev/go:latest` for local Go commands and pass required environment/ldflags explicitly.
- Create mounted Go cache/build directories before invoking Docker and keep outputs in their specified locations.
- Build all eight supported OS/architecture targets with canonical names and version metadata.
- Keep `make test` behavior and test/coverage gates exactly as defined in PARTs 26 and 29.

## KEY DECISIONS (pre-answered)
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Is Makefile for CI? | No; it is for local development only | PART 26, 28 |
| Where does local Go run? | In the specified Docker toolchain container | PART 26 |
| Which targets are required? | Follow the exact PART 26 target contract | PART 26 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| `GO_CACHE` | Host cache directory mounted into the container as specified |
| `GO_BUILD` | Host build cache directory mounted into the container as specified |
| `BINDIR` | Build artifact directory passed consistently to Makefile and tests |

## QUICK REFERENCE
- Use PART 26 for target names, build flags, variables, platform matrix, and cleanup rules.
- Use PART 28 for explicit CI commands; local Makefile behavior is not a CI substitute.
- Use PART 29 for test phases and coverage thresholds.

---
For complete details, see AI.md PART 26
