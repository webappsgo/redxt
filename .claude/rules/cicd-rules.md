# CI/CD Workflow Rules (PART 28)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Use the Makefile inside CI — use explicit `go`/`docker` commands
- Pin third-party GitHub Actions to a tag — always a full commit SHA
- Use `container: image: casjaysdev/go:latest` plus a separate `build-toolchain.yml`/`ensure-build-image` pre-flight — not needed for Go
- Skip the Jenkinsfile — required on every project per `cicd_conventions.md`

## CRITICAL - ALWAYS DO
- Provide `ci.yml` (build/test/lint/coverage/security), `release.yml`, `beta.yml`, `daily.yml`, `docker.yml`
- Use `container: image: casjaysdev/go:latest` directly in `ci.yml`/`release.yml`
- Verify each staged workflow with `act --list -W {file}` before commit
- Create security-only workflows first, `ci.yml`/`release.yml` last

## KEY DECISIONS (pre-answered)
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Toolchain image in CI | `casjaysdev/go:latest`, direct container | PART 28 |
| Action pinning | Full commit SHA, never a tag | PART 28, cicd_conventions.md |
| Required workflow files | ci, release, beta, daily, docker | PART 28 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| Daily build | Nightly build off default branch |
| Beta build | Pre-release channel build |

## QUICK REFERENCE
- Post-push: verify the triggered CI run's status before declaring work done

---
For complete details, see AI.md PART 28
