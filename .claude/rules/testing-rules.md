# Testing / Docs / i18n Rules (PART 29, 30, 31)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Run tests or debugging against the host system — use Docker/Incus per `execution_hierarchy.md`
- Use the project directory for scratch/temp test output — use the project-org tempdir structure
- Commit runtime-generated config files
- Skip browser E2E beta testing (`tests/e2e.sh`, headless Chromium) — required by PART 29
- Ship translated strings without a11y review (PART 31)

## CRITICAL - ALWAYS DO
- Provide `tests/run_tests.sh`, `tests/docker.sh`, `tests/incus.sh`, `tests/e2e.sh` (all required)
- Keep Go unit tests as `*_test.go` next to the code they test
- Meet the required test coverage gate before commit
- Configure `mkdocs.yml` + `.readthedocs.yaml` + `docs/requirements.txt` + theme CSS (dark.css required, light.css optional)
- Support i18n string tables and mobile a11y from day one

## KEY DECISIONS (pre-answered)
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Test host | Docker/Incus only, never bare host | PART 29 |
| Docs generator | MkDocs Material via ReadTheDocs | PART 30 |
| i18n scope | All user-facing strings, WCAG a11y | PART 31 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| E2E test | Headless-Chromium browser test in `tests/e2e.sh` |

## QUICK REFERENCE
- Required docs pages: index, installation, configuration, api, cli, admin, security, integrations, development

---
For complete details, see AI.md PART 29, 30, 31
