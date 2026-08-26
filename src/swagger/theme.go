// Package swagger — theming for the Swagger UI page, required by the
// AI.md PART 14 rule that the API documentation UIs match the
// project-wide light/dark/auto theme system.
//
// Every colour lives in the custom-property token block at the top of
// the stylesheet. Nothing below that block names a colour literal, so
// switching a palette is a change in one place. The dark palette is the
// default; the light palette applies when the root element carries
// theme-light, and auto mode is pure CSS through prefers-color-scheme
// with no script involved.

package swagger

// Theme preference values accepted by Options.Theme.
const (
	// ThemeAuto follows the operating system preference through the
	// prefers-color-scheme media query.
	ThemeAuto = "auto"
	// ThemeDark forces the dark palette.
	ThemeDark = "dark"
	// ThemeLight forces the light palette.
	ThemeLight = "light"
)

// ThemeClass returns the root element class for a theme preference.
// Auto, and any unrecognised value, resolves to theme-auto, which lets
// the media query decide.
func ThemeClass(pref string) string {
	switch pref {
	case ThemeDark:
		return "theme-dark"
	case ThemeLight:
		return "theme-light"
	default:
		return "theme-auto"
	}
}

// CSS returns the complete inline stylesheet for the Swagger UI page.
//
// It is inlined into the page rather than served as a separate asset so
// the document stays self-contained: a strict Content-Security-Policy
// that forbids external hosts cannot break it, and no CDN is contacted.
func CSS() string {
	return themeTokens + swaggerCSS
}

// themeTokens is the only place in this package where a colour literal
// appears. The dark values are the project palette from AI.md PART 14;
// the light values are its light-theme counterpart.
const themeTokens = `
    :root {
      --dark-bg: #282a36;
      --dark-bg-alt: #1e1f29;
      --dark-bg-elevated: #44475a;
      --dark-text: #f8f8f2;
      --dark-text-muted: #6272a4;
      --dark-accent-cyan: #8be9fd;
      --dark-accent-green: #50fa7b;
      --dark-accent-orange: #ffb86c;
      --dark-accent-red: #ff5555;
      --dark-accent-purple: #bd93f9;
      --dark-accent-pink: #ff79c6;
      --dark-accent-yellow: #f1fa8c;

      --light-bg: #ffffff;
      --light-bg-alt: #f5f5f5;
      --light-bg-elevated: #e0e0e0;
      --light-text: #1a1a1a;
      --light-text-muted: #666666;
      --light-accent-blue: #0066cc;
      --light-accent-green: #008000;
      --light-accent-orange: #ff8c00;
      --light-accent-red: #cc0000;
      --light-accent-purple: #6600cc;
      --light-accent-teal: #008080;
    }

    /* Dark is the project default, so the semantic tokens start dark. */
    :root,
    .theme-auto,
    .theme-dark {
      --bg: var(--dark-bg);
      --bg-alt: var(--dark-bg-alt);
      --bg-elevated: var(--dark-bg-elevated);
      --text: var(--dark-text);
      --text-muted: var(--dark-text-muted);
      --border: var(--dark-text-muted);
      --accent: var(--dark-accent-purple);
      --accent-alt: var(--dark-accent-pink);
      --get: var(--dark-accent-cyan);
      --post: var(--dark-accent-green);
      --put: var(--dark-accent-orange);
      --patch: var(--dark-accent-yellow);
      --delete: var(--dark-accent-red);
      --btn-bg: var(--dark-bg-elevated);
      --btn-text: var(--dark-text);
      --tint: rgba(248, 248, 242, 0.06);
    }

    /* Auto mode follows the system preference with no JavaScript. */
    @media (prefers-color-scheme: light) {
      .theme-auto {
        --bg: var(--light-bg);
        --bg-alt: var(--light-bg-alt);
        --bg-elevated: var(--light-bg-elevated);
        --text: var(--light-text);
        --text-muted: var(--light-text-muted);
        --border: var(--light-bg-elevated);
        --accent: var(--light-accent-purple);
        --accent-alt: var(--light-accent-teal);
        --get: var(--light-accent-blue);
        --post: var(--light-accent-green);
        --put: var(--light-accent-orange);
        --patch: var(--light-accent-teal);
        --delete: var(--light-accent-red);
        --btn-bg: var(--light-accent-blue);
        --btn-text: var(--light-bg);
        --tint: rgba(26, 26, 26, 0.04);
      }
    }

    .theme-light {
      --bg: var(--light-bg);
      --bg-alt: var(--light-bg-alt);
      --bg-elevated: var(--light-bg-elevated);
      --text: var(--light-text);
      --text-muted: var(--light-text-muted);
      --border: var(--light-bg-elevated);
      --accent: var(--light-accent-purple);
      --accent-alt: var(--light-accent-teal);
      --get: var(--light-accent-blue);
      --post: var(--light-accent-green);
      --put: var(--light-accent-orange);
      --patch: var(--light-accent-teal);
      --delete: var(--light-accent-red);
      --btn-bg: var(--light-accent-blue);
      --btn-text: var(--light-bg);
      --tint: rgba(26, 26, 26, 0.04);
    }
`

