# Frontend / Admin Panel Rules (PART 16, 17)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Use client-side rendering frameworks (React/Vue) — server-side Go templates only
- Add JavaScript for anything HTML5+CSS already solves (forms, validation, show/hide, dialogs, tabs) — JS is a last resort
- Hardcode colors — use CSS custom properties for the theme system
- Let long strings break mobile layout — use word-break CSS
- Put the admin panel behind a fixed, non-configurable path

## CRITICAL - ALWAYS DO
- Mobile-first responsive CSS (breakpoints: tablet 768px, desktop 1024px)
- Support light/dark/auto theme, project-wide and non-negotiable
- Every feature must work without JavaScript
- Isolate the admin panel under `/server/{admin_path}` (configurable path, default `administration`)
- Implement the first-run setup wizard for the Primary Admin
- Apply the same theme to Swagger/GraphQL UIs as the rest of the app

## KEY DECISIONS (pre-answered)
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Rendering | Server-side Go templates | PART 16 |
| Theme default | auto (light/dark via `prefers-color-scheme`) | PART 16 |
| Admin path | Configurable, default `administration` | PART 17 |
| Server Admin vs Regular User | Separate DB tables, never merged | PART 17 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| admin_path | Configurable URL segment for the admin panel |
| Primary Admin | First admin account, cannot be deleted |

## QUICK REFERENCE
- Breakpoints: `@media (min-width: 768px)` tablet+, `@media (min-width: 1024px)` desktop+
- Error pages must match the active theme

---
For complete details, see AI.md PART 16, 17
