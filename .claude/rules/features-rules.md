# Feature Rules (PART 18-23)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Never use external cron for scheduled work.
- Never implement premium tiers, paywalls, or paid-only features.
- Never send email or webhooks with unvalidated destinations or leaked secrets.

## CRITICAL - ALWAYS DO
- Use the built-in scheduler and persist task state as specified.
- Include metrics, GeoIP, notifications, backups, and updates required by the project.
- Make backup/restore verify integrity, limits, encryption, and retention rules.

## KEY DECISIONS
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Scheduled jobs | Internal scheduler | PART 19 |
| Feature pricing | All features free | PART 1 |
| Backup trust | Verify before restore | PART 22 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| Built-in task | Scheduler-managed recurring or one-shot job |
| Backup | Verified application data and required secrets snapshot |

## QUICK REFERENCE
Apply the complete per-feature contracts, lifecycle rules, security constraints, and failure behavior in AI.md.

---
For complete details, see AI.md PART 18-23
