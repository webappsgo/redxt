# Email, Scheduler, GeoIP, Metrics, Backup & Update Rules (PART 18–23)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Send or queue email when SMTP is unavailable; disable email-dependent features cleanly and never claim an email was sent.
- Use external cron for application tasks or let local time/cluster topology create duplicate global task execution.
- Treat GeoIP as a sole access decision or embed frequently updated security databases in release binaries.
- Expose metrics without the required authentication/access controls or emit unbounded/high-cardinality labels such as raw client IPs.
- Create unverifiable backups, restore live SQLite files by copying them, or update binaries without the required integrity checks.

## CRITICAL - ALWAYS DO
- Provide customizable, embedded-default email templates and admin configuration; follow PART 18's SMTP detection, validation, and disabled-state behavior.
- Use the built-in scheduler for periodic work, with task registration, persistence, cluster-aware execution, failure reporting, and admin controls per PART 19.
- Keep GeoIP source, update, licensing, storage, and fallback behavior canonical to PART 20; use it only as a risk signal.
- Expose required Prometheus metrics with protected endpoints, bounded labels, and the required metric categories.
- Make backups complete, integrity-checked, restorable, and consistent with database/SSL content rules.
- Apply the update command's release eligibility, download, signature/checksum, platform, and rollback safeguards.

## KEY DECISIONS (pre-answered)
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Is an external cron daemon allowed? | No; use the internal scheduler | PART 19 |
| What happens when SMTP cannot be used? | Email stays disabled; dependent flows are hidden/skipped as specified | PART 18 |
| Is GeoIP an access-control gate? | No; it is a risk signal only | PART 20 |
| Are metrics public by default? | No; require bearer authentication unless the explicitly firewalled exception is configured | PART 21 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| Cluster-aware task | Task whose execution scope is coordinated across cluster nodes per PART 19 |
| GeoIP | External geolocation/ASN data used as one security signal |
| Integrity-checked backup/update | Artifact checked with the cryptographic guarantees specified in PART 22/23 |

## QUICK REFERENCE
- PART 18: templates, SMTP availability, safe account emails, notification delivery.
- PART 19: scheduler task contracts and global/local execution.
- PART 20–21: external GeoIP data and authenticated Prometheus metrics.
- PART 22–23: verified backup/restore and secure self-update.

---
For complete details, see AI.md PART 18, 19, 20, 21, 22, 23
