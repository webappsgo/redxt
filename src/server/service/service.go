// Package service holds the policy layer for AI.md PART 34 (Multi-User),
// PART 35 (Organizations), and PART 36 (Custom Domains).
//
// Every rule that decides whether an action is allowed lives here. The
// store below it reads and writes rows without judging them, and the
// handlers above it translate HTTP into calls and results into
// responses. Keeping the decision in one layer is what makes it possible
// to state that an organization's data is scoped on every path: there is
// exactly one place where the scope is applied.
package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/security"
	"github.com/webappsgo/redxt/src/server/store"
	"github.com/webappsgo/redxt/src/ssl"
)

// Errors the service layer reports. Handlers map these onto the PART 14
// response envelope; nothing below this layer knows about HTTP.
var (
	// ErrInvalidCredentials is returned for every failed authentication,
	// whatever the actual cause. A wrong password, an unknown account, a
	// disabled account and an unverified address all produce this one
	// error, because PART 11 forbids telling a caller which of them
	// applied.
	ErrInvalidCredentials = errors.New("service: invalid credentials")
	// ErrAccountLocked reports that too many failed attempts have
	// suspended sign-in for a while.
	ErrAccountLocked = errors.New("service: account locked")
	// ErrTwoFactorRequired reports that the password was accepted and a
	// second factor is still outstanding.
	ErrTwoFactorRequired = errors.New("service: two-factor required")
	// ErrNotFound reports that the requested object does not exist, or
	// exists but is not visible to the caller. The two cases deliberately
	// share one error so a caller cannot map the namespace of another
	// organization by watching which identifiers answer differently.
	ErrNotFound = errors.New("service: not found")
	// ErrForbidden reports that the caller is known but not permitted.
	ErrForbidden = errors.New("service: forbidden")
	// ErrConflict reports a name, address, slug, or domain already taken.
	ErrConflict = errors.New("service: already exists")
	// ErrValidation reports caller input that failed a rule. The wrapped
	// error carries the detail shown to the caller.
	ErrValidation = errors.New("service: validation failed")
	// ErrDisabled reports that the feature is switched off in the server
	// configuration.
	ErrDisabled = errors.New("service: feature disabled")
	// ErrQuotaExceeded reports that a configured per-user or per-org limit
	// has been reached.
	ErrQuotaExceeded = errors.New("service: quota exceeded")
)

// Options configures a Service.
type Options struct {
	// Store is the users.db data-access layer.
	Store *store.Store
	// Config is the live server configuration. It is read on every call
	// rather than snapshotted, so an admin switching registration mode at
	// runtime takes effect on the next request, as PART 34 requires.
	Config *config.Config
	// Cipher encrypts the secrets that must be recoverable rather than
	// merely verifiable, which in PART 34 means TOTP seeds. Passwords are
	// never encrypted; they are Argon2id hashes.
	Cipher *security.Cipher
	// Now returns the current time. Tests replace it; production leaves
	// it nil and gets time.Now.
	Now func() time.Time
}

// Service applies the PART 34, 35 and 36 rules on top of the store.
type Service struct {
	store  *store.Store
	config *config.Config
	cipher *security.Cipher
	clock  func() time.Time
	// resolver performs the DNS lookups custom-domain ownership
	// verification depends on.
	resolver Resolver
	// certs issues certificates for verified custom domains through the
	// server's existing ACME manager.
	certs *ssl.Manager
}

// New returns a Service. A nil store or config is a programming error
// rather than a runtime condition, so the caller gets an error at
// startup instead of a panic on the first request.
func New(opts Options) (*Service, error) {
	if opts.Store == nil {
		return nil, errors.New("service: store is required")
	}
	if opts.Config == nil {
		return nil, errors.New("service: config is required")
	}

	clock := opts.Now
	if clock == nil {
		clock = time.Now
	}

	return &Service{
		store:    opts.Store,
		config:   opts.Config,
		cipher:   opts.Cipher,
		clock:    clock,
		resolver: net.DefaultResolver,
	}, nil
}

// encodeDetails renders an audit row's details column.
//
// A value that cannot be marshalled is recorded as the marshalling error
// rather than dropped, so a gap in the trail is visible in the trail
// itself instead of only in a log nobody reads.
func encodeDetails(details map[string]any) string {
	if len(details) == 0 {
		return "{}"
	}
	raw, err := json.Marshal(details)
	if err != nil {
		return fmt.Sprintf("{%q:%q}", "details_error", err.Error())
	}
	return string(raw)
}

// now returns the current UTC time truncated to the second, matching the
// resolution the TIMESTAMP columns store.
func (s *Service) now() time.Time {
	return s.clock().UTC().Truncate(time.Second)
}

// Store exposes the data-access layer for a caller that needs to read
// rows the service does not wrap, such as the scheduler's purge tasks.
func (s *Service) Store() *store.Store {
	return s.store
}

// users returns the PART 34 configuration block.
func (s *Service) users() config.Users {
	return s.config.Server.Users
}

// orgs returns the PART 35 configuration block.
func (s *Service) orgs() config.Orgs {
	return s.config.Server.Orgs
}

// domains returns the PART 36 configuration block.
func (s *Service) domains() config.CustomDomains {
	return s.config.Server.Features.CustomDomains
}

// mapStoreErr translates the store's sentinels into the service's, so a
// handler never has to import both packages to classify a failure.
func mapStoreErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, store.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, store.ErrConflict):
		return ErrConflict
	default:
		return err
	}
}

// validationError wraps a caller-facing message as a validation failure.
func validationError(msg string) error {
	return &fieldError{message: msg}
}

// fieldError carries a human-readable validation message while still
// matching ErrValidation, so a handler can both classify the failure and
// show the caller what was wrong with their input.
type fieldError struct {
	message string
}

// Error returns the caller-facing message.
func (e *fieldError) Error() string {
	return e.message
}

// Is makes every fieldError match ErrValidation.
func (e *fieldError) Is(target error) bool {
	return target == ErrValidation
}
