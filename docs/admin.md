# Admin Panel

## Access

The admin panel is isolated under `/server/{admin_path}` — the path
segment defaults to `administration` but is fully configurable in
`server.yml`.

On first run, redxt serves a setup wizard at `/server/{admin_path}/setup`
and prints a one-time setup token to the console/log. Completing the
wizard creates the **Primary Admin** account, which cannot be deleted.

Sign-in for admins and regular users happens through the server-rendered
login form at `/server/auth/login` (`GET` renders the form, `POST`
authenticates and starts a session).

## Available pages

The current admin panel provides the first-run setup wizard, dashboard,
application name and tagline settings, server information, theme choice,
per-admin appearance preferences, and logout. The preference page is
`/server/{admin_path}/{admin_username}/preferences`; it stores theme,
font size, reduced motion, date format, and time format in the administrator's
own preferences. Pages require an authenticated Server Admin session; a
Regular User is redirected to their own account area. An administrator can
only read or update their own preference page.

Guest appearance and language preferences are separate from the admin
panel and live at `/server/preferences`. This path is reserved and cannot
be configured as the admin path.

The wider settings, audit, security, backup, update, cluster, and agent
management surfaces are not currently exposed by the admin panel. The
browser UI is server-rendered and its forms work without JavaScript.
