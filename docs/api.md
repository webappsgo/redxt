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

## DNS-over-HTTPS

The native DoH endpoint is served at `/dns-query` over HTTPS and uses the
same authoritative answer engine as the UDP/TCP DNS listener. A trusted
reverse proxy may terminate TLS and report the effective scheme through the
trusted-proxy configuration.

- `GET /dns-query?dns={base64url-message}` — the `dns` value is unpadded
  base64url-encoded DNS wire data.
- `POST /dns-query` — send DNS wire data with
  `Content-Type: application/dns-message`.

Successful DNS exchanges return `application/dns-message`; a DNS-level error
such as REFUSED is carried in the DNS response rather than converted to an
HTTP error. Recursive resolution and other encrypted transports are not
implemented yet.

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
