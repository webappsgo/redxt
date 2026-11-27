# Configuration Rules (PART 5, 6, 12)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Never hardcode development values or secrets.
- Never store credentials in the public configuration file or expose them in responses/logs.
- Never silently accept malformed, unknown, or wrongly typed configuration.

## CRITICAL - ALWAYS DO
- Detect runtime environment and select documented defaults.
- Validate configuration on the server and warn with the setting's source and name.
- Keep secrets in private `secrets.yml`; create and persist generated secrets before serving requests.
- Resolve configuration by the PART 12 precedence and drop-in merge rules.

## KEY DECISIONS
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Where is server config? | `{config_dir}/server.yml` | PART 5 |
| Where are secrets? | Private `secrets.yml` | PART 12 |
| How are development values selected? | Runtime detection | PART 6 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| Config drop-in | A sorted YAML file under `yaml.d/` merged over `server.yml` |
| Secret | Credential material kept outside public configuration |

## QUICK REFERENCE
Apply configuration precedence, strict validation, private permissions, and atomic persistence from AI.md. Never guess a default.

---
For complete details, see AI.md PART 5, 6, 12
