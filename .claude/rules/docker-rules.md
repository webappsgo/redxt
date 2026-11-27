# Docker Rules (PART 27)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Never put Dockerfiles in the repository root.
- Never run containers as root unless the documented setup step requires it.
- Never publish images from a local build target or CI workflow that is not designated to publish.

## CRITICAL - ALWAYS DO
- Keep Docker files under `docker/` and follow the required rootfs layout.
- Build with CGO disabled and the required platform matrix.
- Use health checks, least privilege, and documented volume/config/data paths.
- Keep development, test, and production compose behavior aligned with AI.md.

## KEY DECISIONS
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Dockerfile location | `docker/Dockerfile` | PART 27 |
| Local build push | Never | PART 26, 27 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| Image | Runtime artifact for the server/client/agent as specified |
| Compose | Project-scoped local service orchestration |

## QUICK REFERENCE
Use the exact image names, stages, labels, architecture coverage, and compose profiles from AI.md.

---
For complete details, see AI.md PART 27
