# Web app and offline behavior

redxt serves its web pages from Go templates. Forms and navigation work with
JavaScript disabled; the bundled script adds install prompts, connection
status, service worker updates, and recovery after a server restart.

The root page (`/`) is the public landing page. It uses the configured
application name and tagline, the project description and implemented feature
summary, and links to About, Help, and sign-in. The shared navigation exposes
registration when it is enabled and account pages after sign-in.

## Preferences, consent, and sync

Guest appearance and language settings live at `/server/preferences` and are
stored in browser cookies. Theme, language, and application preference writes
require the visitor to accept the Preferences cookie category. Cookie consent
and the CCPA opt-out remain local to that browser and are not included in a
preference export.

The Reset control removes the app's guest preference, consent, CCPA, and build
cookies, then asks the browser to clear site storage. It preserves sign-in and
CSRF credentials. The page and its reset action work without JavaScript.

Cross-device sync is available at `/server/preferences/sync`. The page shows a
shareable import URL and an unpadded base64url code, with an import field for a
pasted URL, query string, or code. Imports are additive: omitted preferences
keep their current values. The receiving browser must separately accept the
Preferences cookie category before imported settings can be written. Consent,
CCPA, build, session, CSRF, and other credential values are never shared.
Copy-to-clipboard controls are an optional enhancement; the URL and code can
also be selected and copied without JavaScript. See the [API reference](api.md)
for the matching guest preference endpoints and response formats.

## Install redxt

Open the site over HTTPS in a supported browser and use its install or
add-to-home-screen action. On iOS, open the browser's Share menu and choose
**Add to Home Screen**. Service workers require HTTPS; browsers may allow them
on localhost during development.

## Offline behavior

The service worker caches versioned styles, scripts, and icons. It tries the
network first for page navigations and may show a cached copy only for pages
the server explicitly marks as public. If no eligible page is available, it
shows a localized offline page. Authenticated pages and API responses are not
cached or queued for later submission.

Each release uses a version and commit stamp in asset URLs. The browser
revalidates the service worker and manifest, while cached static assets are
discarded when their version expires or a new service worker activates. An
available update waits for the visitor to accept it. The first installation
activates automatically.

The browser can show install and update prompts only where the platform
supports them. The site remains usable if JavaScript, storage, or service
workers are unavailable.
