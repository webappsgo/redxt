package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/database"
	"github.com/webappsgo/redxt/src/security"
	"github.com/webappsgo/redxt/src/server/model"
	"github.com/webappsgo/redxt/src/server/store"
	"github.com/webappsgo/redxt/src/user"
)

// testPassword satisfies the default password policy, so a test that
// means to exercise something else is not turned back at validation.
const testPassword = "Correct-Horse9-Battery"

// stubResolver answers ownership lookups from a fixed table, which is
// what lets the verification flow be exercised without a network.
type stubResolver struct {
	records map[string][]string
	err     error
}

// LookupTXT returns the published values for a name.
func (s stubResolver) LookupTXT(_ context.Context, name string) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	values, ok := s.records[name]
	if !ok {
		return nil, errors.New("no such host")
	}
	return values, nil
}

// newTestService builds a service over a real database with multi-user,
// organizations, and custom domains all switched on.
func newTestService(t *testing.T) *Service {
	t.Helper()

	db, err := database.OpenUsers(config.Database{}, t.TempDir())
	if err != nil {
		t.Fatalf("OpenUsers: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err = database.EnsureUsersSchema(context.Background(), db); err != nil {
		t.Fatalf("EnsureUsersSchema: %v", err)
	}

	key, err := security.GenerateEncryptionKey()
	if err != nil {
		t.Fatalf("GenerateEncryptionKey: %v", err)
	}
	cipher, err := security.NewCipher(key, 1)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Server.Users.Enabled = true
	cfg.Server.Users.Registration.Mode = "open"
	cfg.Server.Users.Registration.RequireEmailVerification = false
	cfg.Server.Orgs.Enabled = true
	cfg.Server.Orgs.Creation.Mode = "open"
	cfg.Server.Features.CustomDomains.Enabled = true

	svc, err := New(Options{Store: store.New(db), Config: cfg, Cipher: cipher})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return svc
}

// newUser registers one account.
func newUser(t *testing.T, svc *Service, username string) model.User {
	t.Helper()

	account, err := svc.Register(context.Background(), RegisterInput{
		Username: username,
		Email:    username + "@example.test",
		Password: testPassword,
	})
	if err != nil {
		t.Fatalf("Register %s: %v", username, err)
	}
	return account
}

// newOrg creates a shared organization owned by the given user.
func newOrg(t *testing.T, svc *Service, ownerID int64, slug string) model.Org {
	t.Helper()

	org, err := svc.CreateOrg(context.Background(), ownerID, CreateOrgInput{
		Slug: slug,
		Name: strings.ToUpper(slug),
	})
	if err != nil {
		t.Fatalf("CreateOrg %s: %v", slug, err)
	}
	return org
}

// addMember puts a second user into an organization at a given role.
func addMember(t *testing.T, svc *Service, org model.Org, ownerID int64, member model.User, role user.Role) {
	t.Helper()

	ctx := context.Background()
	invite, err := svc.InviteMember(ctx, org.ID, ownerID, InviteInput{
		Email: member.Email,
		Role:  string(role),
	})
	if err != nil {
		t.Fatalf("InviteMember: %v", err)
	}
	if _, err = svc.AcceptInvite(ctx, member.ID, invite.Code); err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}
}

