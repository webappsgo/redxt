package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/database"
	"github.com/webappsgo/redxt/src/server/model"
	"github.com/webappsgo/redxt/src/user"
)

// newTestStore opens a real users.db and converges the schema onto it,
// so the constraints under test are the ones the running server gets
// rather than an approximation of them.
func newTestStore(t *testing.T) *Store {
	t.Helper()

	db, err := database.OpenUsers(config.Database{}, t.TempDir())
	if err != nil {
		t.Fatalf("OpenUsers: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err = database.EnsureUsersSchema(context.Background(), db); err != nil {
		t.Fatalf("EnsureUsersSchema: %v", err)
	}
	return New(db)
}

// makeUser inserts one account row.
func makeUser(t *testing.T, s *Store, username, email string) model.User {
	t.Helper()

	account, err := s.CreateUser(context.Background(), model.User{
		Username:     username,
		Email:        email,
		PasswordHash: "hash-" + username,
		DisplayName:  username,
		Role:         string(user.RoleViewer),
		Visibility:   model.VisibilityPrivate,
		Status:       model.StatusActive,
	})
	if err != nil {
		t.Fatalf("CreateUser %s: %v", username, err)
	}
	return account
}

// makeOrg inserts one organization row.
func makeOrg(t *testing.T, s *Store, slug string, ownerID int64) model.Org {
	t.Helper()

	org, err := s.CreateOrg(context.Background(), model.Org{
		Slug:       slug,
		Name:       slug,
		OwnerID:    ownerID,
		Visibility: model.VisibilityPrivate,
	})
	if err != nil {
		t.Fatalf("CreateOrg %s: %v", slug, err)
	}
	return org
}

// TestAnAccountIsUniqueByNameAndAddress covers the uniqueness the schema
// has to enforce: two accounts sharing a name or an address would make
// the sign-in identifier ambiguous.
func TestAnAccountIsUniqueByNameAndAddress(t *testing.T) {
	s := newTestStore(t)
	makeUser(t, s, "ada", "ada@example.test")

	tests := []struct {
		name     string
		username string
		email    string
	}{
		{name: "same username", username: "ada", email: "other@example.test"},
		{name: "same email", username: "other", email: "ada@example.test"},
		{name: "both the same", username: "ada", email: "ada@example.test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.CreateUser(context.Background(), model.User{
				Username:     tt.username,
				Email:        tt.email,
				PasswordHash: "hash",
				Role:         string(user.RoleViewer),
				Visibility:   model.VisibilityPrivate,
				Status:       model.StatusActive,
			})
			if !errors.Is(err, ErrConflict) {
				t.Fatalf("err = %v, want %v", err, ErrConflict)
			}
		})
	}
}

// TestAMissingRowIsReportedAsNotFound covers the lookup path every
// caller branches on.
func TestAMissingRowIsReportedAsNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tests := []struct {
		name   string
		lookup func() error
	}{
		{name: "user by id", lookup: func() error { _, err := s.UserByID(ctx, 9999); return err }},
		{name: "user by name", lookup: func() error { _, err := s.UserByUsername(ctx, "nobody"); return err }},
		{name: "user by email", lookup: func() error { _, err := s.UserByEmail(ctx, "no@example.test"); return err }},
		{name: "org by id", lookup: func() error { _, err := s.OrgByID(ctx, 9999); return err }},
		{name: "org by slug", lookup: func() error { _, err := s.OrgBySlug(ctx, "nowhere"); return err }},
		{name: "token by hash", lookup: func() error { _, err := s.TokenByHash(ctx, "nothing"); return err }},
		{name: "session by hash", lookup: func() error { _, err := s.SessionByHash(ctx, "nothing"); return err }},
		{name: "domain by name", lookup: func() error { _, err := s.DomainByName(ctx, "nowhere.test"); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.lookup(); !errors.Is(err, ErrNotFound) {
				t.Fatalf("err = %v, want %v", err, ErrNotFound)
			}
		})
	}
}

