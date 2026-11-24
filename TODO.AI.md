# redxt implementation backlog

Tasks below are current gaps found while comparing the repository with AI.md and IDEA.md. Remove each item after its implementation and required verification are complete.

- [ ] Regenerate the stale `.claude/rules/*.md` files from the current AI.md PARTs and ensure `.claude/memory/MEMORY.md` exists without duplicating the spec.
- [x] Migrate IDEA.md to the exact required section structure, keep project variables unchanged, and document full DNS RFC scope.
- [ ] Remove remaining implementation details from IDEA.md and verify the complete API route inventory against AI.md and code.
- [ ] Implement and verify every project-defined API route in IDEA.md, including native management and compatibility APIs.
- [ ] Implement the DNS data-plane runtime: authoritative primary/secondary service, recursive resolver, forwarding, DNS firewall, encrypted DNS transports, and wire their configured listeners into startup.
- [ ] Implement the DNS control-plane services and API routes for zones, records, DNSSEC, TSIG, policies, DDNS, data zones, agents, templates, blocklists, transfers, query logs, and analytics. The existing `apiRoutes` table currently contains user and organization routes only.
- [ ] Complete the PART 17 admin pages, route protection, settings actions, and admin-specific shell; the current admin implementation adds setup, dashboard, identity settings, server information, and theme preference, while SSL, logs, scheduler editing, backups, users, cluster, and agents remain outstanding; remove inaccurate admin documentation.
- [ ] Bring all public server pages into PART 16 compliance, including the in-progress shared external CSS/theme and preferences, accessible responsive navigation, metadata, and no-JavaScript behavior.
- [ ] Complete project user-facing strings, supported locales, and language fallback/accessibility requirements from PART 31.
- [ ] Complete project user, organization, and custom-domain requirements under SPEC.md and PARTs 34-36.
- [ ] Finish documented product capabilities that remain placeholders or absent, including DNS roles, cluster/agent runtime, and client behavior.
- [ ] Reconcile README and all docs with implemented routes, configuration, features, and deployment instructions; complete the required documentation site configuration.
- [ ] Complete the required test harnesses and coverage, integration, and browser E2E coverage specified by PART 29.
- [ ] Recheck all AI.md final compliance requirements and resolve newly discovered implementation gaps.
