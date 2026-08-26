# Docker Rules (PART 27)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Put a Dockerfile in the project root — always `docker/Dockerfile`
- Create `docker/Dockerfile.build` for a Go project without a genuine custom-toolchain need
- Bundle debug mode into the production compose file
- Skip multi-arch (`linux/amd64,linux/arm64`) builds

## CRITICAL - ALWAYS DO
- Multi-stage build: `casjaysdev/go:latest` toolchain stage → minimal runtime stage
- Provide `docker-compose.yml` (prod), `docker-compose.dev.yml` (dev), `docker-compose.test.yml` (`:devel` image, `DEBUG: true`, `MODE: development`, valkey cache)
- Use `/config` and `/data` container-only paths, mapped from `./volumes/config` and `./volumes/data`
- Commit `docker/rootfs/` (build-time overlay: entrypoint.sh, service configs)
- Tag `:devel` for the dev image (same as release, binary runs in debug mode)

## KEY DECISIONS (pre-answered)
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Root Dockerfile? | Never — `docker/Dockerfile` only | PART 27 |
| Dockerfile.build for Go? | No, unless a genuine custom toolchain need exists | PART 27, go_conventions.md |
| Container internal port | 80 | PART 4, 27 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| rootfs | Build-time container filesystem overlay, committed |

## QUICK REFERENCE
- `.dockerignore` excludes `.git/`, `.github/`, `docs/`, `tests/`, `Makefile`, AI-config dirs

---
For complete details, see AI.md PART 27