// TestMembershipIsScopedToOneOrganization is the storage half of the
// PART 35 isolation rule: a membership row in one organization must not
// answer for another.
func TestMembershipIsScopedToOneOrganization(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	owner := makeUser(t, s, "ada", "ada@example.test")
	member := makeUser(t, s, "grace", "grace@example.test")
	acme := makeOrg(t, s, "acme", owner.ID)
	rival := makeOrg(t, s, "rival", owner.ID)

	if err := s.AddMember(ctx, acme.ID, member.ID, string(user.RoleEditor)); err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	got, err := s.Membership(ctx, acme.ID, member.ID)
	if err != nil {
		t.Fatalf("Membership: %v", err)
	}
	if got.Role != string(user.RoleEditor) {
		t.Errorf("role = %q, want %q", got.Role, string(user.RoleEditor))
	}

	if _, err = s.Membership(ctx, rival.ID, member.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("membership leaked into another org: err = %v, want %v", err, ErrNotFound)
	}

	// A grant made in one organization must not carry into another.
	if err = s.GrantZone(ctx, acme.ID, member.ID, 7, "write"); err != nil {
		t.Fatalf("GrantZone: %v", err)
	}
	granted, err := s.ZoneGranted(ctx, rival.ID, member.ID, 7)
	if err != nil {
		t.Fatalf("ZoneGranted: %v", err)
	}
	if granted {
		t.Error("a zone grant in one organization applies in another")
	}
}

// TestRemovingAMemberWithdrawsTheirOrgTokens closes the path where a
// removed member keeps acting through a credential the organization can
// no longer see.
func TestRemovingAMemberWithdrawsTheirOrgTokens(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	owner := makeUser(t, s, "ada", "ada@example.test")
	member := makeUser(t, s, "grace", "grace@example.test")
	org := makeOrg(t, s, "acme", owner.ID)

	if err := s.AddMember(ctx, org.ID, member.ID, string(user.RoleEditor)); err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	token, err := s.CreateToken(ctx, model.Token{
		Name:      "ci",
		Hash:      "hash-ci",
		OwnerType: model.ActorUser,
		OwnerID:   member.ID,
		OrgID:     org.ID,
		Role:      string(user.RoleEditor),
		Scope:     "read",
	})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	if err = s.RevokeOrgTokensForUser(ctx, org.ID, member.ID); err != nil {
		t.Fatalf("RevokeOrgTokensForUser: %v", err)
	}

	after, err := s.TokenByID(ctx, token.ID)
	if err != nil {
		t.Fatalf("TokenByID: %v", err)
	}
	if after.Usable(time.Now().UTC()) {
		t.Error("the token survived the member's removal")
	}
}

