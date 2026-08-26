# Service / Privilege Escalation Rules (PART 24, 25)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Run the server permanently as root/Administrator unless IDEA.md explicitly justifies it (ongoing OS-level privilege genuinely required)
- Require escalation for anything other than one-time service install/port-bind setup
- Skip privilege drop after binding a privileged port during service install

## CRITICAL - ALWAYS DO
- Detect escalation needs per-OS (systemd on Linux, launchd on macOS, rc.d on BSD, Windows Service Manager)
- Install as a dedicated system user/group, not root, whenever privilege drop is supported
- Provide `--service --install`/`--uninstall`/`--disable` commands with correct help output
- Support both service (escalated, any port) and `$USER` (non-escalated, >1024 only) run modes

## KEY DECISIONS (pre-answered)
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Default service account | Dedicated non-root system user/group | PART 24 |
| Does redxt need permanent root? | Only if IDEA.md justifies binding privileged DNS port 53 as an ongoing requirement — flag and confirm with the user before assuming | PART 24 |
| Service manager per-OS | systemd / launchd / rc.d / Windows SCM | PART 25 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| Service (escalated) mode | Started via `sudo redxt --service --install` |
| User ($USER) mode | Started as calling user, ports >1024 only |

## QUICK REFERENCE
- redxt's DNS listeners (UDP/TCP 53) are a privileged-port case distinct from the HTTP admin port — resolve this explicitly in IDEA.md/PART 24 implementation, do not assume

---
For complete details, see AI.md PART 24, 25
