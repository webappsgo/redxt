package startup

import (
	"net/http"

	"github.com/webappsgo/redxt/src/server/admin"
	"github.com/webappsgo/redxt/src/server/handler"
	"github.com/webappsgo/redxt/src/server/template"
)

// openAdminService builds the AI.md PART 17 admin account lifecycle.
// Every project ships an admin panel, so unlike the Regular User
// subsystem it has no enabled flag: it is absent only when the
// databases it reads and writes are not open.
//
// It is built ahead of the Regular User handler, which needs it to
// answer an admin sign-in on the shared login form, and ahead of the
// admin panel handler itself, which needs it for the same reason the
// Regular User handler needs the Regular User service.
func (s *Server) openAdminService() *admin.Service {
	if s.UsersDB == nil || s.ServerDB == nil {
		return nil
	}
	return admin.NewService(s.UsersDB, s.ServerDB)
}

// openAdminHandler builds the admin panel's server-rendered surface.
func (s *Server) openAdminHandler(svc *admin.Service, users *handler.Handler) (*admin.Handler, error) {
	if svc == nil {
		return nil, nil
	}

	pages, err := template.New()
	if err != nil {
		return nil, err
	}

	return admin.New(admin.Options{
		Service:       svc,
		Config:        s.Config,
		Templates:     pages,
		IsRegularUser: regularUserSession(users),
	})
}

// regularUserSession reports whether a request carries a valid,
// signed-in Regular User session, without re-verifying the credential:
// it reads the identity the Regular User service already resolves for
// its own session cookie. A disabled Regular User subsystem never
// reports one, so the admin dashboard falls through to the shared
// sign-in page instead.
func regularUserSession(users *handler.Handler) func(*http.Request) bool {
	if users == nil {
		return nil
	}
	return func(r *http.Request) bool {
		cookie, err := r.Cookie(users.SessionCookieName())
		if err != nil || cookie.Value == "" {
			return false
		}
		_, _, err = users.Service().ResolveSession(r.Context(), cookie.Value)
		return err == nil
	}
}
