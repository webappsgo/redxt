# Service Rules (PART 24, 25)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Never run the service as a privileged OS user to represent an app administrator.
- Never perform unbounded shutdown or cleanup work.
- Never use external service managers or cron behavior contrary to the documented lifecycle.

## CRITICAL - ALWAYS DO
- Bind required privileged ports before dropping privileges.
- Apply platform-specific service support, paths, ownership, and restart behavior from AI.md.
- Keep shutdown graceful and bounded, and report service health accurately.

## KEY DECISIONS
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Is Server Admin an OS user? | No, it is an application role | PART 24 |
| Who runs the service? | The configured unprivileged service account | PART 24, 25 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| Server Admin | App operator identity, separate from the OS account |
| Service account | Restricted operating-system identity for the daemon |

## QUICK REFERENCE
Follow the install, privilege, signal, restart, and platform service contracts in AI.md.

---
For complete details, see AI.md PART 24, 25
