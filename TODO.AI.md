# redxt implementation backlog

Tasks below are current gaps found while comparing the repository with AI.md and IDEA.md. Remove each item after its implementation and required verification are complete.

- [x] Regenerate all 14 `.claude/rules/*.md` files from the current AI.md PARTs and ensure `.claude/memory/MEMORY.md` exists without duplicating the spec.
- [x] Migrate IDEA.md to the exact required section structure, keep project variables unchanged, and document full DNS RFC scope.
- [ ] Remove remaining implementation details from IDEA.md and verify the complete API route inventory against AI.md and code.
- [ ] Implement and verify every project-defined API route in IDEA.md, including native management and compatibility APIs.
- [ ] Complete and verify the DNS data plane: `src/dns` has a bounded authoritative handler for verified primary/secondary zones, Do53 UDP/TCP listeners using `dns.address`/`dns.port`, and RFC 8484 GET/POST DoH at `/dns-query` on HTTPS (including trusted proxy scheme resolution). DNSSEC, ACLs, transfers, updates, recursion/forwarding, DoT/DoQ/DNSCrypt, encrypted upstreams, DoH integration behind the TLS/proxy deployment matrix, and the remaining IDEA behavior are still missing.
- [ ] Implement the DNS control-plane services and API routes for zones, records, DNSSEC, TSIG, policies, DDNS, data zones, agents, templates, blocklists, transfers, query logs, and analytics. The existing `apiRoutes` table currently contains user and organization routes only.
- [ ] Implement all PART 10 database drivers accepted by configuration (PostgreSQL, MySQL/MariaDB, SQL Server, libSQL, and MongoDB); current `src/database` opens SQLite only and rejects the other configured types.
- [ ] Complete the PART 17 admin pages, route protection, settings actions, and admin-specific shell; the current admin implementation adds setup, dashboard, identity settings, server information, and theme preference, while SSL, logs, scheduler editing, backups, users, cluster, and agents remain outstanding. `docs/admin.md` and `docs/configuration.md` now describe the current surface and reserved preferences path.
- [ ] Finish PART 16 public pages: a shared external CSS/theme, navigation/footer, and a basic preference form now exist; remaining gaps include the registered Preferences/Help hubs and page registry, consent-gated preferences and CCPA controls, complete translated public routes, offline-capable PWA, metadata/manifest/service worker, and page conformance requirements.
- [ ] Complete project user-facing strings, supported locales, and language fallback/accessibility requirements from PART 31.
- [ ] Complete project user, organization, and custom-domain requirements under SPEC.md and PARTs 34-36.
- [ ] Finish documented product capabilities that remain placeholders or absent, including DNS roles, cluster/agent runtime, and client behavior.
- [ ] Reconcile README and all docs with implemented routes, configuration, features, and deployment instructions; complete the required documentation site configuration.
- [ ] Complete the required test harnesses and coverage, integration, and browser E2E coverage specified by PART 29; `tests/run_tests.sh` now dispatches only to the Docker basic tier, while `tests/docker.sh` still lacks the required full CRUD, setup/auth, endpoint, client/agent, Tor/I2P, and teardown checks.
- [ ] Recheck all AI.md final compliance requirements and resolve newly discovered implementation gaps.
