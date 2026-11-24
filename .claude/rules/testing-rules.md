# Testing, Documentation, i18n & Accessibility Rules (PART 29, 30, 31)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Run project tests, builds, service operations, or debugging on the host when the spec requires isolation.
- Use production/dev compose for automated testing, project-tree volumes for runtime state, or project paths for scratch output.
- Treat unit coverage as endpoint coverage; endpoint routes and content-negotiation variants have separate required coverage.
- Commit runtime-generated configuration, test databases, logs, or build artifacts.
- Ship hardcoded human-readable strings, locale files with mismatched keys/plurals, or untranslated user-facing errors.
- Leave MkDocs/ReadTheDocs docs, accessibility, or localization requirements incomplete.

## CRITICAL - ALWAYS DO
- Follow PART 29's container/VM safety, project-scoped temp directory, unit/integration test, endpoint, content-negotiation, and coverage requirements.
- Keep Go unit tests beside source and exercise every IDEA.md route and required HTML/JSON/plain-text representation.
- Maintain the required test scripts and use isolated Docker/Incus/VM paths as defined by PART 29.
- Maintain the MkDocs Material/ReadTheDocs configuration, required pages, navigation, and theme assets from PART 30.
- Translate all human-readable strings across server, client, agent, web, API, docs surfaces, and emails; use one shared locale registry/file set with key parity and English fallback.
- Meet WCAG accessibility, keyboard, screen-reader, contrast, responsive, and RTL requirements from PART 31.

## KEY DECISIONS (pre-answered)
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Where do tests/builds run? | Isolated containers or VMs according to PART 29 and execution hierarchy | PART 29 |
| What is the default unit coverage gate? | At least 60%; project-specific server endpoint coverage is 100% where specified | PART 29 |
| Which doc system is required? | MkDocs Material published through ReadTheDocs | PART 30 |
| What is the minimum locale set? | English, Spanish, French, Arabic, German, Italian; every supported locale must be complete | PART 31 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| Unit test | Fast test colocated with the code under test |
| Endpoint coverage | Integration verification of every defined route and required response format |
| Supported locale | Complete embedded locale with matching registry and English key set |

## QUICK REFERENCE
- Content negotiation is tested for browser HTML and plain text; API JSON and plain text; `.txt` routes where supported.
- Unit coverage and endpoint coverage are separate gates; verify each threshold in PART 29.
- `tests/` outputs and runtime state belong in project-scoped OS temp directories.
- Read PARTs 29, 30, and 31 for required scripts, docs pages/theme, locale registry, translation use, and accessibility.

---
For complete details, see AI.md PART 29, 30, 31