// swaggerCSS is the mobile-first layout for the operation explorer. It
// names no colour of its own, only the tokens defined above.
const swaggerCSS = `
    * { box-sizing: border-box; }

    body.swagger-ui {
      margin: 0;
      padding: 0;
      background: var(--bg);
      color: var(--text);
      font-family: system-ui, -apple-system, "Segoe UI", Roboto, sans-serif;
      font-size: 16px;
      line-height: 1.5;
      overflow-wrap: anywhere;
      word-break: break-word;
    }

    .topbar {
      background: var(--bg-alt);
      border-bottom: 1px solid var(--border);
      padding: 1rem;
    }

    .topbar .title {
      margin: 0;
      font-size: 1.25rem;
      color: var(--text);
    }

    .topbar .version {
      margin: 0.25rem 0 0;
      color: var(--text-muted);
      font-size: 0.875rem;
    }

    .topbar .links {
      margin-top: 0.5rem;
      display: flex;
      flex-wrap: wrap;
      gap: 1rem;
    }

    .topbar .links a,
    a {
      color: var(--get);
      text-decoration: underline;
    }

    .content {
      padding: 1rem;
      max-width: 60rem;
      margin: 0 auto;
    }

    .intro {
      color: var(--text-muted);
    }

    .tag-name {
      font-size: 1.05rem;
      text-transform: uppercase;
      letter-spacing: 0.05em;
      color: var(--accent);
      border-bottom: 1px solid var(--border);
      padding-bottom: 0.35rem;
    }

    .opblock {
      border: 1px solid var(--border);
      border-left-width: 4px;
      border-radius: 6px;
      margin-bottom: 0.75rem;
      background: var(--tint);
    }

    .opblock-get { border-left-color: var(--get); }
    .opblock-post { border-left-color: var(--post); }
    .opblock-put { border-left-color: var(--put); }
    .opblock-patch { border-left-color: var(--patch); }
    .opblock-delete { border-left-color: var(--delete); }
    .opblock-head,
    .opblock-options { border-left-color: var(--text-muted); }

    .opblock-summary {
      cursor: pointer;
      padding: 0.65rem 0.75rem;
      display: flex;
      flex-wrap: wrap;
      align-items: baseline;
      gap: 0.5rem;
    }

    .opblock-summary:focus-visible {
      outline: 2px solid var(--accent);
      outline-offset: 2px;
    }

    .method {
      font-weight: 700;
      font-size: 0.75rem;
      letter-spacing: 0.05em;
      padding: 0.15rem 0.45rem;
      border-radius: 4px;
      border: 1px solid var(--border);
      color: var(--text);
    }

    .opblock-get .method { color: var(--get); border-color: var(--get); }
    .opblock-post .method { color: var(--post); border-color: var(--post); }
    .opblock-put .method { color: var(--put); border-color: var(--put); }
    .opblock-patch .method { color: var(--patch); border-color: var(--patch); }
    .opblock-delete .method { color: var(--delete); border-color: var(--delete); }

    .path {
      font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
      font-size: 0.9rem;
    }

    .summary-text {
      color: var(--text-muted);
      font-size: 0.875rem;
    }

    .opblock-body {
      padding: 0 0.75rem 0.75rem;
      border-top: 1px solid var(--border);
    }

    .section-title {
      font-size: 0.8rem;
      text-transform: uppercase;
      letter-spacing: 0.05em;
      color: var(--accent-alt);
      margin-bottom: 0.35rem;
    }

    .description { margin: 0.5rem 0; }

    .meta {
      display: grid;
      grid-template-columns: auto 1fr;
      gap: 0.25rem 0.75rem;
      margin: 0.5rem 0;
      font-size: 0.875rem;
    }

    .meta dt { color: var(--text-muted); }
    .meta dd { margin: 0; }

    code, pre {
      font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
      font-size: 0.85rem;
    }

    .code {
      background: var(--bg-alt);
      border: 1px solid var(--border);
      border-radius: 4px;
      padding: 0.6rem;
      overflow-x: auto;
      white-space: pre-wrap;
    }

    .table-scroll { overflow-x: auto; }

    table.params {
      width: 100%;
      border-collapse: collapse;
      font-size: 0.85rem;
    }

    table.params th,
    table.params td {
      text-align: left;
      padding: 0.35rem 0.5rem;
      border-bottom: 1px solid var(--border);
      vertical-align: top;
    }

    table.params th { color: var(--text-muted); font-weight: 600; }

    .tryit {
      display: flex;
      flex-direction: column;
      gap: 0.35rem;
      margin-bottom: 0.75rem;
    }

    .tryit label {
      font-size: 0.8rem;
      color: var(--text-muted);
    }

    input, textarea, select {
      background: var(--bg-elevated);
      color: var(--text);
      border: 1px solid var(--border);
      border-radius: 4px;
      padding: 0.45rem;
      font: inherit;
      width: 100%;
    }

    input:focus-visible,
    textarea:focus-visible,
    select:focus-visible,
    .btn:focus-visible {
      outline: 2px solid var(--accent);
      outline-offset: 2px;
    }

    .btn {
      background: var(--btn-bg);
      color: var(--btn-text);
      border: 1px solid var(--border);
      border-radius: 4px;
      padding: 0.5rem 1rem;
      font: inherit;
      cursor: pointer;
      align-self: flex-start;
      min-height: 2.75rem;
    }

    @media (min-width: 768px) {
      .content { padding: 1.5rem; }
      .topbar { padding: 1rem 1.5rem; }
    }

    @media (min-width: 1024px) {
      body.swagger-ui { font-size: 17px; }
    }
`
