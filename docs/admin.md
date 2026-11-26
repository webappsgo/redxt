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
application name and tagline settings, a read-only SSL/TLS configuration
summary at `/server/{admin_path}/ssl`, server information, theme choice,
per-admin appearance preferences, and logout. The SSL/TLS summary shows
configured certificate paths and ACME options, but never reads or displays
certificate or private-key contents. SSL/TLS editing and renewal actions are
not available in the panel yet. The preference page is
`/server/{admin_path}/{admin_username}/preferences`; it stores theme,
font size, reduced motion, date format, and time format in the administrator's
own preferences. Pages require an authenticated Server Admin session; a
Regular User is redirected to their own account area. An administrator can
only read or update their own preference page.

Guest appearance and language preferences are separate from the admin
panel and live at `/server/preferences`. This path is reserved and cannot
be configured as the admin path.

The backup page supports manual creation, listing, archive verification,
deletion with incremental-base protection, and archive downloads. Backup
restore and policy editing remain unavailable in the panel. The wider settings, audit,
security, update, cluster, and agent management surfaces also remain
incomplete. The browser UI is server-rendered and its forms work without
JavaScript.
