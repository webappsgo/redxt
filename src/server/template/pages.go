package template

// pages holds every server-rendered document.
//
// The markup is kept in one place so a change to the shared chrome
// cannot be applied to some pages and forgotten on others. Styling uses
// custom properties and honors prefers-color-scheme, per PART 16, and
// long values wrap rather than pushing a phone layout sideways.
const pages = `
{{define "head"}}<!DOCTYPE html>
<html lang="{{.Language}}">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}} - {{.AppName}}</title>
  <style>
:root {
  color-scheme: light dark;
  --bg: #f8f8f2;
  --fg: #282a36;
  --muted: #6272a4;
  --accent: #6272a4;
  --line: #d5d7e0;
  --bad: #a6203c;
}
@media (prefers-color-scheme: dark) {
  :root {
    --bg: #282a36;
    --fg: #f8f8f2;
    --muted: #a4b1d6;
    --accent: #8be9fd;
    --line: #44475a;
    --bad: #ff79a8;
  }
}
body {
  background: var(--bg);
  color: var(--fg);
  font-family: system-ui, sans-serif;
  line-height: 1.5;
  margin: 0 auto;
  max-width: 48rem;
  overflow-wrap: anywhere;
  padding: 1.5rem;
  word-break: break-word;
}
header nav a {
  display: inline-block;
  margin-right: 1rem;
}
h1 { font-size: 1.5rem; }
h2 { font-size: 1.2rem; margin-top: 2rem; }
a { color: var(--accent); }
label { display: block; margin-top: 0.75rem; }
input, select, textarea {
  background: var(--bg);
  border: 1px solid var(--line);
  border-radius: 0.25rem;
  color: var(--fg);
  font: inherit;
  max-width: 100%;
  padding: 0.4rem;
  width: 100%;
}
input[type="checkbox"] { width: auto; }
button {
  background: var(--accent);
  border: 0;
  border-radius: 0.25rem;
  color: var(--bg);
  cursor: pointer;
  font: inherit;
  margin-top: 1rem;
  padding: 0.5rem 1rem;
}
table { border-collapse: collapse; width: 100%; }
td, th { border-bottom: 1px solid var(--line); padding: 0.35rem 0.5rem; text-align: left; }
.notice { border-left: 3px solid var(--accent); padding: 0.5rem 0.75rem; }
.error { border-left: 3px solid var(--bad); color: var(--bad); padding: 0.5rem 0.75rem; }
.muted { color: var(--muted); }
@media (min-width: 768px) {
  body { padding: 2rem; }
}
  </style>
</head>
<body>
<header>
  <h1><a href="{{.Base}}/">{{.AppName}}</a></h1>
  <nav>
  {{if .Viewer}}
    <a href="{{.Base}}/users/account">{{.Viewer.Username}}</a>
    <a href="{{.Base}}/users/settings">Settings</a>
    <a href="{{.Base}}/users/security">Security</a>
    <a href="{{.Base}}/orgs">Organizations</a>
    <form method="post" action="{{.Base}}/server/auth/logout">
      <input type="hidden" name="csrf_token" value="{{.CSRF}}">
      <button type="submit">Sign out</button>
    </form>
  {{else}}
    <a href="{{.Base}}/server/auth/login">Sign in</a>
    <a href="{{.Base}}/server/auth/register">Register</a>
  {{end}}
  </nav>
</header>
<main>
{{if .Notice}}<p class="notice">{{.Notice}}</p>{{end}}
{{if .Error}}<p class="error">{{.Error}}</p>{{end}}
{{end}}

{{define "foot"}}</main>
</body>
</html>
{{end}}

{{define "login"}}{{template "head" .}}
<h2>Sign in</h2>
<form method="post" action="{{.Base}}/server/auth/login">
  <input type="hidden" name="csrf_token" value="{{.CSRF}}">
  <label>Username or email
    <input type="text" name="identifier" autocomplete="username" required>
  </label>
  <label>Password
    <input type="password" name="password" autocomplete="current-password" required>
  </label>
  <button type="submit">Sign in</button>
</form>
<p><a href="{{.Base}}/server/auth/password/forgot">Forgot your password?</a></p>
{{if .Data.RegistrationOpen}}<p><a href="{{.Base}}/server/auth/register">Create an account</a></p>{{end}}
{{template "foot" .}}{{end}}

{{define "register"}}{{template "head" .}}
<h2>Create an account</h2>
{{if .Data.Closed}}
<p>Registration is closed on this server.</p>
{{else}}
<form method="post" action="{{.Base}}/server/auth/register">
  <input type="hidden" name="csrf_token" value="{{.CSRF}}">
  <label>Username
    <input type="text" name="username" autocomplete="username" required>
  </label>
  <label>Email
    <input type="email" name="email" autocomplete="email" required>
  </label>
  <label>Password
    <input type="password" name="password" autocomplete="new-password" required>
  </label>
  {{if .Data.InviteRequired}}
  <label>Invite code
    <input type="text" name="invite" value="{{.Data.Code}}" required>
  </label>
  {{end}}
  <button type="submit">Create account</button>
</form>
{{end}}
<p><a href="{{.Base}}/server/auth/login">Already have an account?</a></p>
{{template "foot" .}}{{end}}

{{define "twofactor"}}{{template "head" .}}
<h2>Two-factor code</h2>
<form method="post" action="{{.Base}}/server/auth/2fa">
  <input type="hidden" name="csrf_token" value="{{.CSRF}}">
  <label>Authentication code
    <input type="text" name="code" inputmode="numeric" autocomplete="one-time-code" required>
  </label>
  <button type="submit">Verify</button>
</form>
<p class="muted">A recovery code works here too.</p>
{{template "foot" .}}{{end}}

{{define "forgot"}}{{template "head" .}}
<h2>Reset your password</h2>
<form method="post" action="{{.Base}}/server/auth/password/forgot">
  <input type="hidden" name="csrf_token" value="{{.CSRF}}">
  <label>Username or email
    <input type="text" name="identifier" autocomplete="username" required>
  </label>
  <button type="submit">Send reset link</button>
</form>
{{template "foot" .}}{{end}}

{{define "reset"}}{{template "head" .}}
<h2>Choose a new password</h2>
<form method="post" action="{{.Base}}/server/auth/password/reset">
  <input type="hidden" name="csrf_token" value="{{.CSRF}}">
  <input type="hidden" name="token" value="{{.Data.Token}}">
  <label>New password
    <input type="password" name="password" autocomplete="new-password" required>
  </label>
  <button type="submit">Set password</button>
</form>
{{template "foot" .}}{{end}}

{{define "message"}}{{template "head" .}}
<h2>{{.Title}}</h2>
{{if .Data.Body}}<p>{{.Data.Body}}</p>{{end}}
{{if .Data.LinkURL}}<p><a href="{{.Data.LinkURL}}">{{.Data.LinkText}}</a></p>{{end}}
{{template "foot" .}}{{end}}

{{define "account"}}{{template "head" .}}
<h2>Profile</h2>
<form method="post" action="{{.Base}}/users/account">
  <input type="hidden" name="csrf_token" value="{{.CSRF}}">
  <label>Display name
    <input type="text" name="display_name" value="{{.Data.User.DisplayName}}">
  </label>
  <label>Bio
    <textarea name="bio" rows="3">{{.Data.User.Bio}}</textarea>
  </label>
  <label>Location
    <input type="text" name="location" value="{{.Data.User.Location}}">
  </label>
  <label>Website
    <input type="url" name="website" value="{{.Data.User.Website}}">
  </label>
  <label>Avatar URL
    <input type="url" name="avatar_url" value="{{.Data.User.AvatarURL}}">
  </label>
  <label>Timezone
    <input type="text" name="timezone" value="{{.Data.User.Timezone}}">
  </label>
  <label>Language
    <input type="text" name="language" value="{{.Data.User.Language}}">
  </label>
  <label>Profile visibility
    <select name="visibility">
      <option value="public" {{if eq .Data.User.Visibility "public"}}selected{{end}}>Public</option>
      <option value="private" {{if eq .Data.User.Visibility "private"}}selected{{end}}>Private</option>
    </select>
  </label>
  <label><input type="checkbox" name="org_visibility" value="1" {{if .Data.User.OrgVisibility}}checked{{end}}> Visible to people I share an organization with</label>
  <button type="submit">Save profile</button>
</form>
<p class="muted">Public profile: <a href="{{.Base}}/users/{{.Data.User.Username}}">{{.Base}}/users/{{.Data.User.Username}}</a></p>
{{template "foot" .}}{{end}}

{{define "settings"}}{{template "head" .}}
<h2>Settings</h2>
<form method="post" action="{{.Base}}/users/settings">
  <input type="hidden" name="csrf_token" value="{{.CSRF}}">
  <h3>Privacy</h3>
  <label><input type="checkbox" name="show_email" value="1" {{if .Data.ShowEmail}}checked{{end}}> Show my email on my profile</label>
  <label><input type="checkbox" name="show_activity" value="1" {{if .Data.ShowActivity}}checked{{end}}> Show my activity</label>
  <label><input type="checkbox" name="show_orgs" value="1" {{if .Data.ShowOrgs}}checked{{end}}> Show my organizations</label>
  <label><input type="checkbox" name="searchable" value="1" {{if .Data.Searchable}}checked{{end}}> Allow my profile to be found by search</label>
  <h3>Email</h3>
  <label><input type="checkbox" name="email_security" value="1" {{if .Data.EmailSecurity}}checked{{end}}> Security notifications</label>
  <label><input type="checkbox" name="email_org" value="1" {{if .Data.EmailOrg}}checked{{end}}> Organization notifications</label>
  <label><input type="checkbox" name="email_product" value="1" {{if .Data.EmailProduct}}checked{{end}}> Product announcements</label>
  <h3>Display</h3>
  <label>Theme
    <select name="theme">
      <option value="auto" {{if eq .Data.Theme "auto"}}selected{{end}}>Auto</option>
      <option value="light" {{if eq .Data.Theme "light"}}selected{{end}}>Light</option>
      <option value="dark" {{if eq .Data.Theme "dark"}}selected{{end}}>Dark</option>
    </select>
  </label>
  <label>Font size
    <input type="text" name="font_size" value="{{.Data.FontSize}}">
  </label>
  <label><input type="checkbox" name="reduce_motion" value="1" {{if .Data.ReduceMotion}}checked{{end}}> Reduce motion</label>
  <label>Date format
    <input type="text" name="date_format" value="{{.Data.DateFormat}}">
  </label>
  <label>Time format
    <input type="text" name="time_format" value="{{.Data.TimeFormat}}">
  </label>
  <button type="submit">Save settings</button>
</form>
{{template "foot" .}}{{end}}

{{define "security"}}{{template "head" .}}
<h2>Security</h2>
<p>Email: {{.Data.Email}}{{if not .Data.EmailVerified}} <span class="muted">(unverified)</span>{{end}}</p>

<h3>Password</h3>
<form method="post" action="{{.Base}}/users/security/password">
  <input type="hidden" name="csrf_token" value="{{.CSRF}}">
  <label>Current password
    <input type="password" name="current_password" autocomplete="current-password" required>
  </label>
  <label>New password
    <input type="password" name="new_password" autocomplete="new-password" required>
  </label>
  <button type="submit">Change password</button>
</form>

<h3>Two-factor authentication</h3>
{{if .Data.TwoFactorEnabled}}
<form method="post" action="{{.Base}}/users/security/2fa/disable">
  <input type="hidden" name="csrf_token" value="{{.CSRF}}">
  <label>Password
    <input type="password" name="password" autocomplete="current-password" required>
  </label>
  <button type="submit">Disable two-factor</button>
</form>
{{else}}
<form method="post" action="{{.Base}}/users/security/2fa">
  <input type="hidden" name="csrf_token" value="{{.CSRF}}">
  <button type="submit">Set up two-factor</button>
</form>
{{end}}

<h3>Sessions</h3>
<table>
  <tr><th>Address</th><th>Client</th><th>Last active</th><th></th></tr>
  {{range .Data.Sessions}}
  <tr>
    <td>{{.IP}}</td>
    <td>{{.UserAgent}}</td>
    <td>{{.LastActive}}</td>
    <td>
      <form method="post" action="{{$.Base}}/users/sessions/{{.ID}}/revoke">
        <input type="hidden" name="csrf_token" value="{{$.CSRF}}">
        <button type="submit">Revoke</button>
      </form>
    </td>
  </tr>
  {{end}}
</table>

<h3>API tokens</h3>
<table>
  <tr><th>Name</th><th>Prefix</th><th>Scope</th><th>Role</th><th></th></tr>
  {{range .Data.Tokens}}
  <tr>
    <td>{{.Name}}</td>
    <td>{{.Prefix}}</td>
    <td>{{.Scope}}</td>
    <td>{{.Role}}</td>
    <td>
      <form method="post" action="{{$.Base}}/users/tokens/{{.ID}}/revoke">
        <input type="hidden" name="csrf_token" value="{{$.CSRF}}">
        <button type="submit">Revoke</button>
      </form>
    </td>
  </tr>
  {{end}}
</table>
<form method="post" action="{{.Base}}/users/tokens">
  <input type="hidden" name="csrf_token" value="{{.CSRF}}">
  <label>Token name
    <input type="text" name="name" required>
  </label>
  <label>Organization
    <select name="org">
      {{range .Data.Orgs}}<option value="{{.Slug}}">{{.Name}}</option>{{end}}
    </select>
  </label>
  <label>Role
    <select name="role">
      <option value="viewer">Viewer</option>
      <option value="editor">Editor</option>
      <option value="admin">Admin</option>
    </select>
  </label>
  <label>Capability (optional)
    <input type="text" name="capability" placeholder="records:write">
  </label>
  <button type="submit">Create token</button>
</form>
{{template "foot" .}}{{end}}

{{define "secret"}}{{template "head" .}}
<h2>{{.Title}}</h2>
<p>Copy this value now. It is shown once and cannot be recovered.</p>
<p><code>{{.Data.Secret}}</code></p>
{{if .Data.Extra}}
<h3>{{.Data.ExtraTitle}}</h3>
<ul>{{range .Data.Extra}}<li><code>{{.}}</code></li>{{end}}</ul>
{{end}}
{{if .Data.ConfirmURL}}
<form method="post" action="{{.Data.ConfirmURL}}">
  <input type="hidden" name="csrf_token" value="{{.CSRF}}">
  <label>Authentication code
    <input type="text" name="code" inputmode="numeric" required>
  </label>
  <button type="submit">Confirm</button>
</form>
{{end}}
<p><a href="{{.Data.LinkURL}}">{{.Data.LinkText}}</a></p>
{{template "foot" .}}{{end}}

{{define "orgs"}}{{template "head" .}}
<h2>Organizations</h2>
<table>
  <tr><th>Name</th><th>Slug</th><th>Kind</th></tr>
  {{range .Data.Orgs}}
  <tr>
    <td><a href="{{$.Base}}/orgs/{{.Slug}}">{{.Name}}</a></td>
    <td>{{.Slug}}</td>
    <td>{{if .Personal}}Personal{{else}}Shared{{end}}</td>
  </tr>
  {{end}}
</table>
{{if .Data.CanCreate}}
<h3>Create an organization</h3>
<form method="post" action="{{.Base}}/orgs">
  <input type="hidden" name="csrf_token" value="{{.CSRF}}">
  <label>Name
    <input type="text" name="name" required>
  </label>
  <label>Slug
    <input type="text" name="slug" required>
  </label>
  <label>Description
    <input type="text" name="description">
  </label>
  {{if .Data.InviteRequired}}
  <label>Invite code
    <input type="text" name="invite" required>
  </label>
  {{end}}
  <button type="submit">Create</button>
</form>
{{end}}
{{template "foot" .}}{{end}}

{{define "org"}}{{template "head" .}}
<h2>{{.Data.Org.Name}}</h2>
<p class="muted">Your role: {{.Data.Role}}</p>

{{if .Data.CanSettings}}
<h3>Settings</h3>
<form method="post" action="{{.Base}}/orgs/{{.Data.Org.Slug}}/settings">
  <input type="hidden" name="csrf_token" value="{{.CSRF}}">
  <label>Name
    <input type="text" name="name" value="{{.Data.Org.Name}}">
  </label>
  <label>Description
    <input type="text" name="description" value="{{.Data.Org.Description}}">
  </label>
  <label>Website
    <input type="url" name="website" value="{{.Data.Org.Website}}">
  </label>
  <label>Location
    <input type="text" name="location" value="{{.Data.Org.Location}}">
  </label>
  <label>Visibility
    <select name="visibility">
      <option value="public" {{if eq .Data.Org.Visibility "public"}}selected{{end}}>Public</option>
      <option value="private" {{if eq .Data.Org.Visibility "private"}}selected{{end}}>Private</option>
    </select>
  </label>
  <button type="submit">Save</button>
</form>
{{end}}

<h3>Members</h3>
<table>
  <tr><th>User</th><th>Role</th>{{if .Data.CanManageMembers}}<th></th>{{end}}</tr>
  {{range .Data.Members}}
  <tr>
    <td>{{.Username}}</td>
    <td>{{.Role}}</td>
    {{if $.Data.CanManageMembers}}
    <td>
      <form method="post" action="{{$.Base}}/orgs/{{$.Data.Org.Slug}}/members/{{.UserID}}">
        <input type="hidden" name="csrf_token" value="{{$.CSRF}}">
        <select name="role">
          <option value="viewer" {{if eq .Role "viewer"}}selected{{end}}>Viewer</option>
          <option value="editor" {{if eq .Role "editor"}}selected{{end}}>Editor</option>
          <option value="admin" {{if eq .Role "admin"}}selected{{end}}>Admin</option>
          <option value="owner" {{if eq .Role "owner"}}selected{{end}}>Owner</option>
        </select>
        <button type="submit">Set role</button>
      </form>
      <form method="post" action="{{$.Base}}/orgs/{{$.Data.Org.Slug}}/members/{{.UserID}}/remove">
        <input type="hidden" name="csrf_token" value="{{$.CSRF}}">
        <button type="submit">Remove</button>
      </form>
    </td>
    {{end}}
  </tr>
  {{end}}
</table>

{{if .Data.CanManageMembers}}
<h3>Invitations</h3>
<table>
  <tr><th>Email</th><th>Role</th><th>Uses</th><th>Expires</th><th></th></tr>
  {{range .Data.Invites}}
  <tr>
    <td>{{.Email}}</td>
    <td>{{.Role}}</td>
    <td>{{.UseCount}}/{{.MaxUses}}</td>
    <td>{{.ExpiresAt}}</td>
    <td>
      <form method="post" action="{{$.Base}}/orgs/{{$.Data.Org.Slug}}/invites/{{.ID}}/revoke">
        <input type="hidden" name="csrf_token" value="{{$.CSRF}}">
        <button type="submit">Revoke</button>
      </form>
    </td>
  </tr>
  {{end}}
</table>
<form method="post" action="{{.Base}}/orgs/{{.Data.Org.Slug}}/invites">
  <input type="hidden" name="csrf_token" value="{{.CSRF}}">
  <label>Email
    <input type="email" name="email">
  </label>
  <label>Role
    <select name="role">
      <option value="viewer">Viewer</option>
      <option value="editor">Editor</option>
      <option value="admin">Admin</option>
    </select>
  </label>
  <label>Maximum uses
    <input type="number" name="max_uses" value="1" min="0">
  </label>
  <button type="submit">Invite</button>
</form>
{{end}}

<h3>Custom domains</h3>
<table>
  <tr><th>Domain</th><th>Purpose</th><th>Verification</th><th>Certificate</th><th></th></tr>
  {{range .Data.Domains}}
  <tr>
    <td>{{.Domain}}</td>
    <td>{{.Purpose}}</td>
    <td>{{.VerificationStatus}}</td>
    <td>{{.SSLStatus}}</td>
    <td>
      <a href="{{$.Base}}/orgs/{{$.Data.Org.Slug}}/domains/{{.ID}}">Details</a>
    </td>
  </tr>
  {{end}}
</table>
{{if .Data.CanSettings}}
<form method="post" action="{{.Base}}/orgs/{{.Data.Org.Slug}}/domains">
  <input type="hidden" name="csrf_token" value="{{.CSRF}}">
  <label>Domain
    <input type="text" name="domain" required>
  </label>
  <label>Purpose
    <select name="purpose">
      <option value="ui">Web interface</option>
      <option value="ddns">Dynamic DNS</option>
      <option value="redirect">Redirector</option>
      <option value="parking">Parking</option>
      <option value="gateway">Data gateway</option>
    </select>
  </label>
  <button type="submit">Add domain</button>
</form>
{{end}}
<p><a href="{{.Base}}/orgs/{{.Data.Org.Slug}}/audit">Audit log</a></p>
{{template "foot" .}}{{end}}

{{define "domain"}}{{template "head" .}}
<h2>{{.Data.Domain.Domain}}</h2>
<p class="muted">Verification: {{.Data.Domain.VerificationStatus}} - Certificate: {{.Data.Domain.SSLStatus}}</p>
{{if .Data.Record.Value}}
<h3>Prove ownership</h3>
<p>Publish this record, then check it:</p>
<table>
  <tr><th>Name</th><td>{{.Data.Record.Name}}</td></tr>
  <tr><th>Type</th><td>{{.Data.Record.Type}}</td></tr>
  <tr><th>Value</th><td>{{.Data.Record.Value}}</td></tr>
</table>
<form method="post" action="{{.Base}}/orgs/{{.Data.Slug}}/domains/{{.Data.Domain.ID}}/verify">
  <input type="hidden" name="csrf_token" value="{{.CSRF}}">
  <button type="submit">Check now</button>
</form>
{{end}}
<h3>Purpose</h3>
<form method="post" action="{{.Base}}/orgs/{{.Data.Slug}}/domains/{{.Data.Domain.ID}}">
  <input type="hidden" name="csrf_token" value="{{.CSRF}}">
  <label>Purpose
    <select name="purpose">
      <option value="ui" {{if eq .Data.Domain.Purpose "ui"}}selected{{end}}>Web interface</option>
      <option value="ddns" {{if eq .Data.Domain.Purpose "ddns"}}selected{{end}}>Dynamic DNS</option>
      <option value="redirect" {{if eq .Data.Domain.Purpose "redirect"}}selected{{end}}>Redirector</option>
      <option value="parking" {{if eq .Data.Domain.Purpose "parking"}}selected{{end}}>Parking</option>
      <option value="gateway" {{if eq .Data.Domain.Purpose "gateway"}}selected{{end}}>Data gateway</option>
    </select>
  </label>
  <button type="submit">Save</button>
</form>
<form method="post" action="{{.Base}}/orgs/{{.Data.Slug}}/domains/{{.Data.Domain.ID}}/remove">
  <input type="hidden" name="csrf_token" value="{{.CSRF}}">
  <button type="submit">Remove domain</button>
</form>
<p><a href="{{.Base}}/orgs/{{.Data.Slug}}">Back to organization</a></p>
{{template "foot" .}}{{end}}

{{define "audit"}}{{template "head" .}}
<h2>Audit log</h2>
<table>
  <tr><th>When</th><th>Event</th><th>Actor</th><th>Target</th></tr>
  {{range .Data.Entries}}
  <tr>
    <td>{{.CreatedAt}}</td>
    <td>{{.Event}}</td>
    <td>{{.ActorType}} {{.ActorID}}</td>
    <td>{{.TargetID}}</td>
  </tr>
  {{end}}
</table>
<p><a href="{{.Base}}/orgs/{{.Data.Slug}}">Back to organization</a></p>
{{template "foot" .}}{{end}}

{{define "profile"}}{{template "head" .}}
<h2>{{if .Data.DisplayName}}{{.Data.DisplayName}}{{else}}{{.Data.Username}}{{end}}</h2>
<p class="muted">{{.Data.Username}}</p>
{{if .Data.Bio}}<p>{{.Data.Bio}}</p>{{end}}
{{if .Data.Location}}<p>{{.Data.Location}}</p>{{end}}
{{if .Data.Website}}<p><a href="{{.Data.Website}}" rel="nofollow noopener">{{.Data.Website}}</a></p>{{end}}
{{if .Data.Email}}<p>{{.Data.Email}}</p>{{end}}
{{if .Data.Orgs}}
<h3>Organizations</h3>
<ul>{{range .Data.Orgs}}<li><a href="{{$.Base}}/orgs/{{.Slug}}">{{.Name}}</a></li>{{end}}</ul>
{{end}}
{{template "foot" .}}{{end}}

{{define "orgprofile"}}{{template "head" .}}
<h2>{{.Data.Name}}</h2>
<p class="muted">{{.Data.Slug}} - {{.Data.MemberCount}} members</p>
{{if .Data.Description}}<p>{{.Data.Description}}</p>{{end}}
{{if .Data.Location}}<p>{{.Data.Location}}</p>{{end}}
{{if .Data.Website}}<p><a href="{{.Data.Website}}" rel="nofollow noopener">{{.Data.Website}}</a></p>{{end}}
{{template "foot" .}}{{end}}
`
