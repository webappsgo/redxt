# AI Rules (PART 0, 1)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Guess or assume — READ THE SPEC or ASK
- Implement without reading the relevant PART first
- Modify AI.md PART content (read-only spec)
- Add features not in spec without asking
- Use "I think" or "probably" — KNOW from spec or ASK
- Ask multiple plain-text questions in separate messages — use the AskUserQuestion wizard
- Use generic placeholder content ("Your app name", "Feature 1")
- Create `/server/about` or `/server/help` with placeholder text — source from IDEA.md
- Leave TODO comments in committed code — implement fully or don't implement
- Create stub functions or "future" placeholders
- Ship partial implementations — every feature must be 100% complete
- Rely on memory for spec content — read the relevant PART when needed

## CRITICAL - ALWAYS DO
- Read the relevant PART before implementing ANY feature
- Search AI.md before asking questions (the answer is likely there)
- Follow spec EXACTLY — no unrequested "improvements"
- Update IDEA.md when project-specific features change
- Keep all docs in sync with code
- When unsure, ASK — never guess or assume
- Source `/server/about` and `/server/help` content from IDEA.md
- Commit COMMIT/NEVER/MUST rules to memory each session

## KEY DECISIONS (pre-answered)
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Where do business-logic answers live? | IDEA.md first, then AI.md | PART 0 |
| Can AI.md be edited? | No — read-only spec | PART 0, 1 |
| How to activate optional PART 34-36? | Declare in SPEC.md | PART 0 |
| What if IDEA.md is non-conforming? | Migrate before any other work | PART 0 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| server | `redxt` main binary |
| client | `redxt-cli` companion CLI/TUI |
| agent | `redxt-agent` (enabled — PART 33) |
| Server Admin | manages the app, not an OS user |
| Regular User | end-user account (PART 34, enabled) |

## QUICK REFERENCE
- Session start: read CLAUDE.md → check `.claude/rules/` freshness → read IDEA.md → proceed
- Every code change must trace to a specific AI.md PART or IDEA.md line
- Findings not fixed immediately go into TODO.AI.md, never left only in chat

---
For complete details, see AI.md PART 0, 1
