package startup

import (
	"github.com/webappsgo/redxt/src/security"
	"github.com/webappsgo/redxt/src/server/admin"
	"github.com/webappsgo/redxt/src/server/handler"
	"github.com/webappsgo/redxt/src/server/middleware"
	"github.com/webappsgo/redxt/src/server/service"
	"github.com/webappsgo/redxt/src/server/store"
	"github.com/webappsgo/redxt/src/server/template"
)

// openUsers builds the Regular User subsystem from AI.md PART 34
// (Multi-User), PART 35 (Organizations), and PART 36 (Custom Domains).
//
// It returns nil when the subsystem is switched off, and the caller then
// mounts none of its routes and installs none of its credential
// verification: a disabled subsystem is absent from the running server
// rather than present and refusing.
//
// adminSvc backs the admin sign-in check the shared /server/auth/login
// form also performs (PART 17); a nil value simply leaves that fallback
// disabled.
func (s *Server) openUsers(adminSvc *admin.Service) (*handler.Handler, error) {
	if !s.Config.Server.Users.Enabled || s.UsersDB == nil {
		return nil, nil
	}

	cipher, err := security.NewCipher(
		s.Config.Server.Security.EncryptionKey,
		s.Config.Server.Security.EncryptionKeyVersion,
	)
	if err != nil {
		return nil, err
	}

	svc, err := service.New(service.Options{
		Store:  store.New(s.UsersDB),
		Config: s.Config,
		Cipher: cipher,
	})
	if err != nil {
		return nil, err
	}

	// Custom-domain certificates are issued by the same manager that
	// holds the server's own certificate, which is what keeps PART 36
	// from growing a second TLS path beside PART 15.
	svc.SetCertManager(s.SSL)

	pages, err := template.New()
	if err != nil {
		return nil, err
	}

	return handler.New(handler.Options{
		Service:      svc,
		Config:       s.Config,
		Templates:    pages,
		ClientIP:     middleware.ClientIPFunc(s.URLVars),
		AdminService: adminSvc,
	})
}
