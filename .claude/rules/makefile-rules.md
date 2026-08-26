# Makefile Rules (PART 26)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Use the Makefile in CI/CD — CI uses explicit commands only
- Hardcode project name/org — infer from `git remote get-url origin`
- Build on the host — always via `casjaysdev/go:latest` in Docker
- Skip any of the 8 target platforms in `make build`

## CRITICAL - ALWAYS DO
- Provide `build`, `release`, `docker`, `test`, `dev`, `clean` targets
- Stamp `Version`/`CommitID`/`BuildDate`/`OfficialSite` via `-ldflags`
- Use `-e GOFLAGS=-buildvcs=false` for all dockerized Go invocations
- Create `$(GO_CACHE)`/`$(GO_BUILD)` dirs before any Docker-mounted build

## KEY DECISIONS (pre-answered)
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Toolchain image | `casjaysdev/go:latest`, never pinned | PART 26 |
| Binary naming | `{project_name}-{GOOS}-{GOARCH}` | PART 26 |
| Local dev vs CI | Makefile is local-dev only | PART 26 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| GO_DOCKER | The `docker run` wrapper invoking the toolchain image |

## QUICK REFERENCE
- `make test` = `go vet` + `govulncheck` + `go test -v -cover`, all inside Docker

---
For complete details, see AI.md PART 26