// TestRegistrationModesDecideWhoMaySignUp covers the PART 34 mode table,
// including the case where open mode accepts an invite it does not
// require.
func TestRegistrationModesDecideWhoMaySignUp(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		invited bool
		wantErr error
	}{
		{name: "open accepts an unaided signup", mode: "open"},
		{name: "open still accepts an invited signup", mode: "open", invited: true},
		{name: "invite refuses an unaided signup", mode: "invite", wantErr: ErrForbidden},
		{name: "admin_only refuses an unaided signup", mode: "admin_only", wantErr: ErrForbidden},
		{name: "disabled refuses an unaided signup", mode: "disabled", wantErr: ErrForbidden},
		{name: "disabled refuses an issued invite", mode: "disabled", invited: true, wantErr: ErrDisabled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService(t)
			ctx := context.Background()

			// The invite is issued while the mode still permits it, so
			// the case under test is the redemption and not the issuing.
			var code string
			if tt.invited {
				host := newUser(t, svc, "ada")
				org := newOrg(t, svc, host.ID, "acme")
				invite, err := svc.InviteMember(ctx, org.ID, host.ID, InviteInput{
					Email: "grace@example.test",
					Role:  string(user.RoleViewer),
				})
				if err != nil {
					t.Fatalf("InviteMember: %v", err)
				}
				code = invite.Code
			}

			svc.config.Server.Users.Registration.Mode = tt.mode

			_, err := svc.Register(ctx, RegisterInput{
				Username:   "grace",
				Email:      "grace@example.test",
				Password:   testPassword,
				InviteCode: code,
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// TestRepeatedFailuresLockTheAccount is the PART 34 lockout rule, which
// is what stops an online guessing run.
func TestRepeatedFailuresLockTheAccount(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	const limit = 3
	svc.config.Server.Users.Auth.MaxFailedLogins = limit
	account := newUser(t, svc, "ada")

	for i := 0; i < limit; i++ {
		_, err := svc.Login(ctx, LoginInput{Identifier: "ada", Password: "wrong"})
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d: err = %v, want %v", i+1, err, ErrInvalidCredentials)
		}
	}

	// Past the limit the correct password must not open a session
	// either, or the lockout would only delay a guessing run rather
	// than stop it.
	_, err := svc.Login(ctx, LoginInput{Identifier: "ada", Password: testPassword})
	if !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("err = %v, want %v", err, ErrAccountLocked)
	}

	if _, err = svc.User(ctx, account.ID); err != nil {
		t.Fatalf("User: %v", err)
	}
}

// TestSignInIsRefusedTheSameWayWhicheverHalfIsWrong is the PART 11 rule
// against a credential oracle, checked at the service boundary where the
// distinction actually exists.
func TestSignInIsRefusedTheSameWayWhicheverHalfIsWrong(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	newUser(t, svc, "ada")

	_, wrongPassword := svc.Login(ctx, LoginInput{Identifier: "ada", Password: "wrong"})
	_, noSuchUser := svc.Login(ctx, LoginInput{Identifier: "nobody", Password: "wrong"})

	if !errors.Is(wrongPassword, ErrInvalidCredentials) {
		t.Fatalf("wrong password: err = %v, want %v", wrongPassword, ErrInvalidCredentials)
	}
	if !errors.Is(noSuchUser, ErrInvalidCredentials) {
		t.Fatalf("unknown account: err = %v, want %v", noSuchUser, ErrInvalidCredentials)
	}
	if wrongPassword.Error() != noSuchUser.Error() {
		t.Errorf("a wrong password fails with %q and an unknown account with %q, which distinguishes them",
			wrongPassword, noSuchUser)
	}
}

// TestRoleDecidesWhatAMemberMayDo is the PART 35 permission gate,
// exercised through the service rather than the role table alone.
func TestRoleDecidesWhatAMemberMayDo(t *testing.T) {
	tests := []struct {
		name    string
		role    user.Role
		action  func(svc *Service, org model.Org, actorID int64) error
		wantErr bool
	}{
		{
			name: "an editor cannot rename the organization",
			role: user.RoleEditor,
			action: func(svc *Service, org model.Org, actorID int64) error {
				_, err := svc.UpdateOrg(context.Background(), org.ID, actorID,
					CreateOrgInput{Slug: org.Slug, Name: "Renamed"})
				return err
			},
			wantErr: true,
		},
		{
			name: "an editor cannot invite a member",
			role: user.RoleEditor,
			action: func(svc *Service, org model.Org, actorID int64) error {
				_, err := svc.InviteMember(context.Background(), org.ID, actorID,
					InviteInput{Email: "new@example.test", Role: string(user.RoleViewer)})
				return err
			},
			wantErr: true,
		},
		{
			name: "a viewer cannot issue a token",
			role: user.RoleViewer,
			action: func(svc *Service, org model.Org, actorID int64) error {
				_, err := svc.IssueToken(context.Background(), actorID,
					IssueTokenInput{Name: "ci", OrgID: org.ID})
				return err
			},
			wantErr: true,
		},
		{
			name: "a viewer cannot claim a domain",
			role: user.RoleViewer,
			action: func(svc *Service, org model.Org, actorID int64) error {
				_, err := svc.AddDomain(context.Background(), org.ID, actorID,
					AddDomainInput{Domain: "example.test"})
				return err
			},
			wantErr: true,
		},
		{
			name: "an admin may invite a member",
			role: user.RoleAdmin,
			action: func(svc *Service, org model.Org, actorID int64) error {
				_, err := svc.InviteMember(context.Background(), org.ID, actorID,
					InviteInput{Email: "new@example.test", Role: string(user.RoleViewer)})
				return err
			},
		},
		{
			name: "an admin cannot delete the organization",
			role: user.RoleAdmin,
			action: func(svc *Service, org model.Org, actorID int64) error {
				return svc.DeleteOrg(context.Background(), org.ID, actorID)
			},
			wantErr: true,
		},
		{
			name: "a viewer may read the member list",
			role: user.RoleViewer,
			action: func(svc *Service, org model.Org, actorID int64) error {
				_, err := svc.Members(context.Background(), org.ID, actorID)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService(t)
			owner := newUser(t, svc, "ada")
			member := newUser(t, svc, "grace")
			org := newOrg(t, svc, owner.ID, "acme")
			addMember(t, svc, org, owner.ID, member, tt.role)

			err := tt.action(svc, org, member.ID)
			if tt.wantErr {
				if !errors.Is(err, ErrForbidden) {
					t.Fatalf("err = %v, want %v", err, ErrForbidden)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v, want no error", err)
			}
		})
	}
}

// TestAnOutsiderCannotReachAnOrganization is the PART 35 isolation rule
// at the service boundary. A caller outside the organization is told it
// does not exist, because a refusal would confirm that it does.
func TestAnOutsiderCannotReachAnOrganization(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	owner := newUser(t, svc, "ada")
	outsider := newUser(t, svc, "mallory")
	org := newOrg(t, svc, owner.ID, "acme")

	tests := []struct {
		name   string
		action func() error
	}{
		{name: "read the members", action: func() error {
			_, err := svc.Members(ctx, org.ID, outsider.ID)
			return err
		}},
		{name: "read the audit trail", action: func() error {
			_, err := svc.OrgAudit(ctx, org.ID, outsider.ID, 10, 0)
			return err
		}},
		{name: "rename it", action: func() error {
			_, err := svc.UpdateOrg(ctx, org.ID, outsider.ID, CreateOrgInput{Slug: "acme", Name: "Stolen"})
			return err
		}},
		{name: "delete it", action: func() error {
			return svc.DeleteOrg(ctx, org.ID, outsider.ID)
		}},
		{name: "invite to it", action: func() error {
			_, err := svc.InviteMember(ctx, org.ID, outsider.ID, InviteInput{Email: "x@example.test"})
			return err
		}},
		{name: "issue a token for it", action: func() error {
			_, err := svc.IssueToken(ctx, outsider.ID, IssueTokenInput{Name: "ci", OrgID: org.ID})
			return err
		}},
		{name: "claim a domain for it", action: func() error {
			_, err := svc.AddDomain(ctx, org.ID, outsider.ID, AddDomainInput{Domain: "example.test"})
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.action(); !errors.Is(err, ErrNotFound) {
				t.Fatalf("err = %v, want %v", err, ErrNotFound)
			}
		})
	}
}

// TestATokenNeverExceedsItsIssuer is the PART 35 rule that a credential
// cannot be used to escalate: whatever role is asked for, the token
// comes back capped at the issuer's own.
func TestATokenNeverExceedsItsIssuer(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	owner := newUser(t, svc, "ada")
	member := newUser(t, svc, "grace")
	org := newOrg(t, svc, owner.ID, "acme")
	addMember(t, svc, org, owner.ID, member, user.RoleAdmin)

	issued, err := svc.IssueToken(ctx, member.ID, IssueTokenInput{
		Name:  "ci",
		OrgID: org.ID,
		Role:  string(user.RoleOwner),
	})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	if issued.Token.Role == string(user.RoleOwner) {
		t.Fatal("an admin issued a token carrying the owner role")
	}
	if issued.Token.Role != string(user.RoleAdmin) {
		t.Fatalf("role = %q, want %q", issued.Token.Role, string(user.RoleAdmin))
	}

	// The secret is returned once and stored only as a digest, so the
	// stored row must not carry anything that can be presented.
	if issued.Secret == "" {
		t.Fatal("no secret returned")
	}
	if strings.Contains(issued.Token.Hash, issued.Secret) {
		t.Error("the stored row carries the presentable secret")
	}

	auth, err := svc.AuthenticateToken(ctx, issued.Secret)
	if err != nil {
		t.Fatalf("AuthenticateToken: %v", err)
	}
	if auth.Role != user.RoleAdmin {
		t.Errorf("authenticated role = %q, want %q", auth.Role, string(user.RoleAdmin))
	}
	if auth.Token.OrgID != org.ID {
		t.Errorf("token org = %d, want %d", auth.Token.OrgID, org.ID)
	}
}

// TestAViewerCannotIssueAWritingToken covers the other half of the
// scoping rule: the requested scope is refused rather than quietly
// widened past what the role can do.
func TestAViewerCannotIssueAWritingToken(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	owner := newUser(t, svc, "ada")
	member := newUser(t, svc, "grace")
	org := newOrg(t, svc, owner.ID, "acme")
	addMember(t, svc, org, owner.ID, member, user.RoleViewer)

	_, err := svc.IssueToken(ctx, member.ID, IssueTokenInput{
		Name:  "ci",
		OrgID: org.ID,
		Scope: string(security.ScopeReadWrite),
	})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v, want %v", err, ErrForbidden)
	}
}

// TestATokenIsRefusedOnceRevoked covers the lifecycle: a credential that
// was withdrawn must stop working immediately.
func TestATokenIsRefusedOnceRevoked(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	owner := newUser(t, svc, "ada")
	org := newOrg(t, svc, owner.ID, "acme")

	issued, err := svc.IssueToken(ctx, owner.ID, IssueTokenInput{Name: "ci", OrgID: org.ID})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	if err = svc.RevokeToken(ctx, owner.ID, issued.Token.ID); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}
	if _, err = svc.AuthenticateToken(ctx, issued.Secret); err == nil {
		t.Fatal("a revoked token still authenticates")
	}
}

// TestGarbageIsNotAToken covers the invalid-input path on the credential
// check, which must fail rather than fall through to a match.
func TestGarbageIsNotAToken(t *testing.T) {
	svc := newTestService(t)

	for _, presented := range []string{"", "   ", "not-a-token", strings.Repeat("a", 512)} {
		if _, err := svc.AuthenticateToken(context.Background(), presented); err == nil {
			t.Errorf("AuthenticateToken(%q) succeeded", presented)
		}
	}
}

// TestADomainIsNotServedUntilItIsVerified is the PART 36 rule that
// ownership is proven before a custom domain is activated. Skipping it
// would let one organization claim a name another controls.
func TestADomainIsNotServedUntilItIsVerified(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	owner := newUser(t, svc, "ada")
	org := newOrg(t, svc, owner.ID, "acme")

	domain, err := svc.AddDomain(ctx, org.ID, owner.ID, AddDomainInput{Domain: "dns.example.test"})
	if err != nil {
		t.Fatalf("AddDomain: %v", err)
	}
	if domain.Verified() {
		t.Fatal("a freshly claimed domain is already verified")
	}
	if domain.VerificationToken == "" {
		t.Fatal("no verification token was issued")
	}
	if _, err = svc.ResolveServableDomain(ctx, domain.Name); err == nil {
		t.Fatal("an unverified domain is already being served")
	}

	// With nothing published the check must fail and leave the domain
	// where it was.
	svc.SetResolver(stubResolver{records: map[string][]string{}})
	if _, err = svc.VerifyDomain(ctx, org.ID, owner.ID, domain.ID); err == nil {
		t.Fatal("verification passed with no record published")
	}
	if _, err = svc.ResolveServableDomain(ctx, domain.Name); err == nil {
		t.Fatal("a failed verification activated the domain")
	}

	// A record carrying the wrong value must not pass either.
	svc.SetResolver(stubResolver{records: map[string][]string{
		VerificationPrefix + domain.Name: {"some-other-value"},
	}})
	if _, err = svc.VerifyDomain(ctx, org.ID, owner.ID, domain.ID); err == nil {
		t.Fatal("verification passed on a mismatched record")
	}

	// The published token activates it.
	svc.SetResolver(stubResolver{records: map[string][]string{
		VerificationPrefix + domain.Name: {domain.VerificationToken},
	}})
	verified, err := svc.VerifyDomain(ctx, org.ID, owner.ID, domain.ID)
	if err != nil {
		t.Fatalf("VerifyDomain: %v", err)
	}
	if !verified.Verified() {
		t.Fatal("a domain with its record published is still unverified")
	}
}

// TestAnotherOrganizationCannotVerifyTheDomain closes the path where the
// ownership proof is satisfied but the caller is not entitled to the
// claim in the first place.
func TestAnotherOrganizationCannotVerifyTheDomain(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	owner := newUser(t, svc, "ada")
	outsider := newUser(t, svc, "mallory")
	org := newOrg(t, svc, owner.ID, "acme")
	other := newOrg(t, svc, outsider.ID, "rival")

	domain, err := svc.AddDomain(ctx, org.ID, owner.ID, AddDomainInput{Domain: "dns.example.test"})
	if err != nil {
		t.Fatalf("AddDomain: %v", err)
	}
	svc.SetResolver(stubResolver{records: map[string][]string{
		VerificationPrefix + domain.Name: {domain.VerificationToken},
	}})

	if _, err = svc.VerifyDomain(ctx, org.ID, outsider.ID, domain.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("outsider on the owning org: err = %v, want %v", err, ErrNotFound)
	}
	if _, err = svc.VerifyDomain(ctx, other.ID, outsider.ID, domain.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("domain addressed through another org: err = %v, want %v", err, ErrNotFound)
	}
}

// TestPasswordChangeRequiresTheCurrentOne stops a borrowed session from
// being turned into permanent control of the account.
func TestPasswordChangeRequiresTheCurrentOne(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	account := newUser(t, svc, "ada")

	if err := svc.ChangePassword(ctx, account.ID, "wrong", "Another-Pass9-Here"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("err = %v, want %v", err, ErrInvalidCredentials)
	}
	if err := svc.ChangePassword(ctx, account.ID, testPassword, "short"); !errors.Is(err, ErrValidation) {
		t.Fatalf("weak replacement: err = %v, want %v", err, ErrValidation)
	}
	if err := svc.ChangePassword(ctx, account.ID, testPassword, "Another-Pass9-Here"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if _, err := svc.Login(ctx, LoginInput{Identifier: "ada", Password: testPassword}); !errors.Is(err, ErrInvalidCredentials) {
		t.Error("the old password still signs in")
	}
	if _, err := svc.Login(ctx, LoginInput{Identifier: "ada", Password: "Another-Pass9-Here"}); err != nil {
		t.Errorf("the new password does not sign in: %v", err)
	}
}

// TestEveryUserGetsAPersonalOrganization is the PART 35 rule that gives
// a new account somewhere to put a zone, since nothing attaches to a
// user directly.
func TestEveryUserGetsAPersonalOrganization(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	account := newUser(t, svc, "ada")

	orgs, err := svc.OrgsForUser(ctx, account.ID)
	if err != nil {
		t.Fatalf("OrgsForUser: %v", err)
	}
	if len(orgs) != 1 {
		t.Fatalf("orgs = %d, want 1", len(orgs))
	}

	access, err := svc.Access(ctx, orgs[0].ID, account.ID)
	if err != nil {
		t.Fatalf("Access: %v", err)
	}
	if access.Role != user.RoleOwner {
		t.Errorf("role = %q, want %q", access.Role, string(user.RoleOwner))
	}
}

// TestAPrivateProfileIsHiddenFromStrangers covers the PART 34 privacy
// setting the public vanity URL has to respect.
func TestAPrivateProfileIsHiddenFromStrangers(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	account := newUser(t, svc, "ada")
	stranger := newUser(t, svc, "grace")

	if _, err := svc.UpdateProfile(ctx, account.ID, ProfileInput{
		DisplayName: "Ada",
		Visibility:  model.VisibilityPrivate,
	}); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}

	if _, err := svc.UserProfile(ctx, "ada", 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("anonymous viewer: err = %v, want %v", err, ErrNotFound)
	}
	if _, err := svc.UserProfile(ctx, "ada", stranger.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("signed-in stranger: err = %v, want %v", err, ErrNotFound)
	}
	if _, err := svc.UserProfile(ctx, "ada", account.ID); err != nil {
		t.Errorf("the owner cannot see their own profile: %v", err)
	}
}
