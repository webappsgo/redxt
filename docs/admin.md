# Admin Panel

## Access

The admin panel is isolated under `/server/{admin_path}` — the path
segment defaults to `administration` but is fully configurable in
`server.yml`.

On first run, redxt serves a setup wizard at `/server/{admin_path}/setup`
and prints a one-time setup token to the console. It also creates the
operator API token (`server.token`) and prints that credential once. The
operator token is stored in `secrets.yml`, never in `server.yml` or the
database. Completing the wizard creates the **Primary Admin** account, which
cannot be deleted.
The setup form defaults the username to `steward`; operators may choose
another name that passes the same checks.
Admin usernames must pass the shared reserved-name and route-collision
checks. The additional blocklist used for Regular User usernames does not
apply to Server Admins; a reserved or conflicting setup name receives the
same generic conflict response as another name collision.

Sign-in for admins and regular users happens through the server-rendered
login form at `/server/auth/login` (`GET` renders the form, `POST`
authenticates and starts a session).

## Available pages

The current admin panel provides the first-run setup wizard, dashboard,
application name and tagline settings, editable SSL/TLS and ACME settings at
`/server/{admin_path}/ssl`, server information, theme choice, per-admin
appearance preferences, and logout. The SSL/TLS form includes the complete
lego DNS provider list and a free-form `VARIABLE=value` credential editor;
credential values are never returned to the page. Operators can set certificate
and private-key paths or upload a matching PEM pair; uploaded files are stored
under the configured SSL directory with private-key access restricted to the
server owner. Listener, certificate, private-key, and TLS version changes require
a server restart. The preference page is
`/server/{admin_path}/{admin_username}/preferences`; it stores theme,
font size, reduced motion, date format, and time format in the administrator's
own preferences. Pages require an authenticated Server Admin session; a
Regular User is redirected to their own account area. An administrator can
only read or update their own preference page.

The bearer-authenticated SSL API is available at
`/api/{api_version}/server/{admin_path}/ssl` (`GET` and `PATCH`) and
`/api/{api_version}/server/{admin_path}/ssl/renew` (`POST`). It accepts the
operator token or a usable stored Server Admin API token in the
`Authorization: Bearer` header. DNS credential values can be updated through
the API but are never included in its responses; responses include configured
credential variable names only.

An admin can inspect the masked default API token and replace it at
`/api/{api_version}/server/{admin_path}/{admin_username}/profile/token`.
`GET` returns only the token prefix and a mask. `POST` returns a newly
generated `adm_` token once and invalidates the previous default token. These
profile routes require a stored token belonging to the named admin; the
operator token cannot read or rotate an admin's personal token.

Guest appearance and language preferences are separate from the admin
panel and live at `/server/preferences`. This path is reserved and cannot
be configured as the admin path.

The backup page supports manual creation, listing, archive verification,
deletion with incremental-base protection, and archive downloads. Backup
restore and policy editing remain unavailable in the panel. The wider settings, audit,
security, update, cluster, and agent management surfaces also remain
incomplete. The browser UI is server-rendered and its forms work without
JavaScript.
