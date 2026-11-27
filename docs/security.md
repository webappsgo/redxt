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
- `GET /.well-known/security.txt` adds the key fingerprint comment and `Encryption` URL while that keypair is active; the key is also published to configured keyservers with durable retry state.
- `GET /server/security/report/{tracking_id}?token=…` returns only the researcher's status, comment, and expected disclosure date; the token is limited to one use per UTC day.

Security report bodies use the active project OpenPGP key when present and AES-256-GCM otherwise. Researcher contact details always use AES-256-GCM. A researcher supplied OpenPGP key protects the acknowledgment email, which contains only the tracking ID and status URL.

Server Admins manage reports under `/server/{admin_path}/security/reports`. Opening a report decrypts its stored contents only for that authenticated request and writes a content-free audit event. Status updates move forward through `received`, `triaged`, `confirmed`, `patching`, and `disclosed`, or terminate as `wont_fix`. Disclosure updates create the acknowledgment record in the same database transaction. The policy editor is at `/server/{admin_path}/security/policy`; acknowledgments are managed at `/server/{admin_path}/security/thanks`.

The keypair page at `/server/{admin_path}/security/keypair` supports generation, rotation, keyserver republishing, public-key download, private-key import, and decrypted private-key export. Import and private-key export require administrator password confirmation. A private-key export also requires a reason and is limited to one download per admin per hour; the audit record contains the admin, client IP, and reason. Rotation re-encrypts retained PGP report bodies transactionally and retains the prior encrypted private key for 30 days. Imported keys must match the configured identity unless the admin explicitly confirms the mismatch.

Each Server Admin can register an ASCII-armored personal public key at `/server/{admin_path}/{admin_username}/profile/security/pgp`. Vulnerability report notification emails are encrypted separately to every admin who has registered a key. The server sends an AES-256-GCM encrypted fallback with a warning only when no admin has a registered key.

Closed reports are removed by the daily `security_cleanup` scheduler task after `server.security.reports.retention_days`; open reports are retained. A retired OpenPGP private key is removed 30 days after rotation. `keyserver_retry` retries transient publication failures every 15 minutes, using `security/keyservers.state` so success and retry state survive restarts. CAPTCHA challenges expire after ten minutes and are removed by `token_cleanup`.

Backups include `security/pgp.pub.asc`, `security/pgp.priv.asc.enc`, and `security/keyservers.state` whenever present. Restore writes the private key and state with owner-only permissions, then publishes the restored public key only where the restored keyserver state does not already record successful publication.

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
