# Backend Rules (PART 9, 10, 11, 32)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Leak stack traces or internal errors to clients in production
- Skip connection pooling / query timeouts on any DB driver
- Store passwords in plaintext or with bcrypt — Argon2id only
- Treat GeoIP or any single signal as the sole access-control gate
- Run relay/exit/proxy functions on the Tor hidden service — app-scoped only
- Auto-enable I2P — opt-in only via `features.i2p.enabled`

## CRITICAL - ALWAYS DO
- Support multiple DB drivers via `DATABASE_DRIVER`/`DATABASE_URL` (sqlite, libsql/turso, postgres, mysql, mssql, mongodb)
- Implement graceful panic recovery (log + 500) in every mode
- Implement cluster support for horizontal scaling (cluster nodes = other redxt instances)
- Auto-enable Tor hidden service when a Tor binary is found (PART 32.1, required)
- Enforce security headers, CSP, and public-endpoint safety principles on every response

## KEY DECISIONS (pre-answered)
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Password hashing | Argon2id | PART 11 |
| Token hashing | SHA-256 | PART 11 |
| Tor hidden service | Required, auto-enabled if Tor found | PART 32.1 |
| I2P eepsite | Optional, opt-in only | PART 32.2 |
| Cluster vs managed node | Cluster = other redxt instances; managed = external resources | PART 10 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| Cluster Node | Another instance of redxt |
| Managed Node | External DNS/redirect target monitored by redxt |

## QUICK REFERENCE
- Caching: config-driven, disabled by default in development for hot reload
- Error responses: unified format across REST/Swagger/GraphQL (PART 9, 14)

---
For complete details, see AI.md PART 9, 10, 11, 32
