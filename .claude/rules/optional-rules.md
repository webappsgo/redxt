# Multi-User, Organizations & Custom Domains Rules (PART 34–36)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

**Project selection (IDEA.md):** PART 34 Multi-User, PART 35 Organizations, and PART 36 Custom Domains are enabled. Follow IDEA.md for DNS-specific product behavior and AI.md for implementation rules.

## CRITICAL - NEVER DO
- Merge regular-user accounts, sessions, permissions, or credential storage with server-admin accounts.
- Implement Organizations without Multi-User; PART 35 depends on PART 34.
- Treat optional sections as permission to omit baseline AI.md behavior once selected.
- Allow cross-user or cross-organization access by trusting client-supplied IDs or slugs.
- Activate custom domains before verifying ownership, or issue/renew certificates outside PART 36's challenge and domain-validation flow.
- Invent registration, recovery, identity-provider, visibility, role, or domain behavior not defined by IDEA.md and AI.md.

## CRITICAL - ALWAYS DO
- Implement the selected PARTs completely, including required web/API routes, settings, roles, security flows, validation, tests, and admin controls.
- Default registration to IDEA.md's `invite` mode; support the modes and controls defined by PART 34 and the project intent.
- Enforce organization-scoped authorization and isolation for every product resource; each user has the personal/shared organization behavior specified by IDEA.md.
- Preserve the separation between server admins and regular users and apply PART 34's MFA, tokens, privacy, recovery, moderation, and route rules.
- Apply PART 35's organization creation, membership roles, settings, visibility, and profile rules.
- Apply PART 36's domain ownership verification, resolution, DNS-01/other specified challenge handling, per-domain SSL, renewal, and admin management.

## KEY DECISIONS (pre-answered)
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Are PARTs 34–36 selected for this project? | Yes: Multi-User, Organizations, and Custom Domains | IDEA.md |
| What is the registration default? | `invite`; admins may select the modes defined in IDEA.md/PART 34 | IDEA.md, PART 34 |
| Are server admins regular users? | No; distinct identities, tables, permissions, and flows | PART 17, 34 |
| Does custom-domain activation require proof? | Yes; domain ownership verification precedes activation | PART 36 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| Server Admin | Manages the redxt instance; not a privileged OS user or regular user |
| Regular User | End-user account subject to PART 34 |
| Organization | Shared tenant boundary for product resources as specified in IDEA.md |
| Custom domain | Verified domain mapped to a user/org resource under PART 36 |

## QUICK REFERENCE
- Organization isolation and DNS resource ownership are enforced server-side on every route and data lookup.
- API tokens never exceed their issuer's effective role/scope.
- User/org vanity profile and privacy behavior follows the selected optional sections and IDEA.md.
- PARTs 34–36 are optional in the template but fully mandatory for this selected project.

---
For complete details, see AI.md PART 34, 35, 36
