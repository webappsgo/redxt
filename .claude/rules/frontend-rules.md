# Web Frontend & Admin Panel Rules (PART 16, 17)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Use a client-side rendering framework; render pages with Go `html/template` and server-side handlers.
- Pass user-controlled content to trusted template/HTML types without the specified sanitization.
- Add JavaScript for behavior that native HTML and CSS can provide, or make a feature unusable with JavaScript disabled.
- Put page styles inline or hardcode theme colors; use the shared CSS/theme system.
- Expose admin navigation or authenticated/private pages through public routes, sitemap, feeds, or cacheable responses.

## CRITICAL - ALWAYS DO
- Build a complete, mobile-first, accessible frontend that consumes the backend API and exposes browser workflows for user-facing features.
- Use shared page partials, theme-matched error responses, route normalization, and the security/privacy rules in PART 16.
- Meet WCAG AA accessibility, keyboard and screen-reader support, responsive layouts, and light/dark theme contrast.
- Implement the admin path and setup/auth flows exactly as configured by PART 17; keep server admins distinct from regular users.
- Source project-specific public/about/help/privacy content from IDEA.md and deployment facts from configuration; never use generic placeholder copy.
- Localize every human-readable frontend/admin string and keep templates and assets embedded as specified.

## KEY DECISIONS (pre-answered)
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Rendering model? | Server-rendered Go `html/template` | PART 16 |
| Is JavaScript required for core workflows? | No; it is progressive enhancement only | PART 16 |
| What pages may be indexed? | Only public pages allowed by robots and the PART 16 sitemap/feed rules | PART 16 |
| Are server admins and regular users one account type? | No; distinct roles and storage | PART 17 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| Progressive enhancement | Optional JavaScript improves a workflow that already works without it |
| Primary Admin | First server admin with the protections defined by PART 17 |
| `admin_path` | Configured admin URL segment with canonical routing rules |

## QUICK REFERENCE
- Use native forms, validation, navigation, dialogs, and CSS where supported; scripts remain external and minimal.
- Apply the shared theme to normal, admin, error, and generated documentation interfaces.
- PART 17 controls setup, auth, admin settings, roles, sessions, and administration routes.

---
For complete details, see AI.md PART 16, 17
