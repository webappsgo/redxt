package admin

import (
	"context"
	"errors"
	"testing"

	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/database"
	"github.com/webappsgo/redxt/src/security"
)

// newTestService opens real server.db and users.db handles and
// converges both schemas, so the constraints under test are the ones
// the running server gets rather than an approximation of them.
func newTestService(t *testing.T) (*Service, *database.DB) {
	t.Helper()
	ctx := context.Background()

	usersDB, err := database.OpenUsers(config.Database{}, t.TempDir())
	if err != nil {
		t.Fatalf("OpenUsers: %v", err)
	}
	t.Cleanup(func() { _ = usersDB.Close() })
	if err := database.EnsureUsersSchema(ctx, usersDB); err != nil {
		t.Fatalf("EnsureUsersSchema: %v", err)
	}

	serverDB, err := database.OpenServer(config.Database{}, t.TempDir())
	if err != nil {
		t.Fatalf("OpenServer: %v", err)
	}
	t.Cleanup(func() { _ = serverDB.Close() })
	if err := database.EnsureServerSchema(ctx, serverDB); err != nil {
		t.Fatalf("EnsureServerSchema: %v", err)
	}

	return NewService(usersDB, serverDB), serverDB
}

// seedSetupToken mints a setup token and stores its hash in server.db
// the same way startup.go's ensureSetupToken does, returning the
// plaintext token for the test to submit.
func seedSetupToken(t *testing.T, serverDB *database.DB) string {
	t.Helper()
	token, err := security.GenerateSetupToken()
	if err != nil {
		t.Fatalf("GenerateSetupToken: %v", err)
	}
	_, _, err = database.EnsureSecret(context.Background(), serverDB, security.SecretSetupToken, func() (string, error) {
		return security.HashToken(token), nil
	})
	if err != nil {
		t.Fatalf("EnsureSecret: %v", err)
	}
	return token
}

func TestNeedsSetup(t *testing.T) {
	svc, serverDB := newTestService(t)
	ctx := context.Background()

	needs, err := svc.NeedsSetup(ctx)
	if err != nil {
		t.Fatalf("NeedsSetup: %v", err)
	}
	if !needs {
		t.Fatal("NeedsSetup = false, want true before any admin exists")
	}

	token := seedSetupToken(t, serverDB)
	if _, err := svc.CompleteSetup(ctx, token, "root", "root@example.test", "correct horse battery staple"); err != nil {
		t.Fatalf("CompleteSetup: %v", err)
	}

	needs, err = svc.NeedsSetup(ctx)
	if err != nil {
		t.Fatalf("NeedsSetup after setup: %v", err)
	}
	if needs {
		t.Fatal("NeedsSetup = true, want false once an admin exists")
	}
}

func TestCompleteSetupWrongToken(t *testing.T) {
	svc, serverDB := newTestService(t)
	ctx := context.Background()
	seedSetupToken(t, serverDB)

	_, err := svc.CompleteSetup(ctx, "not-the-real-token", "root", "root@example.test", "correct horse battery staple")
	if !errors.Is(err, ErrInvalidSetupToken) {
		t.Fatalf("CompleteSetup error = %v, want ErrInvalidSetupToken", err)
	}
}

func TestCompleteSetupAlreadyDone(t *testing.T) {
	svc, serverDB := newTestService(t)
	ctx := context.Background()
	token := seedSetupToken(t, serverDB)

	if _, err := svc.CompleteSetup(ctx, token, "root", "root@example.test", "correct horse battery staple"); err != nil {
		t.Fatalf("first CompleteSetup: %v", err)
	}
	_, err := svc.CompleteSetup(ctx, token, "second", "second@example.test", "correct horse battery staple")
	if !errors.Is(err, ErrSetupComplete) {
		t.Fatalf("second CompleteSetup error = %v, want ErrSetupComplete", err)
	}
}

func TestLoginSuccessAndFailure(t *testing.T) {
	svc, serverDB := newTestService(t)
	ctx := context.Background()
	token := seedSetupToken(t, serverDB)

	if _, err := svc.CompleteSetup(ctx, token, "root", "root@example.test", "correct horse battery staple"); err != nil {
		t.Fatalf("CompleteSetup: %v", err)
	}

	sessionToken, found, err := svc.Login(ctx, "root", "correct horse battery staple", "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if sessionToken == "" {
		t.Fatal("Login: expected a non-empty session token")
	}
	if found.Username != "root" {
		t.Fatalf("Login admin = %q, want %q", found.Username, "root")
	}

	current, err := svc.CurrentAdmin(ctx, sessionToken)
	if err != nil {
		t.Fatalf("CurrentAdmin: %v", err)
	}
	if current.ID != found.ID {
		t.Fatalf("CurrentAdmin id = %d, want %d", current.ID, found.ID)
	}

	if err := svc.Logout(ctx, sessionToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := svc.CurrentAdmin(ctx, sessionToken); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("CurrentAdmin after logout = %v, want ErrInvalidCredentials", err)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	svc, serverDB := newTestService(t)
	ctx := context.Background()
	token := seedSetupToken(t, serverDB)

	if _, err := svc.CompleteSetup(ctx, token, "root", "root@example.test", "correct horse battery staple"); err != nil {
		t.Fatalf("CompleteSetup: %v", err)
	}

	_, _, err := svc.Login(ctx, "root", "wrong password", "127.0.0.1", "test-agent")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login wrong password error = %v, want ErrInvalidCredentials", err)
	}
}

func TestLoginUnknownUsername(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_, _, err := svc.Login(ctx, "nobody", "irrelevant", "127.0.0.1", "test-agent")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login unknown username error = %v, want ErrInvalidCredentials", err)
	}
}
