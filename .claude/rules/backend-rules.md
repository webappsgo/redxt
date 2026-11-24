# Backend, Database, Security & Overlay Network Rules (PART 9, 10, 11, 32)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Return internal error details, credentials, tokens, session material, full connection URLs, or sensitive request data to public endpoints or logs.
- Use unparameterized SQL, omit connection pooling/query timeouts, or perform destructive schema changes.
- Treat SQLite as safe for concurrent writes without the per-file read/write handle and transaction contract in PART 10.
- Treat GeoIP, a blocklist, or any single signal as the sole security/access-control gate.
- Let untrusted inbound requests choose destinations fetched through Tor; expose Tor SOCKS beyond loopback or enable relay/exit behavior.
- Enable I2P unless explicitly configured; it is optional and its platform/container conditions are specified in PART 32.

## CRITICAL - ALWAYS DO
- Support the database modes and drivers actually specified in PART 10; use SQLite for the default single-node mode and remote database/cache requirements for clustering.
- Use contextual query/transaction timeouts, pooled connections, idempotent additive schema upgrades, and transaction rollback discipline.
- Preserve the distinction between cluster nodes (redxt instances) and agents/managed external nodes.
- Apply panic recovery and the public-endpoint disclosure tiers, security headers, CSP, and structured logging requirements.
- Hash passwords with Argon2id and tokens with SHA-256; never persist raw credentials.
- Auto-enable the required Tor hidden-service behavior when available, while keeping outbound requests app-scoped and Tor relay/exit disabled.

## KEY DECISIONS (pre-answered)
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Default database mode? | Local SQLite for a single instance | PART 10 |
| What enables cluster mode? | Shared supported remote database and cache as specified | PART 10 |
| Password/token storage? | Argon2id password hashes; SHA-256 token hashes | PART 11 |
| May inbound visitors use the app as a Tor proxy? | No; outbound Tor destinations must be app-determined | PART 32 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| Cluster node | Another redxt server instance sharing cluster services |
| Agent | Purpose-specific worker; never a cluster node |
| Public-safe data | Information explicitly allowed for unauthenticated disclosure by PART 11 |

## QUICK REFERENCE
- PART 9 defines error handling, recovery, and caching.
- PART 10 defines database drivers, schema, pooling, SQLite concurrency, and cluster behavior.
- PART 11 defines security, credential handling, endpoint disclosure, and logging.
- PART 32 defines Tor/I2P; Tor hidden-service and outbound-client trust boundaries are separate.

---
For complete details, see AI.md PART 9, 10, 11, 32
