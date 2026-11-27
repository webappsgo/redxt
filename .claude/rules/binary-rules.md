# Binary Rules (PART 7, 8, 33)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Never enable CGO.
- Never omit a required target platform or rename a required binary.
- Never put secrets in command-line arguments, logs, or help output.

## CRITICAL - ALWAYS DO
- Build with `CGO_ENABLED=0`.
- Keep server `redxt`, required client `redxt-cli`, and agent `redxt-agent` distinct.
- Support all eight Linux, macOS, Windows, and FreeBSD amd64/arm64 targets.
- Follow PART 8 CLI, environment, config, data, and log path resolution.

## KEY DECISIONS
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| What is the server binary? | `redxt` | PART 7 |
| Is the client required? | Yes, `redxt-cli` | PART 7, 33 |
| Is CGO allowed? | No | PART 7 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| Server | Main long-running service binary |
| Client | Required CLI/TUI/GUI companion |
| Agent | `redxt-agent`, manages external nodes |

## QUICK REFERENCE
Use documented flags, environment variables, stable paths, and cross-platform build names.

---
For complete details, see AI.md PART 7, 8, 33
