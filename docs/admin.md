# Admin Panel

## Access

The admin panel is isolated under `/server/{admin_path}` — the path
segment defaults to `administration` but is fully configurable in
`server.yml`.

On first run, redxt serves a setup wizard at `{admin_path}/config/setup`
and prints a one-time setup token to the console/log. Completing the
wizard creates the **Primary Admin** account, which cannot be deleted.

Sign-in for admins and regular users happens through the server-rendered
login form at `/server/auth/login` (`GET` renders the form, `POST`
authenticates and starts a session).

## Features

- Full settings surface — every configurable value in `server.yml` is
  editable from the admin panel, with no feature gated behind a paywall
- Server-side rendered Go templates only — no client-side framework;
  every admin feature works with JavaScript disabled
- Mobile-first responsive layout (tablet 768px, desktop 1024px
  breakpoints) with light/dark/auto theme
- Separate account tables for **Server Admins** (manage the app) and
  **Regular Users** (end-users) — the two are never merged

## Admin API

Administrative actions are also reachable through the versioned REST
API (`/api/v1/...`), sharing the same authentication/session model as
the HTML admin panel and the same unified response shape described in
[API Reference](api.md).
