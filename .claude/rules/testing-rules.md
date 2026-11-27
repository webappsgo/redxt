# Testing, Docs, and I18N Rules (PART 29, 30, 31)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Never claim a build, test, lint, translation, or documentation check that was not run.
- Never leave user-facing strings untranslated or inaccessible.
- Never omit the specified integration, browser, container, or platform test layers.

## CRITICAL - ALWAYS DO
- Run the complete test harness and required coverage targets from PART 29.
- Keep README and documentation site current with actual commands and behavior.
- Use translation keys for user-visible strings and preserve locale fallback behavior.
- Meet semantic HTML, keyboard, screen-reader, contrast, and responsive requirements.

## KEY DECISIONS
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Are tests required? | Yes, every specified tier and project flow | PART 29 |
| How are UI strings written? | Localized keys, with documented fallback | PART 31 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| E2E | Browser-visible end-to-end flow through app layers |
| Locale fallback | Resolve unsupported/missing translation per PART 31 |

## QUICK REFERENCE
Docs must match implemented behavior; tests must cover observable page, API, deployment, and failure behavior.

---
For complete details, see AI.md PART 29, 30, 31
