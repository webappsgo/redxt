# Optional Feature Rules (PART 34-36)

⚠️ **These rules are NON-NEGOTIABLE when the feature is enabled.** ⚠️

## CRITICAL - NEVER DO
- Never implement these optional parts unless activated by SPEC.md.
- Never treat a Server Admin as a Regular User or merge their credential stores.
- Never bypass organization membership, ownership, or custom-domain verification.

## CRITICAL - ALWAYS DO
- Implement every activated PART 34-36 requirement completely.
- Keep users, organizations, roles, sessions, and tokens scoped to their owner.
- Verify custom-domain control before activation and revalidate it according to AI.md.

## KEY DECISIONS
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Are PARTs 34-36 baseline? | No; activation is explicit in SPEC.md | PART 34-36 |
| Are account types interchangeable? | No; Server Admin and Regular User are separate | PART 34 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| Regular User | Application end user |
| Organization | User-owned collaborative tenant |
| Custom domain | Verified domain attached to an eligible organization |

## QUICK REFERENCE
The project SPEC.md activates optional parts; IDEA.md describes product behavior, AI.md governs implementation.

---
For complete details, see AI.md PART 34-36
