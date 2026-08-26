# Project Structure Rules (PART 2, 3, 4)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Use a license other than MIT for the project itself
- Omit third-party license attribution in LICENSE.md for MIT/Apache/BSD/ISC/MPL deps
- Use GPL/AGPL/LGPL-licensed dependencies
- Create a root file not on the Allowed Root Files list — ask first
- Put source files outside `src/`
- Hardcode `{internal_org}`/`{internal_name}` paths — resolve via `src/paths`
- Use `.yaml` for the config filename — always `server.yml`

## CRITICAL - ALWAYS DO
- Keep LICENSE.md's embedded-license section current with go.mod deps
- Follow the exact directory layout in PART 3 (src/, docker/, docs/, tests/, scripts/, .github/workflows/)
- Resolve OS-specific paths (Linux/macOS/BSD/Windows/Docker, privileged vs user) via `src/paths`
- Auto-migrate a legacy `server.yaml` to `server.yml` on startup
- Keep `docker/rootfs/` committed (build-time overlay), `binaries/`/`releases/`/`volumes/` gitignored

## KEY DECISIONS (pre-answered)
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| License | MIT, `LICENSE.md` at root | PART 2 |
| Copyright holder | webappsgo | PART 2 |
| Config filename | `server.yml` (never `.yaml`) | PART 4, 5 |
| Docker container paths | `/config`, `/data` (container-only) | PART 4 |
| Namespacing | `{internal_org}/{internal_name}` = webappsgo/redxt | PART 4 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| internal_org / internal_name | Frozen path-namespace identifiers (webappsgo/redxt) |
| project_name | Public binary/repo name (redxt) |

## QUICK REFERENCE
- Privileged Linux config: `/etc/webappsgo/redxt/server.yml`
- User Linux config: `~/.config/webappsgo/redxt/server.yml`
- Docker config: `/config/redxt/server.yml`
- SQLite DB dir: `.../db/` (server.db, users.db)

---
For complete details, see AI.md PART 2, 3, 4
