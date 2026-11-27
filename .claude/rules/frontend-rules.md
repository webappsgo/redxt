# Frontend Rules (PART 16, 17)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- ❌ Client-side rendering (React, Vue, Angular, etc.)
- ❌ Require JavaScript for core functionality
- ❌ Client-side routing (SPA)
- ❌ Business logic in JavaScript
- ❌ Let long strings break mobile layout
- ❌ Desktop-first CSS (use mobile-first)
- ❌ Inline CSS or JavaScript
- ❌ JavaScript alerts (use toast notifications)
- ❌ Generic placeholder content in /server/about or /server/help pages
- ❌ "Your application name here" or "Feature 1, Feature 2" text
- ❌ Stub templates or "coming soon" pages
- ❌ Empty handlers or placeholder routes

## CRITICAL - ALWAYS DO
- ✅ Server-side rendering (Go templates)
- ✅ Progressive enhancement (works without JS)
- ✅ Mobile-first responsive CSS
- ✅ CSS `word-break: break-all` for long strings (IPv6, .onion, tokens)
- ✅ Full admin panel with ALL settings
- ✅ WCAG 2.1 AA accessibility
- ✅ Touch targets minimum 44x44px
- ✅ /server/about content from IDEA.md (name, tagline, description, features)
- ✅ /server/help content from IDEA.md (real endpoints, real examples)
- ✅ All pages fully functional - no "coming soon" or placeholder pages
- ✅ All routes implemented - no 501 Not Implemented responses

## PAGE CONTENT SOURCING
| Page | Content Source |
|------|----------------|
| /server/about | IDEA.md → name, tagline, description, features, links |
| /server/help | IDEA.md → real endpoints, real curl examples, real FAQ |
| /server/privacy | Built-in privacy text (project data), extended by `server.branding.content.privacy`; `server.privacy.*` holds the behavior switches (consent, `data.sold`) AND deployment facts the page shows (`data.sharing`, `retention`, `cookies`, `third_party`); the other sections are built-in text |
| /server/terms | Config → customizable, default template |
| /server/contact | Config → `server.contact.general.*` + `server.pages.contact.*` settings |

## KEY DECISIONS
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| How are pages rendered? | Server-side Go templates | PART 16 |
| Is JavaScript required for core behavior? | No; use progressive enhancement | PART 16 |
| What standard governs accessibility? | WCAG 2.1 AA | PART 16 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| Hub | Registered page listing pages beneath Preferences or Help |
| No-JS | Core page behavior available without JavaScript |

## SERVER VS CLIENT
| Task | Where | Why |
|------|-------|-----|
| Data validation | SERVER | Server is authoritative |
| HTML rendering | SERVER | Works without JS |
| Business logic | SERVER | Security, consistency |
| Formatting | SERVER | Consistent output |
| Theme toggle | Client JS | Instant UX feedback |
| Copy to clipboard | Client JS | Browser API required |
| Form feedback | Client JS | UX enhancement only |

## LONG STRINGS (REQUIRED CSS)
```css
.long-string, .ip-address, .api-token, .hash {
  word-break: break-all;
  overflow-wrap: break-word;
  font-family: monospace;
}
```

Apply to: IPv6, API tokens, hashes, UUIDs, Base64 — values with no adjacent copy button.

`.onion-address` / `.i2p-address` are copy-button values, NOT covered by the wrap rule above — using wrap CSS on them breaks the copy target into multiple lines. They require the scroll-box treatment; see "### Long Strings (IPv6, Tor, Tokens, Hashes)" in AI.md for the full wrap-vs-scroll-box split and required CSS.

## BREAKPOINTS (mobile-first)
| Target | CSS |
|--------|-----|
| Mobile (base) | No media query |
| Tablet+ | `@media (min-width: 768px)` |
| Desktop+ | `@media (min-width: 1024px)` |

## QUICK REFERENCE
Use shared shell/components, token-based mobile-first CSS, accessible server-rendered templates, and required page conformance tests.

---
For complete details, see AI.md PART 16, 17
