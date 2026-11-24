# Binary, Server CLI, Client & Agent Rules (PART 7, 8, 33)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Require CGO or add runtime dependencies; release binaries are static Go binaries with embedded application assets.
- Change the server's specified command/flag behavior or make help/version require elevated privileges.
- Introduce undocumented flag aliases, compound toggle flags, or a second parsing convention.
- Skip a required target platform or silently substitute a noncanonical OS/architecture name.
- Confuse the CLI client, server, agent, cluster node, and managed node roles.

## CRITICAL - ALWAYS DO
- Build and distribute the required `redxt`, `redxt-cli`, and `redxt-agent` binaries as defined by PARTs 7, 8, and 33.
- Honor the common help, version, color, signal, startup, and exit-code contracts across binaries.
- Support the documented eight targets: linux, darwin, windows, and freebsd on amd64 and arm64.
- Keep embedded app assets in the binary; download frequently updated security data into the data directory.
- Follow PART 33 for client setup/access, agent execution context, registration, updates, and cluster behavior.

## KEY DECISIONS (pre-answered)
| Question | Answer | Spec Reference |
|----------|--------|----------------|
| Required binaries? | `redxt` server, `redxt-cli` client, `redxt-agent` agent | PART 7, 8, 33 |
| Build runtime model? | One static binary per target, `CGO_ENABLED=0` | PART 7 |
| Are agents cluster nodes? | No; cluster nodes are server instances, agents are purpose-specific workers | PART 10, 33 |
| Are clients restricted to a subset of server operations? | Follow the PART 33 client access contract | PART 33 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| Server | `redxt`, the main service binary |
| Client | `redxt-cli`, the user-context companion |
| Agent | `redxt-agent`, purpose-specific worker on a managed node |
| Cluster node | Another redxt server instance sharing cluster services |
| Managed node | External DNS/redirect target operated or monitored by redxt |

## QUICK REFERENCE
- Keep CLI flags, commands, help output, color handling, and exit codes aligned with PART 8.
- Client and agent configuration, execution context, build output, and update rules are in PART 33.
- Cross-platform binary requirements are in PART 7.

---
For complete details, see AI.md PART 7, 8, 33
