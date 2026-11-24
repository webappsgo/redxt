# Privilege Escalation & Service Rules (PART 24, 25)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Assume an init system from the OS name or install-time process context; detect the host's actual service manager using PART 24/25 order.
- Require permanent root/admin execution unless IDEA.md explicitly documents and justifies that exception.
- Leave service startup running with unnecessary privileges after privileged setup or binding is complete.
- Skip dedicated service identity creation when required; the only exception is explicit permanent-root authorization in IDEA.md.
- Omit user-service fallback when system service installation is unavailable.

## CRITICAL - ALWAYS DO
- Follow PART 24's privilege escalation, account creation, UID/GID, startup, and privilege-drop sequence on each supported OS.
- Support the service managers and user/system install modes defined by PART 25 (including Linux init variants, launchd, BSD rc.d, and Windows Service Manager).
- Make service install/uninstall/disable behavior idempotent, report accurate state, preserve the binary on uninstall, and emit the required help text.
- Keep the interactive server process usable without escalation with user-level directories when escalation is unavailable.

## KEY DECISIONS (pre-answered)
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Does installing an OS account require elevation? | Yes; when escalation is unavailable, run as the current user | PART 24 |
| Should service installation assume systemd? | No; detect the live init/service manager in the specified order | PART 24, 25 |
| Is permanent root the default? | No; explicit project justification in IDEA.md is required | PART 24 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| System service | Service installed at host scope, with dedicated account/privilege handling |
| User service | Service installed under the current user's session/permissions |
| Privilege drop | Transition from elevated setup/bind to the configured unprivileged runtime identity |

## QUICK REFERENCE
- PART 24 defines escalation detection and service account/privilege lifecycle.
- PART 25 defines service managers, command semantics, and service support requirements.
- Verify each platform's exact behavior in the spec; do not copy one OS's flow to another.

---
For complete details, see AI.md PART 24, 25
