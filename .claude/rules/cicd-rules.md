# CI/CD Rules (PART 28)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Never call Makefile targets from CI/CD.
- Never use unpinned third-party actions or silently omit a required provider workflow.
- Never publish or deploy from a workflow without the spec's explicit conditions.

## CRITICAL - ALWAYS DO
- Use explicit toolchain commands and the documented provider workflow set.
- Pin actions and verify their immutable commit hashes.
- Run required tests, linters, security checks, and artifact checks for each supported provider.
- Keep workflows synchronized across GitHub, GitLab, Gitea/Forgejo, and Jenkins where required.

## KEY DECISIONS
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Use `make` in CI? | No | PART 26, 28 |
| Pin CI actions? | Yes, immutable SHAs | PART 28 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| Workflow | Provider-native pipeline with explicit commands |
| SHA pin | Immutable action/plugin commit identifier |

## QUICK REFERENCE
Follow provider-specific event, permissions, caching, artifact, and deployment rules in AI.md.

---
For complete details, see AI.md PART 28
