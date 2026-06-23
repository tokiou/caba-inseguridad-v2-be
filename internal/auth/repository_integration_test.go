//go:build integration

// Integration tests for the Postgres (sqlc-backed) auth repository. Excluded
// from the default build; run against a live database with:
//
//	go test -tags=integration ./internal/auth/...
//
// Uses DATABASE_URL when set, otherwise the local docker-compose Postgres.
package auth

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	postgresplatform "github.com/tokiou/caba-inseguridad-routes-go/internal/platform/postgres"
)

const defaultTestDSN = "postgres://postgres:postgres@localhost:5434/caba_routes?sslmode=disable"

func newTestRepo(t *testing.T) *PostgresRepository {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = defaultTestDSN
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := postgresplatform.NewPool(ctx, dsn)
	if err != nil {
		t.Skipf("skipping: cannot reach Postgres at %s: %v", dsn, err)
	}
	t.Cleanup(pool.Close)
	return NewRepository(pool)
}

// uniqueEmail keeps test rows from colliding across runs.
func uniqueEmail() string {
	return "it-" + uuid.NewString() + "@example.test"
}

func TestIntegrationUserLifecycle(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	email := uniqueEmail()

	user, err := repo.CreateUser(ctx, email, "hash-value")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if user.ID == uuid.Nil || user.Email != email || !user.IsActive {
		t.Fatalf("unexpected user %+v", user)
	}

	// Duplicate email is rejected.
	if _, err := repo.CreateUser(ctx, email, "hash-value"); err != ErrEmailTaken {
		t.Errorf("duplicate CreateUser err = %v, want ErrEmailTaken", err)
	}

	creds, err := repo.CredentialsByEmail(ctx, email)
	if err != nil {
		t.Fatalf("CredentialsByEmail: %v", err)
	}
	if creds.PasswordHash != "hash-value" || creds.ID != user.ID {
		t.Errorf("unexpected creds %+v", creds)
	}

	got, err := repo.UserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("UserByID: %v", err)
	}
	if got.ID != user.ID {
		t.Errorf("UserByID = %v, want %v", got.ID, user.ID)
	}

	if _, err := repo.CredentialsByEmail(ctx, uniqueEmail()); err != ErrUserNotFound {
		t.Errorf("missing user err = %v, want ErrUserNotFound", err)
	}
}

func TestIntegrationSessionRotation(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	user, err := repo.CreateUser(ctx, uniqueEmail(), "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	p := SessionParams{UserID: user.ID, TokenHash: hashToken(uuid.NewString()), ExpiresAt: time.Now().Add(time.Hour)}
	oldID, err := repo.CreateSession(ctx, p)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	session, err := repo.SessionByHash(ctx, p.TokenHash)
	if err != nil {
		t.Fatalf("SessionByHash: %v", err)
	}
	if session.ID != oldID || session.IsRevoked() {
		t.Fatalf("unexpected session %+v", session)
	}

	// Rotate: old becomes revoked, new is active.
	newParams := SessionParams{UserID: user.ID, TokenHash: hashToken(uuid.NewString()), ExpiresAt: time.Now().Add(time.Hour)}
	newID, err := repo.RotateSession(ctx, oldID, newParams)
	if err != nil {
		t.Fatalf("RotateSession: %v", err)
	}

	old, err := repo.SessionByHash(ctx, p.TokenHash)
	if err != nil {
		t.Fatalf("SessionByHash(old): %v", err)
	}
	if !old.IsRevoked() {
		t.Error("old session should be revoked after rotation")
	}

	fresh, err := repo.SessionByHash(ctx, newParams.TokenHash)
	if err != nil {
		t.Fatalf("SessionByHash(new): %v", err)
	}
	if fresh.ID != newID || fresh.IsRevoked() {
		t.Errorf("new session should be active, got %+v", fresh)
	}

	// Revoke the new one directly.
	if err := repo.RevokeSession(ctx, newID); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}
	revoked, _ := repo.SessionByHash(ctx, newParams.TokenHash)
	if !revoked.IsRevoked() {
		t.Error("session should be revoked")
	}
}

func TestIntegrationAttemptAndAudit(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	user, err := repo.CreateUser(ctx, uniqueEmail(), "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if err := repo.RecordLoginAttempt(ctx, LoginAttempt{Email: user.Email, Success: false, Reason: "bad_password"}); err != nil {
		t.Errorf("RecordLoginAttempt: %v", err)
	}
	if err := repo.WriteAudit(ctx, AuditEntry{UserID: &user.ID, Action: ActionLogin, Metadata: []byte(`{"k":"v"}`)}); err != nil {
		t.Errorf("WriteAudit: %v", err)
	}
	// A nil user id (e.g. failed login) must still write.
	if err := repo.WriteAudit(ctx, AuditEntry{Action: ActionLogin}); err != nil {
		t.Errorf("WriteAudit nil user: %v", err)
	}
}
