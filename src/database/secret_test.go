package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// constantSecret returns a generator that always produces value, so a test can
// tell which candidate won a race.
func constantSecret(value string) func() (string, error) {
	return func() (string, error) { return value, nil }
}

func TestGetSecretMissing(t *testing.T) {
	db := openServerDB(t)

	_, _, _, err := GetSecret(context.Background(), db, "installation_secret")
	if !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("err = %v, want ErrSecretNotFound", err)
	}
}

func TestEnsureSecretGeneratesOnce(t *testing.T) {
	db := openServerDB(t)
	ctx := context.Background()

	calls := 0
	generate := func() (string, error) {
		calls++
		return fmt.Sprintf("secret-%d", calls), nil
	}

	value, version, err := EnsureSecret(ctx, db, "installation_secret", generate)
	if err != nil {
		t.Fatalf("EnsureSecret: %v", err)
	}
	if value != "secret-1" || version != 1 {
		t.Fatalf("got %q v%d, want secret-1 v1", value, version)
	}

	// A later call must return the stored value and never generate again.
	value2, version2, err := EnsureSecret(ctx, db, "installation_secret", generate)
	if err != nil {
		t.Fatalf("EnsureSecret 2: %v", err)
	}
	if value2 != value || version2 != version {
		t.Errorf("second call returned %q v%d, want %q v%d", value2, version2, value, version)
	}
	if calls != 1 {
		t.Errorf("generator called %d times, want 1", calls)
	}
}

// TestEnsureSecretConvergesUnderRace is the documented first-start race: many
// goroutines standing in for cluster nodes each generate a different candidate,
// and every one of them must come back holding the SAME stored value. If any
// caller returned its own losing candidate, the cookies it signed would fail
// to validate everywhere else.
func TestEnsureSecretConvergesUnderRace(t *testing.T) {
	db := openServerDB(t)
	ctx := context.Background()

	const goroutines = 8
	results := make([]string, goroutines)
	errs := make([]error, goroutines)

	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			v, _, err := EnsureSecret(ctx, db, "cookie_signing_key", constantSecret(fmt.Sprintf("candidate-%d", i)))
			results[i] = v
			errs[i] = err
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
	}
	for i, v := range results {
		if v != results[0] {
			t.Errorf("goroutine %d got %q, goroutine 0 got %q: nodes diverged", i, v, results[0])
		}
	}

	stored, version, _, err := GetSecret(ctx, db, "cookie_signing_key")
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if stored != results[0] {
		t.Errorf("stored %q, callers hold %q", stored, results[0])
	}
	if version != 1 {
		t.Errorf("version = %d, want 1", version)
	}

	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM app_secrets WHERE name = ?`, "cookie_signing_key").Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 1 {
		t.Errorf("stored %d rows, want exactly 1", rows)
	}
}

func TestEnsureSecretGeneratorError(t *testing.T) {
	db := openServerDB(t)
	ctx := context.Background()

	sentinel := errors.New("no entropy")
	_, _, err := EnsureSecret(ctx, db, "csrf_token_secret", func() (string, error) {
		return "", sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want the generator error", err)
	}
}

func TestEnsureSecretRejectsEmpty(t *testing.T) {
	db := openServerDB(t)
	ctx := context.Background()

	_, _, err := EnsureSecret(ctx, db, "csrf_token_secret", constantSecret(""))
	if !errors.Is(err, ErrEmptySecret) {
		t.Errorf("err = %v, want ErrEmptySecret", err)
	}
}

func TestRotateSecret(t *testing.T) {
	db := openServerDB(t)
	ctx := context.Background()

	if _, _, err := EnsureSecret(ctx, db, "cookie_signing_key", constantSecret("v1-value")); err != nil {
		t.Fatalf("EnsureSecret: %v", err)
	}
	if err := RotateSecret(ctx, db, "cookie_signing_key", constantSecret("v2-value"), time.Hour); err != nil {
		t.Fatalf("RotateSecret: %v", err)
	}

	value, version, expires, err := GetSecret(ctx, db, "cookie_signing_key")
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if value != "v2-value" || version != 2 {
		t.Errorf("current = %q v%d, want v2-value v2", value, version)
	}
	if expires.Valid {
		t.Errorf("current version carries an expiry: %v", expires)
	}

	// The previous version must survive the window so in-flight signatures
	// still validate.
	var oldValue string
	var oldExpires any
	err = db.QueryRow(
		`SELECT value, expires_at FROM app_secrets WHERE name = ? AND version = ?`,
		"cookie_signing_key", 1).Scan(&oldValue, &oldExpires)
	if err != nil {
		t.Fatalf("read previous version: %v", err)
	}
	if oldValue != "v1-value" {
		t.Errorf("previous value = %q, want v1-value", oldValue)
	}
	if parsed := toTime(oldExpires); !parsed.After(time.Now()) {
		t.Errorf("previous version expiry %v is not in the future", parsed)
	}
}

func TestRotateSecretMissing(t *testing.T) {
	db := openServerDB(t)
	err := RotateSecret(context.Background(), db, "cookie_signing_key", constantSecret("x"), time.Hour)
	if !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("err = %v, want ErrSecretNotFound", err)
	}
}

func TestRotateSecretRepeatedly(t *testing.T) {
	db := openServerDB(t)
	ctx := context.Background()

	if _, _, err := EnsureSecret(ctx, db, "installation_secret", constantSecret("v1")); err != nil {
		t.Fatalf("EnsureSecret: %v", err)
	}
	for i := 2; i <= 4; i++ {
		if err := RotateSecret(ctx, db, "installation_secret", constantSecret(fmt.Sprintf("v%d", i)), time.Hour); err != nil {
			t.Fatalf("RotateSecret to v%d: %v", i, err)
		}
		_, version, _, err := GetSecret(ctx, db, "installation_secret")
		if err != nil {
			t.Fatalf("GetSecret: %v", err)
		}
		if version != i {
			t.Errorf("version = %d, want %d", version, i)
		}
	}

	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM app_secrets WHERE name = ?`, "installation_secret").Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 4 {
		t.Errorf("kept %d versions, want all 4 inside their windows", rows)
	}
}

