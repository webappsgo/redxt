# License, Project Structure & OS Paths Rules (PART 2, 3, 4)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Change the project license from MIT or omit required third-party license attribution.
- Add files or directories outside AI.md's allowed project layout, or place Go source outside `src/`.
- Commit generated runtime configuration, binaries, releases, volumes, or scratch reports.
- Hardcode OS-specific config/data/log paths or the frozen `{internal_org}/{internal_name}` namespace.
- Use `server.yaml` as the canonical config name; the canonical file is `server.yml`.

## CRITICAL - ALWAYS DO
- Keep the root inventories, required directories, `.gitignore`, and project files aligned with PART 3.
- Resolve config, data, cache, log, and backup locations through the shared path package and OS rules in PART 4.
- Use `{internal_org}` and `{internal_name}` for stable on-disk/service identifiers; use project variables from IDEA.md.
- Migrate the legacy `server.yaml` file to `server.yml` according to the startup migration rules.
- Maintain LICENSE.md attribution and the project's Go module license metadata as dependencies change.

## KEY DECISIONS (pre-answered)
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Project license? | MIT | PART 2 |
| Canonical config filename? | `server.yml`; migrate legacy `server.yaml` | PART 4 |
| Where do source files live? | `src/` | PART 3 |
| Where do runtime files live? | OS-specific config/data/log/cache/backup directories | PART 4 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| `project_name` / `project_org` | Public repository/product identifiers |
| `internal_name` / `internal_org` | Stable identifiers for persistent paths and services |
| Embedded licenses | Required third-party attribution maintained in LICENSE.md |

## QUICK REFERENCE
- Required structure and allowed root files are defined by PART 3.
- Linux, macOS, BSD, Windows, privileged, user, and container paths are defined by PART 4; do not infer them.
- License compatibility, attribution format, and maintenance are defined by PART 2.

---
For complete details, see AI.md PART 2, 3, 4
