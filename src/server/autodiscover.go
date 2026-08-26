package server

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/webappsgo/redxt/src/common/httputil"
	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/retry"
)

// Autodiscovery is the /api/autodiscover document from AI.md PART 14.
// It is deliberately a struct rather than a map so that the two rules
// the spec calls out — never publish the admin path, never publish a
// secret — are enforced by the type itself.
type Autodiscovery struct {
	// Primary is the URL of the server answering the request.
	Primary string `json:"primary"`
	// Cluster lists every advertised node URL, empty on a single node.
	Cluster []string `json:"cluster"`
	// APIVersion is the version segment clients should use.
	APIVersion string `json:"api_version"`
	// Timeout is the recommended request timeout in seconds.
	Timeout int `json:"timeout"`
	// Retry is the recommended number of retry attempts.
	Retry int `json:"retry"`
	// RetryDelay is the recommended delay between retries in seconds.
	RetryDelay int `json:"retry_delay"`
	// Config publishes the accepted configuration value sets.
	Config AutodiscoveryConfig `json:"config"`
}

// AutodiscoveryConfig publishes the configuration schema a client needs
// to validate a setting locally before sending it.
type AutodiscoveryConfig struct {
	Database AutodiscoveryDatabase `json:"database"`
	Cache    AutodiscoveryCache    `json:"cache"`
	Formats  AutodiscoveryFormats  `json:"formats"`
	Logging  AutodiscoveryLogging  `json:"logging"`
	SMTP     AutodiscoverySMTP     `json:"smtp"`
	Features map[string]any        `json:"features"`
}

// AutodiscoveryDatabase publishes the database driver value sets.
type AutodiscoveryDatabase struct {
	Drivers  []string          `json:"drivers"`
	Aliases  map[string]string `json:"aliases"`
	SSLModes []string          `json:"ssl_modes"`
}

// AutodiscoveryCache publishes the accepted cache backends.
type AutodiscoveryCache struct {
	Types []string `json:"types"`
}

// AutodiscoveryFormats publishes the accepted unit suffixes.
type AutodiscoveryFormats struct {
	Duration []string `json:"duration"`
	Size     []string `json:"size"`
}

// AutodiscoveryLogging publishes the accepted log levels.
type AutodiscoveryLogging struct {
	Levels []string `json:"levels"`
}

// AutodiscoverySMTP publishes the accepted SMTP transport modes.
type AutodiscoverySMTP struct {
	TLSModes []string `json:"tls_modes"`
}

// BuildAutodiscovery assembles the document for one request. The
// primary URL is derived from the request rather than from the config
// so a server reached through several hostnames tells each client the
// address that client can actually use.
func BuildAutodiscovery(o Options, r *http.Request) Autodiscovery {
	timeout := int(o.Config.Server.Limits.ReadTimeout.Duration().Seconds())
	if timeout <= 0 {
		timeout = 30
	}

	// The recommended retry advice mirrors the PART 9 default policy so
	// a client and the server give up after the same amount of work.
	backoff := retry.DefaultPolicy().Backoff
	attempts := len(backoff)
	retryDelay := 1
	for _, wait := range backoff {
		if seconds := int(wait.Seconds()); seconds > 0 {
			retryDelay = seconds
			break
		}
	}

	doc := Autodiscovery{
		Primary:    RequestBaseURL(r, o.trustsProxy(r)),
		Cluster:    []string{},
		APIVersion: strings.TrimPrefix(o.Config.APIBasePath(), "/api/"),
		Timeout:    timeout,
		Retry:      attempts,
		RetryDelay: retryDelay,
		Config: AutodiscoveryConfig{
			Database: AutodiscoveryDatabase{
				Drivers:  config.DatabaseDrivers,
				Aliases:  config.DatabaseDriverAliases,
				SSLModes: config.DatabaseSSLModes,
			},
			Cache:   AutodiscoveryCache{Types: config.CacheTypes},
			Formats: AutodiscoveryFormats{Duration: config.DurationUnits, Size: config.SizeUnits},
			Logging: AutodiscoveryLogging{Levels: config.LogLevels},
			SMTP:    AutodiscoverySMTP{TLSModes: config.SMTPTLSModes},
			// Tor is the one feature this PART can report on its own;
			// everything else comes from the provider so no flag is
			// invented before the PART that owns it is built.
			Features: map[string]any{
				"tor": o.Config.Tor.OnionAddress != "",
			},
		},
	}

	if o.Cluster != nil {
		if nodes := o.Cluster(); len(nodes) > 0 {
			doc.Cluster = nodes
		}
	}
	if o.Features != nil {
		for name, value := range o.Features() {
			doc.Config.Features[name] = value
		}
	}

	return doc
}

