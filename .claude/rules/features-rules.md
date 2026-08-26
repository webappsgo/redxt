# Features Rules (PART 18-23)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Use an external cron daemon — the internal scheduler (PART 19) is required
- Skip sane email template defaults — every template must render usably out of the box
- Create premium/paid tiers for any built-in feature (GeoIP, metrics, backup, update, notifications) — all free
- Silently drop backups — backup/restore must be verifiable

## CRITICAL - ALWAYS DO
- Auto-detect SMTP where possible; expose full config when not
- Provide `--update` self-update command with signature/checksum verification
- Provide built-in scheduler for all periodic tasks (root-hints refresh, backups, metrics rollups, etc.)
- Treat GeoIP as a risk signal only, never a sole access gate
- Expose Prometheus-compatible `/metrics` with required categories

## KEY DECISIONS (pre-answered)
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Scheduler | Built-in only, never external cron | PART 19 |
| GeoIP source | ip-location-db, single canonical source per category | PART 20 |
| Metrics format | Prometheus-compatible | PART 21 |
| Backup command | `--backup` / admin panel, restorable | PART 22 |
| Update mechanism | `--update` self-update, signed | PART 23 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| Notification | In-app/WebUI alert |
| Email | SMTP-delivered message, template-driven |

## QUICK REFERENCE
- Notification vs Email: use the decision matrix in PART 18 before choosing a channel
- Backup/restore covers config + DB + zone data

---
For complete details, see AI.md PART 18, 19, 20, 21, 22, 23
