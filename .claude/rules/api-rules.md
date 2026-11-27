# API Rules (PART 13, 14, 15)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Never create an undocumented route or a separate hand-maintained schema.
- Never let unauthenticated cookies authorize API requests.
- Never treat successful compilation as API or TLS conformance.

## CRITICAL - ALWAYS DO
- Generate REST, OpenAPI, GraphQL, and MCP operation metadata from shared definitions.
- Use versioned API routes and direct aliases where specified.
- Validate server-side and return canonical response/error envelopes.
- Resolve and enforce SSL/TLS settings and certificate lifecycle requirements.

## KEY DECISIONS
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| API version | The configured `{api_version}` | PART 13, 14 |
| API default response | JSON, with documented negotiated text forms | PART 14 |
| TLS challenge | Use the configured supported challenge and provider | PART 15 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| Operation | One shared route declaration feeding API docs and dispatch |
| API alias | Direct same-handler route, never a redirect |

## QUICK REFERENCE
Honor content negotiation, route scopes, auth/scope policy, and the TLS fallback order.

---
For complete details, see AI.md PART 13, 14, 15
