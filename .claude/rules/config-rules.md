# Configuration, Modes & Server Config Rules (PART 5, 6, 12)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Put YAML comments inline, except same-line GitHub Action SHA version annotations required by Renovate.
- Treat filesystem directory overrides as URL paths or run them through URL `SafePath` normalization.
- Assume runtime configuration is committed; `server.yml`, secrets, and other runtime files are generated under OS config paths.
- Log or expose credentials, full connection URLs, generated secrets, or secret values in any mode.
- Re-read settings marked init-only on later starts or override saved cluster connection settings from the environment.
- Treat debug mode as permission to bypass authentication, validation, or security controls.

## CRITICAL - ALWAYS DO
- Normalize and validate URL/config paths; use the filesystem-specific path validation for directory overrides.
- Use precedence and first-run/init-only semantics exactly as defined in PARTs 5 and 12; persist generated values.
- Generate secret material atomically with restrictive permissions and disclose only the specifically allowed first-run token.
- Keep every setting available through the admin panel where PART 12 requires it.
- Configure production as the default mode; debug endpoints require the explicit debug flag, while `MODE=debug` only supplies the documented default.
- Keep the merged configuration/drop-in precedence and early-read/restart behavior defined in PART 12.

## KEY DECISIONS (pre-answered)
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Default port outside an application container? | Random unused port in 64000–64999, persisted to `server.yml` | PART 5 |
| Default port in an application container? | 80 unless `PORT` sets it | PART 5 |
| Is `PORT` a permanent override? | First-run seed only; the saved configuration controls later starts | PART 5 |
| May debug mode weaken security? | No; debug endpoints are independently gated by the debug flag | PART 6 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| Init-only variable | Environment input consumed once to seed initial config |
| Drop-in | Ordered config fragment merged with `server.yml` |
| Debug flag | Explicit gate for debug endpoints, independent of operational mode |

## QUICK REFERENCE
- Use PART 5 for path safety, environment inputs, defaults, and first-run persistence.
- Use PART 6 for the mode/debug state matrix and security boundaries.
- Use PART 12 for canonical server config, secrets, drop-ins, defaults, and admin-editable settings.

---
For complete details, see AI.md PART 5, 6, 12
