package admin

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/webappsgo/redxt/src/config"
	"github.com/webappsgo/redxt/src/database"
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
	return NewStore(db)
}

func TestCreateAndReadAdmin(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	created, err := s.CreateAdmin(ctx, Admin{
		Username:     "root",
		Email:        "root@example.test",
		PasswordHash: "hash-root",
	})
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("CreateAdmin: expected a non-zero id")
	}

	byID, err := s.AdminByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("AdminByID: %v", err)
	}
	if byID.Username != "root" {
		t.Fatalf("AdminByID username = %q, want %q", byID.Username, "root")
	}

	byName, err := s.AdminByUsername(ctx, "root")
	if err != nil {
		t.Fatalf("AdminByUsername: %v", err)
	}
	if byName.ID != created.ID {
		t.Fatalf("AdminByUsername id = %d, want %d", byName.ID, created.ID)
	}
}

func TestCreateAdminDuplicateUsername(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.CreateAdmin(ctx, Admin{Username: "root", Email: "a@example.test", PasswordHash: "h"}); err != nil {
		t.Fatalf("first CreateAdmin: %v", err)
	}
	_, err := s.CreateAdmin(ctx, Admin{Username: "root", Email: "b@example.test", PasswordHash: "h"})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate CreateAdmin error = %v, want ErrConflict", err)
	}
}

func TestAdminByIDNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.AdminByID(context.Background(), 12345)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("AdminByID error = %v, want ErrNotFound", err)
	}
}

func TestCountAdmins(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	n, err := s.CountAdmins(ctx)
	if err != nil {
		t.Fatalf("CountAdmins: %v", err)
	}
	if n != 0 {
		t.Fatalf("CountAdmins = %d, want 0 before any admin exists", n)
	}

	if _, err := s.CreateAdmin(ctx, Admin{Username: "root", Email: "a@example.test", PasswordHash: "h"}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}

	n, err = s.CountAdmins(ctx)
	if err != nil {
		t.Fatalf("CountAdmins: %v", err)
	}
	if n != 1 {
		t.Fatalf("CountAdmins = %d, want 1 after creating an admin", n)
	}
}

func TestRecordLoginSuccess(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	created, err := s.CreateAdmin(ctx, Admin{Username: "root", Email: "a@example.test", PasswordHash: "h"})
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	if !created.LastLoginAt.IsZero() {
		t.Fatal("expected a fresh admin to have no last login time")
	}

	if err := s.RecordLoginSuccess(ctx, created.ID); err != nil {
		t.Fatalf("RecordLoginSuccess: %v", err)
	}

	got, err := s.AdminByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("AdminByID: %v", err)
	}
	if got.LastLoginAt.IsZero() {
		t.Fatal("expected LastLoginAt to be set after RecordLoginSuccess")
	}
}

func TestRecordLoginSuccessNotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.RecordLoginSuccess(context.Background(), 99999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("RecordLoginSuccess error = %v, want ErrNotFound", err)
	}
}

func TestSessionLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	admin, err := s.CreateAdmin(ctx, Admin{Username: "root", Email: "a@example.test", PasswordHash: "h"})
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}

	now := time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)
	created, err := s.CreateSession(ctx, Session{
		AdminID:    admin.ID,
		Hash:       "session-hash-1",
		IP:         "127.0.0.1",
		UserAgent:  "test-agent",
		CreatedAt:  now,
		LastActive: now,
		ExpiresAt:  now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("CreateSession: expected a non-zero id")
	}

	byHash, err := s.SessionByHash(ctx, "session-hash-1")
	if err != nil {
		t.Fatalf("SessionByHash: %v", err)
	}
	if byHash.AdminID != admin.ID {
		t.Fatalf("SessionByHash admin id = %d, want %d", byHash.AdminID, admin.ID)
	}

	if err := s.TouchSession(ctx, "session-hash-1"); err != nil {
		t.Fatalf("TouchSession: %v", err)
	}

	if err := s.DeleteSession(ctx, "session-hash-1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := s.SessionByHash(ctx, "session-hash-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SessionByHash after delete = %v, want ErrNotFound", err)
	}
}

func TestPurgeExpiredSessions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	admin, err := s.CreateAdmin(ctx, Admin{Username: "root", Email: "a@example.test", PasswordHash: "h"})
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}

	cutoff := time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		expiresAt time.Time
	}{
		{name: "expires before cutoff", expiresAt: cutoff.Add(-time.Hour)},
		{name: "expires exactly at cutoff", expiresAt: cutoff},
	}
	for i, tt := range tests {
		_, err := s.CreateSession(ctx, Session{
			AdminID:    admin.ID,
			Hash:       "expired-" + tt.name + "-" + time.Duration(i).String(),
			CreatedAt:  cutoff.Add(-2 * time.Hour),
			LastActive: cutoff.Add(-2 * time.Hour),
			ExpiresAt:  tt.expiresAt,
		})
		if err != nil {
			t.Fatalf("CreateSession %s: %v", tt.name, err)
		}
	}

	live, err := s.CreateSession(ctx, Session{
		AdminID:    admin.ID,
		Hash:       "still-live",
		CreatedAt:  cutoff,
		LastActive: cutoff,
		ExpiresAt:  cutoff.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateSession live: %v", err)
	}

	n, err := s.PurgeExpiredSessions(ctx, cutoff)
	if err != nil {
		t.Fatalf("PurgeExpiredSessions: %v", err)
	}
	if n != 2 {
		t.Fatalf("PurgeExpiredSessions removed %d rows, want 2", n)
	}

	if _, err := s.SessionByHash(ctx, live.Hash); err != nil {
		t.Fatalf("SessionByHash live after purge: %v", err)
	}
}
