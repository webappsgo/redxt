# Health, API Structure & SSL/TLS Rules (PART 13, 14, 15)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Implement routes that are absent from the project's API inventory or keep route aliases after the spec requires canonical terms.
- Let native/protocol routes, REST, OpenAPI/Swagger, GraphQL, and frontend behavior drift from the specified contracts.
- Expose health, version, or debug information beyond the disclosure and authentication rules for that endpoint.
- Return inconsistent response/error shapes, leak secrets, or omit request ID propagation where required.
- Hand-roll certificate lifecycle or bypass the built-in TLS/ACME rules.

## CRITICAL - ALWAYS DO
- Keep the health/version endpoints and their authentication and disclosure rules aligned with PART 13.
- Use versioned, canonical API routes and the request/response, content-negotiation, validation, and middleware order in PART 14.
- Include every project-defined native and compatibility API route in IDEA.md's `### API routes` inventory.
- Keep OpenAPI/Swagger and GraphQL descriptions and schemas synchronized with the implemented API where enabled by the spec.
- Follow PART 15 for TLS listeners, certificate storage, ACME challenge selection, renewal, and failure behavior.

## KEY DECISIONS (pre-answered)
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Where are project-specific API routes declared? | IDEA.md `### API routes` inventory | PART 0, 14 |
| How are public errors represented? | Shared API error contract defined by PART 14 | PART 14 |
| Which endpoints may be unauthenticated? | Only those explicitly designated public by PARTs 13–15 and the project spec | PART 13, 14 |
| How are certificates managed? | Built-in TLS/ACME implementation per PART 15 | PART 15 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| Native route | Protocol-defined path such as DNS, DoH, RDAP, or compatibility endpoints |
| API version | Canonical version segment used by the applicable REST routes |
| ACME challenge | Certificate domain-control validation method chosen under PART 15 |

## QUICK REFERENCE
- Trace route registration, auth, validation, and response shape to PART 14 plus IDEA.md.
- Health/version behavior is specified in PART 13; TLS and Let's Encrypt behavior in PART 15.
- Localization applies to human-readable API/schema/error text (PART 31).

---
For complete details, see AI.md PART 13, 14, 15
