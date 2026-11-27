# Backend Rules (PART 9, 10, 11, 32)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Never leak internal error details, credentials, or personal data into responses or logs.
- Never use bcrypt for new password hashes; use Argon2id.
- Never disable required Tor/I2P code paths or weaken encryption and ownership checks.

## CRITICAL - ALWAYS DO
- Return the canonical error envelope and stable codes.
- Use idempotent schema updates and the database/cluster contracts.
- Hash bearer tokens with SHA-256; encrypt sensitive data with the configured AES-256-GCM key.
- Keep audit records sanitized and append-only; support Tor and I2P according to runtime rules.

## KEY DECISIONS
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Password hashing | Argon2id | PART 11 |
| Token storage | SHA-256 hash | PART 11 |
| Tor and I2P | Required implementation; activation follows PART 32 | PART 32 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| Cluster node | Another redxt instance |
| Managed node | External DNS/redirect target monitored by this app |

## QUICK REFERENCE
Validate every input, preserve tenant ownership, and keep private data encrypted and redacted.

---
For complete details, see AI.md PART 9, 10, 11, 32
