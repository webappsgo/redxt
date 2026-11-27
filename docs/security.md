# Security

## Authentication & Identity

- Passwords are hashed with Argon2id; bearer tokens are stored as SHA-256 hashes.
- Server Admins and Regular Users remain separate identities and database records.
- Credentials and cryptographic secrets are not returned by APIs or written to logs.

## Public Security Endpoints

- `GET /.well-known/security.txt` serves the RFC 9116 reporting contacts and expiry.
- `GET /server/security` shows the configured reporting channels and disclosure resources.
- `GET /server/security/policy` publishes the coordinated-disclosure policy.
- `GET /server/security/thanks` lists researchers whose disclosures were published.
- `GET /api/v1/server/security`, `/policy`, and `/thanks` provide JSON mirrors.
- `GET /.well-known/pgp-key.asc` is available when a project keypair is configured.
- `GET /server/security/report/{tracking_id}?token=…` returns only the researcher's status, comment, and expected disclosure date; the token is limited to one use per UTC day.

Server Admins manage reports under `/server/{admin_path}/security/reports`. Opening a report decrypts its stored contents only for that authenticated request and writes a content-free audit event. Status updates move forward through `received`, `triaged`, `confirmed`, `patching`, and `disclosed`, or terminate as `wont_fix`. Disclosure updates create the acknowledgment record in the same database transaction. The policy editor is at `/server/{admin_path}/security/policy`; acknowledgments are managed at `/server/{admin_path}/security/thanks`.

Closed reports are removed by the daily `security_cleanup` scheduler task after `server.security.reports.retention_days`; open reports are retained. CAPTCHA challenges expire after ten minutes and are removed by `token_cleanup`.

The GitHub private vulnerability reporting URL is the primary channel for
source-code vulnerabilities. A deployment's security-report form is included
when enabled. The optional security email is a secondary contact and is read
from the live `server.contact.security.email` configuration; the admin email
is never published. Security form links use a deterministic HMAC identifier
that rotates every 48 hours and accepts the preceding window during transition.

## Transport Security

- Let's Encrypt HTTP-01 (port 80) and TLS-ALPN-01 (port 443) issuance is built in.
- Tor hidden-service support is available when Tor is detected.
- I2P eepsite support is controlled by the resolved I2P configuration.
