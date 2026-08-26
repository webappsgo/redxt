// Package graphql — theming for the GraphiQL page, required by the
// AI.md PART 14 rule that both API documentation UIs follow the
// project-wide light/dark/auto theme system.
//
// Every colour lives in the custom-property token block at the top of
// the stylesheet; nothing below that block names a colour literal. The
// dark palette is the default, the light palette applies when the root
// element carries theme-light, and auto mode is pure CSS through
// prefers-color-scheme, with no script involved.

package graphql

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

// CSS returns the complete inline stylesheet for the GraphiQL page.
//
// It is inlined into the page rather than served as a separate asset so
// the document stays self-contained: a strict Content-Security-Policy
// that forbids external hosts cannot break it, and no CDN is contacted.
func CSS() string {
	return themeTokens + graphiqlCSS
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
      --field: var(--dark-accent-cyan);
      --ok: var(--dark-accent-green);
      --warn: var(--dark-accent-orange);
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
        --field: var(--light-accent-blue);
        --ok: var(--light-accent-green);
        --warn: var(--light-accent-orange);
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
      --field: var(--light-accent-blue);
      --ok: var(--light-accent-green);
      --warn: var(--light-accent-orange);
      --btn-bg: var(--light-accent-blue);
      --btn-text: var(--light-bg);
      --tint: rgba(26, 26, 26, 0.04);
    }
`

// graphiqlCSS is the mobile-first layout for the query explorer. It
// names no colour of its own, only the tokens defined above.
const graphiqlCSS = `
    * { box-sizing: border-box; }

    body.graphiql-container {
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

    a { color: var(--field); text-decoration: underline; }

    .content {
      padding: 1rem;
      max-width: 60rem;
      margin: 0 auto;
    }

    .intro { color: var(--text-muted); }

    .notice {
      border: 1px solid var(--warn);
      border-left-width: 4px;
      border-radius: 6px;
      padding: 0.6rem 0.75rem;
      color: var(--warn);
      background: var(--tint);
    }

    .section-title {
      font-size: 0.8rem;
      text-transform: uppercase;
      letter-spacing: 0.05em;
      color: var(--accent-alt);
      margin-bottom: 0.35rem;
    }

    .root-name {
      font-size: 1rem;
      color: var(--accent);
      margin-bottom: 0.25rem;
    }

    .editor {
      display: flex;
      flex-direction: column;
      gap: 0.4rem;
      margin: 1rem 0;
    }

    .editor label {
      font-size: 0.8rem;
      color: var(--text-muted);
    }

    code, pre, textarea {
      font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
      font-size: 0.85rem;
    }

    input, textarea {
      background: var(--bg-elevated);
      color: var(--text);
      border: 1px solid var(--border);
      border-radius: 4px;
      padding: 0.45rem;
      width: 100%;
    }

    input { font: inherit; }

    textarea { resize: vertical; }

    input:focus-visible,
    textarea:focus-visible,
    .btn:focus-visible,
    summary:focus-visible {
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

    .execute-button { font-weight: 600; }

    .result-window {
      background: var(--bg-alt);
      border: 1px solid var(--border);
      border-left: 4px solid var(--ok);
      border-radius: 6px;
      padding: 0.75rem;
      overflow-x: auto;
      white-space: pre-wrap;
      margin: 0 0 1rem;
    }

    .fields {
      margin: 0 0 1rem;
      border: 1px solid var(--border);
      border-radius: 6px;
      background: var(--tint);
      padding: 0.65rem 0.75rem;
    }

    .fields dt {
      color: var(--field);
      margin-top: 0.5rem;
    }

    .fields dt:first-child { margin-top: 0; }

    .fields dd {
      margin: 0.15rem 0 0;
      color: var(--text-muted);
      font-size: 0.875rem;
    }

    .schema summary {
      cursor: pointer;
      padding: 0.5rem 0;
      color: var(--accent);
    }

    @media (min-width: 768px) {
      .content { padding: 1.5rem; }
      .topbar { padding: 1rem 1.5rem; }
    }

    @media (min-width: 1024px) {
      body.graphiql-container { font-size: 17px; }
    }
`
