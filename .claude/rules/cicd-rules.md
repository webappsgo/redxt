# CI/CD Workflow Rules (PART 28)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Use Makefile targets or inline-install tools in CI; use explicit provider-native commands and preinstalled toolchains.
- Weaken, skip, or suppress build, test, coverage, lint, secret-scan, workflow-policy, vulnerability, image, release-integrity, or artifact checks on any provider.
- Pin GitHub/Gitea/Forgejo actions only by a tag or GitLab/Jenkins images without the required digest pin.
- Expose write credentials to fork pull requests or grant workflow-wide write permissions.
- Auto-cancel runs across different refs; concurrency must be scoped to the same branch or exact tag.
- Add conditional workflows for images/features whose required project artifact or IDEA.md selection is absent.

## CRITICAL - ALWAYS DO
- Maintain equivalent required CI/CD gates for GitHub Actions, GitLab CI, Gitea Actions, Forgejo Actions, and Jenkins.
- For GitHub/Gitea/Forgejo, maintain `ci.yml`, `release.yml`, `beta.yml`, `devel.yml`, and `docker.yml`; keep required provider-native equivalents and `Jenkinsfile`.
- Use `casjaysdev/go:latest` directly for Go jobs; use the host runner only for jobs requiring the host Docker daemon. Never add inline tool installation or a build-toolchain preflight for Go.
- Use the triggers, per-ref concurrency, least-privilege permissions, explicit build metadata, all-platform matrices, and fork-secret isolation in PART 28.
- Keep test, lint, coverage, secret scanning, workflow policy, vulnerability scanning, and image scanning gates enabled at their specified triggers and thresholds.
- Pin actions by full commit SHA and container images by digest where required; retain Renovate version annotations.
- Preserve release preflight ownership of tags and checksums, build all release assets, validate generated release text, and fail on critical/high vulnerabilities.
- Check the real provider CI run after every push; local syntax/job-graph checks do not prove the hosted build passed.

## KEY DECISIONS (pre-answered)
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Which providers are mandatory? | GitHub, GitLab, Gitea, Forgejo, Jenkins | PART 28 |
| Is a Go build-toolchain image workflow needed? | No; Go uses `casjaysdev/go:latest` directly | PART 28 |
| Are action tags sufficient pins? | No; use full commit SHA | PART 28 |
| Is a dedicated AIO workflow always required? | Only if the project selects AIO and provides the required AIO Dockerfile | PART 28 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| Hosted CI run | Real run on the configured provider, with its actual secrets and runners |
| Build metadata | Runtime-derived version, commit ID, build epoch/date, and official-site values |
| Security-only scheduled run | Weekly security jobs without build/test/release work |

## QUICK REFERENCE
- CI runs build/test/lint/coverage and security gates; weekly schedules run only the specified security jobs.
- Branch pushes and tag releases use separate exact-ref concurrency groups.
- Release and image workflows must preserve checksums, signatures/integrity rules, labels, and asset validation.
- See PART 28 for provider-specific syntax, runner/toolchain rules, workflow triggers, and gate graphs.

---
For complete details, see AI.md PART 28
