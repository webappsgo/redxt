# Configuration Rules (PART 5, 6, 12)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Put comments inline in YAML — comments always go ABOVE the setting
- Auto-enable debug mode — `MODE=debug` is explicit opt-in only, never implied
- Let debug mode bypass authentication or security checks, in any mode
- Skip persisting a randomly-selected first-run port — save it to server.yml
- Assume a fixed default port other than a random 64000-64999 value on first run

## CRITICAL - ALWAYS DO
- Priority for persisted settings: CLI flag > environment variable > config file > built-in default
- Mode priority: `--mode` flag > `MODE` env > default `production`
- Debug priority: `--debug`/`DEBUG` flag/env > `MODE=debug` default-on > default `false`
- Redact credentials (keys, tokens, passwords, secrets) in ALL modes, including debug
- Auto-enable Let's Encrypt HTTP-01 on port 80, TLS-ALPN-01 + auto-SSL on port 443
- Honor init-only env vars (CONFIG_DIR, DATA_DIR, LOG_DIR, DATABASE_DIR, BACKUP_DIR, PORT, LISTEN, APPLICATION_NAME, APPLICATION_TAGLINE) only on first run

## KEY DECISIONS (pre-answered)
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Default port range | Random 64000-64999, persisted after selection | PART 5 |
| Six operational states | prod/dev/debug × debug-flag on/off | PART 6 |
| Debug endpoints gate | `--debug`/`DEBUG=true` only, independent of mode | PART 6 |
| Privileged port <1024 | Escalate once at service install, then drop privileges | PART 5 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| Production mode | Default; minimal logging, no debug endpoints |
| Development mode | Verbose logging, relaxed caching, debug endpoints still off |
| Debug mode (`MODE=debug`) | Explicit opt-in; defaults debug flag on |
| Debug flag | Enables `/debug/*`, pprof, expvar regardless of mode |

## QUICK REFERENCE
- Console banner: `🔒 Running in mode: production [debugging]` / `🔧 Running in mode: development [debugging]`
- Config design: clean, everything configurable, sane defaults, single-line comments <140 chars

---
For complete details, see AI.md PART 5, 6, 12
