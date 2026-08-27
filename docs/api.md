# API Reference

## REST API

All API routes are versioned under `/api/{api_version}/...` (default
`v1`). Every response — REST, Swagger, and GraphQL — shares one unified
response and error shape, and all three surfaces describe the same API
and stay in sync.

### Endpoints

- `GET /server/healthz` — unauthenticated health check (root alias:
  `GET /healthz`)
- `GET /api/v1/server/healthz` — versioned health check, mirrors
  `/server/healthz`

Content negotiation is honored on every route (`Accept:
application/json`, `text/plain`, `text/html` where applicable).

## Swagger UI

- UI: `/server/docs/swagger`
- JSON: `/api/v1/server/swagger` (alias: `/api/swagger`)

## GraphQL

- GraphiQL UI: `/server/docs/graphql`
- Endpoint: `POST /api/v1/server/graphql` (alias: `POST /api/graphql`)

### Schema

The GraphQL schema mirrors the REST surface one-for-one; any endpoint
reachable over REST is reachable through the same operation in
GraphQL, and vice versa.