// TestExpiredSessionsArePurged covers the scheduled cleanup, without
// which a stolen session token would stay usable indefinitely.
func TestExpiredSessionsArePurged(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	account := makeUser(t, s, "ada", "ada@example.test")
	now := time.Now().UTC()

	stale, err := s.CreateSession(ctx, model.Session{
		UserID:    account.ID,
		Hash:      "stale",
		ExpiresAt: now.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateSession stale: %v", err)
	}
	if _, err = s.CreateSession(ctx, model.Session{
		UserID:    account.ID,
		Hash:      "live",
		ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("CreateSession live: %v", err)
	}

	purged, err := s.PurgeExpiredSessions(ctx, now)
	if err != nil {
		t.Fatalf("PurgeExpiredSessions: %v", err)
	}
	if purged != 1 {
		t.Fatalf("purged = %d, want 1", purged)
	}
	if _, err = s.SessionByHash(ctx, "stale"); !errors.Is(err, ErrNotFound) {
		t.Errorf("the expired session survived: err = %v", err)
	}
	if _, err = s.SessionByHash(ctx, "live"); err != nil {
		t.Errorf("the live session was purged: %v", err)
	}
	if stale.ID == 0 {
		t.Error("the stale session was never assigned an id")
	}
}

// TestFailuresAccumulateUntilTheAccountLocks covers the counter the
// lockout rule is built on.
func TestFailuresAccumulateUntilTheAccountLocks(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	account := makeUser(t, s, "ada", "ada@example.test")
	const max = 3

	for i := 1; i <= max; i++ {
		if err := s.RecordLoginFailure(ctx, account.ID, max, time.Hour); err != nil {
			t.Fatalf("attempt %d: %v", i, err)
		}
		reloaded, err := s.UserByID(ctx, account.ID)
		if err != nil {
			t.Fatalf("UserByID: %v", err)
		}
		locked := reloaded.Locked(time.Now().UTC())
		if i < max && locked {
			t.Fatalf("locked after %d of %d failures", i, max)
		}
		if i == max && !locked {
			t.Fatalf("still unlocked after %d failures", i)
		}
	}
}

// TestADomainIsNotServableUntilActivated is the storage half of the
// PART 36 rule: an unverified claim must never appear in the set of
// names the server will answer for.
func TestADomainIsNotServableUntilActivated(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	owner := makeUser(t, s, "ada", "ada@example.test")
	org := makeOrg(t, s, "acme", owner.ID)

	domain, err := s.CreateDomain(ctx, model.Domain{
		OrgID:             org.ID,
		Name:              "dns.example.test",
		Purpose:           model.PurposeUI,
		VerificationToken: "token-value",
	})
	if err != nil {
		t.Fatalf("CreateDomain: %v", err)
	}

	servable, err := s.ListServableDomains(ctx)
	if err != nil {
		t.Fatalf("ListServableDomains: %v", err)
	}
	if len(servable) != 0 {
		t.Fatalf("servable = %d, want 0 before verification", len(servable))
	}

	if err = s.RecordVerificationAttempt(ctx, domain.ID, model.VerificationVerified); err != nil {
		t.Fatalf("RecordVerificationAttempt: %v", err)
	}
	if err = s.ActivateDomain(ctx, domain.ID); err != nil {
		t.Fatalf("ActivateDomain: %v", err)
	}

	servable, err = s.ListServableDomains(ctx)
	if err != nil {
		t.Fatalf("ListServableDomains: %v", err)
	}
	if len(servable) != 1 {
		t.Fatalf("servable = %d, want 1 after verification", len(servable))
	}

	// Suspending it takes the name back out of service without
	// discarding the proof that was already given.
	if err = s.SuspendDomain(ctx, domain.ID, "operator request"); err != nil {
		t.Fatalf("SuspendDomain: %v", err)
	}
	servable, err = s.ListServableDomains(ctx)
	if err != nil {
		t.Fatalf("ListServableDomains: %v", err)
	}
	if len(servable) != 0 {
		t.Fatalf("servable = %d, want 0 after suspension", len(servable))
	}
}

// TestTheSameNameCannotBeClaimedTwice stops two organizations from
// holding the same custom domain, which would make the name ambiguous
// at request time.
func TestTheSameNameCannotBeClaimedTwice(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	owner := makeUser(t, s, "ada", "ada@example.test")
	acme := makeOrg(t, s, "acme", owner.ID)
	rival := makeOrg(t, s, "rival", owner.ID)

	claim := model.Domain{Name: "dns.example.test", Purpose: model.PurposeUI, VerificationToken: "token"}

	claim.OrgID = acme.ID
	if _, err := s.CreateDomain(ctx, claim); err != nil {
		t.Fatalf("CreateDomain: %v", err)
	}

	claim.OrgID = rival.ID
	if _, err := s.CreateDomain(ctx, claim); !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v, want %v", err, ErrConflict)
	}
}

// TestAnInviteIsSpentOnceItIsRedeemed covers the single-use guarantee,
// without which one code would onboard an unbounded number of accounts.
func TestAnInviteIsSpentOnceItIsRedeemed(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	owner := makeUser(t, s, "ada", "ada@example.test")
	org := makeOrg(t, s, "acme", owner.ID)

	invite, err := s.CreateInvite(ctx, model.Invite{
		OrgID:     org.ID,
		Email:     "grace@example.test",
		Role:      string(user.RoleViewer),
		CodeHash:  "hash-code",
		MaxUses:   1,
		InvitedBy: owner.ID,
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	if err = s.RedeemInvite(ctx, invite.ID); err != nil {
		t.Fatalf("RedeemInvite: %v", err)
	}

	spent, err := s.InviteByHash(ctx, "hash-code")
	if err != nil {
		t.Fatalf("InviteByHash: %v", err)
	}
	if spent.UseCount != 1 {
		t.Errorf("uses = %d, want 1", spent.UseCount)
	}
	if spent.Redeemable(time.Now().UTC()) {
		t.Error("a spent single-use invite is still usable")
	}
}
