# Optional Rules — Multi-User, Organizations, Custom Domains (PART 34-36)

⚠️ **These rules are NON-NEGOTIABLE ONCE IMPLEMENTED. Violations are bugs.** ⚠️

**Status for this project (per IDEA.md):** PART 34 (Multi-User), PART 35 (Organizations), and PART 36 (Custom Domains) are ALL enabled for redxt.

## CRITICAL - NEVER DO
- Merge Regular User accounts with Server Admin accounts — always separate DB tables
- Implement Organizations without Multi-User already in place (PART 35 requires PART 34)
- Leave unused optional-feature code paths in the codebase if a feature is ultimately disabled
- Skip domain ownership verification before activating a custom domain

## CRITICAL - ALWAYS DO
- Implement full Regular User registration, roles/permissions, profile, and API tokens (PART 34)
- Implement Organization creation modes, profiles, and org-scoped user visibility (PART 35)
- Implement custom-domain verification flow (DNS/HTTP challenge) and SSL issuance per domain (PART 36)
- Keep public vanity-URL profiles (user and org) consistent with privacy settings

## KEY DECISIONS (pre-answered)
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Multi-user enabled? | Yes | IDEA.md, PART 34 |
| Organizations enabled? | Yes | IDEA.md, PART 35 |
| Custom domains enabled? | Yes | IDEA.md, PART 36 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| Vanity URL | Public profile path for a user or org |
| Custom domain | User/org-branded domain mapped onto redxt |

## QUICK REFERENCE
- Custom domain SSL: built-in Let's Encrypt (PART 15) issuance per verified domain

---
For complete details, see AI.md PART 34, 35, 36
