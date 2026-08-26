package server

import (
	"net/http"
	"strings"

	"github.com/webappsgo/redxt/src/apierror"
	"github.com/webappsgo/redxt/src/common/httputil"
	"github.com/webappsgo/redxt/src/ssl"
)

// Routes carries the handlers the router mounts. A nil field means the
// PART that owns that surface is not wired in yet, and the router simply
// does not mount its paths; the routes that PART 14 itself owns (the
// index, health, and autodiscover) fall back to handlers built here.
type Routes struct {
	// Index answers GET / with the web interface.
	Index http.Handler
	// Health answers the frontend health routes with content
	// negotiation. When nil, the router builds one from Options.
	Health http.Handler
	// HealthAPI answers the API health routes, defaulting to JSON.
	// When nil, the router builds one from Options.
	HealthAPI http.Handler
	// Autodiscover answers /api/autodiscover. When nil, the router
	// builds one from Options.
	Autodiscover http.Handler
	// SwaggerUI serves the interactive REST explorer (PART 14).
	SwaggerUI http.Handler
	// SwaggerSpec serves the OpenAPI JSON document (PART 14).
	SwaggerSpec http.Handler
	// GraphQLUI serves the GraphiQL explorer (PART 14).
	GraphQLUI http.Handler
	// GraphQLAPI answers GraphQL POST queries (PART 14).
	GraphQLAPI http.Handler
	// Metrics serves the Prometheus surface (PART 21).
	Metrics http.Handler
	// Admin serves the admin panel (PART 17).
	Admin http.Handler
	// AdminAPI serves the bearer-authenticated admin API (PART 17).
	AdminAPI http.Handler
	// ACME serves the HTTP-01 challenge tree (PART 15). When nil, the
	// router falls back to the TLS provider's own challenge handler.
	ACME http.Handler
	// Users serves the server-rendered Regular User, organization and
	// custom-domain pages (PART 34, 35, 36).
	Users http.Handler
	// UsersPrefixes are the patterns Users is mounted on. The handler
	// routes the full path itself, so nothing is stripped here.
	UsersPrefixes []string
	// UsersAPI serves the versioned REST surface for the same PARTs.
	UsersAPI http.Handler
	// UsersAPIPrefixes are the patterns UsersAPI is mounted on.
	UsersAPIPrefixes []string
}

// NewRouter builds the complete route tree from the endpoint table in
// AI.md PART 14, "Root-Level Endpoints".
//
// Every unversioned /api alias is mounted on the same handler value as
// its versioned path, never as a redirect, exactly as PART 14 requires.
func NewRouter(o Options) *http.ServeMux {
	mux := http.NewServeMux()

	routes := o.Routes
	if routes.Health == nil {
		routes.Health = NewHealthHandler(o, NegotiateHealthFrontend)
	}
	if routes.HealthAPI == nil {
		routes.HealthAPI = NewHealthHandler(o, httputil.NegotiateAPI)
	}
	if routes.Autodiscover == nil {
		routes.Autodiscover = NewAutodiscoverHandler(o)
	}
	if routes.Index == nil {
		routes.Index = newIndexHandler(o)
	}
	if routes.ACME == nil && o.TLS != nil {
		routes.ACME = o.TLS.HTTP01Handler()
	}

	api := o.Config.APIBasePath()
	adminSegment := strings.TrimPrefix(o.Config.AdminBasePath(), "/server/")

	// The frontend surface.
	mux.Handle("GET /{$}", routes.Index)
	mux.Handle("GET /server/healthz", routes.Health)
	if o.Config.Server.Healthz.Root.Enabled {
		// Same handler value, never a redirect, per PART 14's optional
		// root operational alias table.
		mux.Handle("GET /healthz", routes.Health)
	}

	if routes.SwaggerUI != nil {
		mux.Handle("GET /server/docs/swagger", routes.SwaggerUI)
	}
	if routes.GraphQLUI != nil {
		mux.Handle("GET /server/docs/graphql", routes.GraphQLUI)
	}

	if routes.Metrics != nil {
		mount(mux, "GET", routes.Metrics,
			"/server/metrics",
			"/server/metrics/{service}",
			// The root alias defaults to enabled per PART 14.
			"/metrics",
			"/metrics/{service}",
			"/api/metrics",
			"/api/metrics/{service}",
			api+"/server/metrics",
			api+"/server/metrics/{service}",
		)
	}

	if routes.Admin != nil {
		mux.Handle("/server/"+adminSegment, routes.Admin)
		mux.Handle("/server/"+adminSegment+"/", routes.Admin)
	}
	if routes.AdminAPI != nil {
		mux.Handle(api+"/server/"+adminSegment+"/", routes.AdminAPI)
	}

	// The Regular User surfaces (PART 34, 35, 36). Both are mounted on
	// prefixes their own handler resolves, which keeps the route table
	// for those PARTs in one place instead of split across two files.
	if routes.Users != nil {
		mountPrefixes(mux, routes.Users, routes.UsersPrefixes)
	}
	if routes.UsersAPI != nil {
		mountPrefixes(mux, routes.UsersAPI, routes.UsersAPIPrefixes)
	}

	// The API surface. Each alias shares the handler value with the
	// versioned path it aliases.
	mux.Handle("GET /api/autodiscover", routes.Autodiscover)
	mount(mux, "GET", routes.HealthAPI, "/api/healthz", api+"/server/healthz")

	if routes.SwaggerSpec != nil {
		mount(mux, "GET", routes.SwaggerSpec, "/api/swagger", api+"/server/swagger")
	}
	if routes.GraphQLAPI != nil {
		mount(mux, "POST", routes.GraphQLAPI, "/api/graphql", api+"/server/graphql")
	}

	// The ACME HTTP-01 challenge tree is served in the clear on every
	// listener so a renewal succeeds even before a certificate exists.
	if routes.ACME != nil {
		mux.Handle("GET "+ssl.ChallengePathPrefix, routes.ACME)
	}

	mux.Handle("/", notFoundHandler(o))

	return mux
}

// mount registers one handler under several patterns with the same
// method, which is how PART 14's unversioned aliases avoid becoming
// redirects or duplicated handler code.
func mount(mux *http.ServeMux, method string, h http.Handler, patterns ...string) {
	for _, p := range patterns {
		mux.Handle(method+" "+p, h)
	}
}

// mountPrefixes registers one handler under several path prefixes for
// every method, leaving method routing to the handler itself.
func mountPrefixes(mux *http.ServeMux, h http.Handler, patterns []string) {
	for _, p := range patterns {
		if p == "" {
			continue
		}
		mux.Handle(p, h)
	}
}

// notFoundHandler answers an unmatched path in the format the caller
// asked for, so a missing API route never returns an HTML error page.
func notFoundHandler(o Options) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, o, apierror.New(apierror.CodeNotFound))
	})
}

// newIndexHandler answers GET / until PART 16 supplies the real web
// interface. It reports the running application rather than a
// placeholder page, and it honors the same negotiation as every other
// frontend route.
func newIndexHandler(o Options) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload := map[string]any{
			"name":    o.Config.Server.ApplicationName,
			"tagline": o.Config.Server.ApplicationTagline,
		}
		title := o.Config.Server.ApplicationName
		body := "<h1>" + escapeHTML(title) + "</h1>\n<p>" +
			escapeHTML(o.Config.Server.ApplicationTagline) + "</p>\n"
		WriteNegotiated(w, r, o, httputil.NegotiateFrontend, Payload{
			JSON: payload,
			Text: title + "\n" + o.Config.Server.ApplicationTagline + "\n",
			HTML: body,
		})
	})
}
