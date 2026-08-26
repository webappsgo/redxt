package user

import "strings"

// Blocklist is the reserved-name list from AI.md PART 34, "Username
// Blocklist". Users and organizations share one namespace, so the same
// list guards both a username and an organization slug.
//
// Server Admin accounts are exempt: the primary admin is created before
// any of this runs and is not addressable through a vanity URL.
var Blocklist = []string{
	// System and administrative.
	"admin", "administrator", "root", "system", "sysadmin", "superuser",
	"master", "owner", "operator", "manager", "moderator", "mod",
	"staff", "support", "helpdesk", "help", "service", "daemon",

	// Server and technical.
	"server", "host", "node", "cluster", "api", "www", "web", "mail",
	"email", "smtp", "ftp", "ssh", "dns", "proxy", "gateway", "router",
	"firewall", "localhost", "local", "internal", "external", "public",
	"private", "network", "database", "db", "cache", "redis", "mysql",
	"postgres", "mongodb", "elastic", "nginx", "apache", "docker",
	"healthz", "metrics", "swagger",

	// Application and service names.
	"app", "application", "bot", "robot", "crawler", "spider", "scraper",
	"webhook", "callback", "cron", "scheduler", "worker", "queue", "job",
	"task", "process", "microservice", "lambda", "function",

	// Authentication and security.
	"auth", "authentication", "login", "logout", "signin", "signout",
	"signup", "register", "password", "passwd", "token", "oauth", "sso",
	"saml", "ldap", "kerberos", "security", "secure", "ssl", "tls",
	"certificate", "cert", "key", "secret", "credential", "session",

	// Roles and permissions.
	"guest", "anonymous", "anon", "user", "users", "member", "members",
	"subscriber", "editor", "author", "contributor", "reviewer", "auditor",
	"analyst", "developer", "dev", "devops", "engineer", "architect",
	"designer", "tester", "qa", "billing", "finance", "legal", "hr",
	"sales", "marketing", "ceo", "cto", "cfo", "coo", "founder", "cofounder",

	// Common reserved.
	"account", "accounts", "profile", "profiles", "settings", "config",
	"configuration", "dashboard", "panel", "console", "portal", "home",
	"index", "main", "default", "null", "nil", "undefined", "void",
	"true", "false", "test", "testing", "debug", "demo", "example",
	"sample", "temp", "temporary", "tmp", "backup", "archive", "log",
	"logs", "audit", "report", "reports", "analytics", "stats", "status",
	"about", "contact", "privacy", "terms", "docs",

	// API and endpoints.
	"rest", "graphql", "grpc", "websocket", "ws", "wss", "http",
	"https", "endpoint", "endpoints", "route", "routes", "path", "url",
	"uri", "hook", "hooks", "event", "events", "stream",
	"autodiscover",

	// Content and media.
	"blog", "news", "article", "articles", "post", "posts", "page", "pages",
	"feed", "rss", "atom", "sitemap", "robots", "favicon", "static",
	"assets", "images", "image", "img", "media", "upload", "uploads",
	"download", "downloads", "file", "files", "document", "documents",

	// Communication.
	"message", "messages", "chat", "notification", "notifications",
	"alert", "alerts", "inbox", "outbox", "sent", "draft", "drafts",
	"spam", "abuse", "flag", "block", "mute", "ban",

	// Commerce and billing.
	"shop", "store", "cart", "checkout", "order", "orders", "invoice",
	"invoices", "payment", "payments", "subscription", "subscriptions",
	"plan", "plans", "pricing", "refund", "coupon", "discount",

	// Social features.
	"follow", "follower", "followers", "following", "friend", "friends",
	"like", "likes", "share", "shares", "comment", "comments", "reply",
	"mention", "mentions", "tag", "tags", "group", "groups", "team", "teams",
	"community", "communities", "forum", "forums", "channel", "channels",

	// Brand and legal.
	"official", "verified", "trusted", "partner", "affiliate", "sponsor",
	"brand", "trademark", "copyright", "policy", "policies", "tos",
	"eula", "gdpr", "dmca",

	// Offensive and impersonation prevention.
	"fuck", "shit", "ass", "bitch", "bastard", "damn", "cunt", "dick",
	"penis", "vagina", "sex", "porn", "xxx", "nude", "naked", "nsfw",
	"kill", "murder", "death", "die", "suicide", "hate", "nazi", "hitler",
	"racist", "racism", "terrorist", "terrorism", "isis", "alqaeda",

	// Numbers and special.
	"0", "1", "123", "1234", "12345", "000", "111", "666", "911", "420", "69",

	// Common spam patterns.
	"info", "noreply", "no-reply", "donotreply", "mailer", "postmaster",
	"webmaster", "hostmaster", "junk", "trash",

	// Project-specific, per the "{project_name}"/"{project_org}" entries
	// in the PART 34 list.
	"redxt", "webappsgo",

	// The IDEA.md HTTP surface map: every service subdomain redxt serves
	// is reserved so a vanity name can never shadow one.
	"ddns", "cve", "data", "rdap", "whoami", "ns1", "ns2", "wpad", "isatap",
	"administration",
}

// substringBlocked are the terms PART 34 blocks anywhere inside a name,
// not only as the whole name, because they invite impersonation.
var substringBlocked = []string{"admin", "root", "system", "mod", "official", "verified"}

// blocklistSet indexes Blocklist for constant-time membership tests. It
// is built once at package initialization.
var blocklistSet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(Blocklist))
	for _, name := range Blocklist {
		set[name] = struct{}{}
	}
	return set
}()

// IsBlocked reports whether name is reserved, case-insensitively. It
// matches an exact reserved name, and also any name that contains one of
// the impersonation-prone terms as a substring.
func IsBlocked(name string) bool {
	lowered := strings.ToLower(strings.TrimSpace(name))
	if lowered == "" {
		return true
	}
	if _, found := blocklistSet[lowered]; found {
		return true
	}
	for _, term := range substringBlocked {
		if strings.Contains(lowered, term) {
			return true
		}
	}
	return false
}