func TestPruneSecrets(t *testing.T) {
	db := openServerDB(t)
	ctx := context.Background()

	if _, _, err := EnsureSecret(ctx, db, "csrf_token_secret", constantSecret("v1")); err != nil {
		t.Fatalf("EnsureSecret: %v", err)
	}
	// A grace of zero closes the previous version's window immediately.
	if err := RotateSecret(ctx, db, "csrf_token_secret", constantSecret("v2"), 0); err != nil {
		t.Fatalf("RotateSecret: %v", err)
	}

	n, err := PruneSecrets(ctx, db, time.Now().Add(time.Second))
	if err != nil {
		t.Fatalf("PruneSecrets: %v", err)
	}
	if n != 1 {
		t.Errorf("pruned %d rows, want 1", n)
	}

	value, version, _, err := GetSecret(ctx, db, "csrf_token_secret")
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if value != "v2" || version != 2 {
		t.Errorf("current = %q v%d, want v2 v2", value, version)
	}
}

// TestPruneSecretsKeepsCurrentAndInWindow guards the two rows prune must never
// touch: the live version, and a superseded version whose grace window is
// still open.
func TestPruneSecretsKeepsCurrentAndInWindow(t *testing.T) {
	db := openServerDB(t)
	ctx := context.Background()

	if _, _, err := EnsureSecret(ctx, db, "cookie_signing_key", constantSecret("v1")); err != nil {
		t.Fatalf("EnsureSecret: %v", err)
	}
	if err := RotateSecret(ctx, db, "cookie_signing_key", constantSecret("v2"), time.Hour); err != nil {
		t.Fatalf("RotateSecret: %v", err)
	}

	n, err := PruneSecrets(ctx, db, time.Now())
	if err != nil {
		t.Fatalf("PruneSecrets: %v", err)
	}
	if n != 0 {
		t.Errorf("pruned %d rows, want 0 while the window is open", n)
	}

	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM app_secrets WHERE name = ?`, "cookie_signing_key").Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 2 {
		t.Errorf("kept %d rows, want 2", rows)
	}
}

// TestSecretErrorsDoNotLeakValues enforces the file-level rule: no error path
// may put a secret value into a message that could reach a log or a response.
func TestSecretErrorsDoNotLeakValues(t *testing.T) {
	db := openServerDB(t)
	ctx := context.Background()

	const value = "TOP-SECRET-VALUE-9f3a"

	// Storing it first, then forcing failures that mention the same name.
	if _, _, err := EnsureSecret(ctx, db, "installation_secret", constantSecret(value)); err != nil {
		t.Fatalf("EnsureSecret: %v", err)
	}

	var errs []error

	_, _, _, err := GetSecret(ctx, db, "missing_secret")
	errs = append(errs, err)

	errs = append(errs, RotateSecret(ctx, db, "missing_secret", constantSecret(value), time.Hour))

	_, _, err = EnsureSecret(ctx, db, "another_missing", constantSecret(""))
	errs = append(errs, err)

	// A rotation whose write fails inside the driver, so the driver's own text
	// is part of the returned message.
	if _, err := db.Exec(`DROP TABLE app_secrets`); err != nil {
		t.Fatalf("drop table: %v", err)
	}
	errs = append(errs, RotateSecret(ctx, db, "installation_secret", constantSecret(value), time.Hour))

	for i, e := range errs {
		if e == nil {
			t.Errorf("case %d produced no error", i)
			continue
		}
		if strings.Contains(e.Error(), value) {
			t.Errorf("case %d leaks the secret value: %s", i, e.Error())
		}
	}
}