// NewAutodiscoverHandler answers GET /api/autodiscover.
func NewAutodiscoverHandler(o Options) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doc := BuildAutodiscovery(o, r)
		// The document changes only when the operator edits the config,
		// so clients may cache it for an hour, per AI.md PART 14.
		w.Header().Set("Cache-Control", "public, max-age=3600")
		WriteNegotiated(w, r, o, httputil.NegotiateAPI, Payload{
			JSON: doc,
			Text: autodiscoveryText(doc),
		})
	})
}

// autodiscoveryText renders the document in the same flattened
// dot-notation form PART 13 uses for the health endpoint, so a shell
// client parses both surfaces with one set of rules.
func autodiscoveryText(doc Autodiscovery) string {
	var b strings.Builder
	b.WriteString("primary: " + doc.Primary + "\n")
	b.WriteString("cluster: " + strings.Join(doc.Cluster, ", ") + "\n")
	b.WriteString("api_version: " + doc.APIVersion + "\n")
	b.WriteString("timeout: " + strconv.Itoa(doc.Timeout) + "\n")
	b.WriteString("retry: " + strconv.Itoa(doc.Retry) + "\n")
	b.WriteString("retry_delay: " + strconv.Itoa(doc.RetryDelay) + "\n")
	b.WriteString("config.database.drivers: " + strings.Join(doc.Config.Database.Drivers, ", ") + "\n")
	b.WriteString("config.database.ssl_modes: " + strings.Join(doc.Config.Database.SSLModes, ", ") + "\n")
	b.WriteString("config.cache.types: " + strings.Join(doc.Config.Cache.Types, ", ") + "\n")
	b.WriteString("config.formats.duration: " + strings.Join(doc.Config.Formats.Duration, ", ") + "\n")
	b.WriteString("config.formats.size: " + strings.Join(doc.Config.Formats.Size, ", ") + "\n")
	b.WriteString("config.logging.levels: " + strings.Join(doc.Config.Logging.Levels, ", ") + "\n")
	b.WriteString("config.smtp.tls_modes: " + strings.Join(doc.Config.SMTP.TLSModes, ", ") + "\n")
	for _, name := range sortedFeatureNames(doc.Config.Features) {
		b.WriteString("config.features." + name + ": " +
			fmt.Sprintf("%v", doc.Config.Features[name]) + "\n")
	}
	return b.String()
}

// RequestBaseURL returns the scheme-and-host prefix a client used to
// reach this server. The X-Forwarded-* headers are honored only when
// trusted is true, because an untrusted client can set them freely and
// would otherwise choose the URL this server advertises about itself.
// Default ports are stripped per the PART 15 display rules.
func RequestBaseURL(r *http.Request, trusted bool) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host

	if trusted {
		if proto := headerFirst(r, "X-Forwarded-Proto"); proto != "" {
			scheme = strings.ToLower(proto)
		}
		if forwarded := headerFirst(r, "X-Forwarded-Host"); forwarded != "" {
			host = forwarded
		}
	}

	if scheme == "https" {
		host = strings.TrimSuffix(host, ":443")
	} else {
		host = strings.TrimSuffix(host, ":80")
	}

	return scheme + "://" + host
}

// headerFirst returns the first comma-separated value of a header,
// which is the originating client's value in a proxy chain.
func headerFirst(r *http.Request, name string) string {
	value := r.Header.Get(name)
	if value == "" {
		return ""
	}
	if i := strings.IndexByte(value, ','); i >= 0 {
		value = value[:i]
	}
	return strings.TrimSpace(value)
}

// sortedFeatureNames returns the feature keys in a stable order so the
// text rendering is byte-identical between requests.
func sortedFeatureNames(features map[string]any) []string {
	names := make([]string, 0, len(features))
	for name := range features {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
