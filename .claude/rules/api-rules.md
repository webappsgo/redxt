# API Rules (PART 13, 14, 15)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Let REST, Swagger, and GraphQL drift out of sync — all three describe the same API surface
- Skip API versioning (`/api/{api_version}/...`)
- Hand-roll SSL cert management — use the built-in Let's Encrypt integration
- Return inconsistent error/response shapes across endpoints

## CRITICAL - ALWAYS DO
- Expose REST, Swagger UI (`/server/docs/swagger`, JSON at `/api/{api_version}/server/swagger`, alias `/api/swagger`), and GraphiQL (`/server/docs/graphql`, POST at `/api/{api_version}/server/graphql`, alias `/api/graphql`)
- Use a unified response format across all three API surfaces
- Auto-enable Let's Encrypt HTTP-01 (port 80) / TLS-ALPN-01 (port 443)
- Provide health check endpoints (`/server/healthz`) that don't require auth

## KEY DECISIONS (pre-answered)
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| API surfaces | REST + Swagger + GraphQL, all in sync | PART 14 |
| Health endpoint | `/server/healthz` | PART 13 |
| SSL | Built-in Let's Encrypt, auto on 443 | PART 15 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| api_version | Versioned API path segment, e.g. `v1` |

## QUICK REFERENCE
- Trailing-slash 301 normalization applies to doc/API paths
- Content negotiation supported across REST responses

---
For complete details, see AI.md PART 13, 14, 15
