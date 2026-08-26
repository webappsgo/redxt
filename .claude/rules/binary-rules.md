# Binary / CLI / Client & Agent Rules (PART 7, 8, 33)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Ship anything but a single static binary per platform (no external runtime deps)
- Use CGO — `CGO_ENABLED=0` always
- Change the fixed server binary command/flag set documented in PART 8
- Hand-roll argument parsing — use `flag` stdlib or `pflag`/`cobra`
- Use compound hyphenated toggle flags (`--enable-tls`) — use `--enable tls`
- Require root for `--help`/`--version` at any command level
- Skip any of the 8 target platforms (linux/darwin/windows/freebsd × amd64/arm64)

## CRITICAL - ALWAYS DO
- Ship `redxt` (server), `redxt-cli` (client, required), `redxt-agent` (agent, required — this project manages/monitors external nodes per PART 33)
- Honor `NO_COLOR` and `TERM=dumb` on every binary
- Support both `--flag value` and `--flag=value`
- Register `help` as a bare command at every level, identical output to `--help`
- Use Argon2id for password hashing (never bcrypt), SHA-256 for token hashing

## KEY DECISIONS (pre-answered)
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Client required? | Yes, `redxt-cli`, for all projects | PART 33 |
| Agent required? | Yes — redxt manages/monitors cluster + external nodes | PART 33 |
| Color default | `auto` (TTY-detect); `NO_COLOR` env always disables | PART 7, 8 |
| Help field width | 38-char left-aligned item, `- ` + description ≤100 chars | PART 8 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| Server binary | `redxt` — fixed CLI command set, cannot be changed |
| Client binary | `redxt-cli` — multi-command CLI/TUI |
| Agent binary | `redxt-agent` — runs on remote/managed nodes |

## QUICK REFERENCE
- Binary naming: `{name}-{GOOS}-{GOARCH}`, `.exe` suffix on Windows, `darwin` never `macos`
- Standard flags: `--help/-h`, `--version/-v`, `--debug`, `--color`

---
For complete details, see AI.md PART 7, 8, 33
