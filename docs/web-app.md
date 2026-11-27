# Web app and offline behavior

redxt serves its web pages from Go templates. Forms and navigation work with
JavaScript disabled; the bundled script adds install prompts, connection
status, service worker updates, and recovery after a server restart.

The root page (`/`) is the public landing page. It uses the configured
application name and tagline, the project description and implemented feature
summary, and links to About, Help, and sign-in. The shared navigation exposes
registration when it is enabled and account pages after sign-in.

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
