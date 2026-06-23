package auth

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// fakeRepo is an in-memory Repository for service tests.
type fakeRepo struct {
	credsByEmail map[string]Credentials
	usersByID    map[uuid.UUID]User
	sessions     map[string]*RefreshSession // keyed by token hash

	attempts    []LoginAttempt
	audits      []AuditEntry
	rotateCalls int
	revokeCalls int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		credsByEmail: map[string]Credentials{},
		usersByID:    map[uuid.UUID]User{},
		sessions:     map[string]*RefreshSession{},
	}
}

func (f *fakeRepo) addUser(email, password string, active bool) User {
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	u := User{ID: uuid.New(), Email: email, IsActive: active}
	f.credsByEmail[email] = Credentials{User: u, PasswordHash: string(hash)}
	f.usersByID[u.ID] = u
	return u
}

func (f *fakeRepo) CreateUser(_ context.Context, email, passwordHash string) (User, error) {
	if _, ok := f.credsByEmail[email]; ok {
		return User{}, ErrEmailTaken
	}
	u := User{ID: uuid.New(), Email: email, IsActive: true}
	f.credsByEmail[email] = Credentials{User: u, PasswordHash: passwordHash}
	f.usersByID[u.ID] = u
	return u, nil
}

func (f *fakeRepo) CredentialsByEmail(_ context.Context, email string) (Credentials, error) {
	c, ok := f.credsByEmail[email]
	if !ok {
		return Credentials{}, ErrUserNotFound
	}
	return c, nil
}

func (f *fakeRepo) UserByID(_ context.Context, id uuid.UUID) (User, error) {
	u, ok := f.usersByID[id]
	if !ok {
		return User{}, ErrUserNotFound
	}
	return u, nil
}

func (f *fakeRepo) CreateSession(_ context.Context, p SessionParams) (uuid.UUID, error) {
	id := uuid.New()
	f.sessions[p.TokenHash] = &RefreshSession{ID: id, UserID: p.UserID, ExpiresAt: p.ExpiresAt}
	return id, nil
}

func (f *fakeRepo) SessionByHash(_ context.Context, tokenHash string) (RefreshSession, error) {
	s, ok := f.sessions[tokenHash]
	if !ok {
		return RefreshSession{}, ErrSessionNotFound
	}
	return *s, nil
}

func (f *fakeRepo) RevokeSession(_ context.Context, id uuid.UUID) error {
	f.revokeCalls++
	for _, s := range f.sessions {
		if s.ID == id {
			now := time.Now()
			s.RevokedAt = &now
		}
	}
	return nil
}

func (f *fakeRepo) RotateSession(_ context.Context, oldID uuid.UUID, p SessionParams) (uuid.UUID, error) {
	f.rotateCalls++
	for _, s := range f.sessions {
		if s.ID == oldID {
			now := time.Now()
			s.RevokedAt = &now
		}
	}
	id := uuid.New()
	f.sessions[p.TokenHash] = &RefreshSession{ID: id, UserID: p.UserID, ExpiresAt: p.ExpiresAt}
	return id, nil
}

func (f *fakeRepo) RecordLoginAttempt(_ context.Context, a LoginAttempt) error {
	f.attempts = append(f.attempts, a)
	return nil
}

func (f *fakeRepo) WriteAudit(_ context.Context, e AuditEntry) error {
	f.audits = append(f.audits, e)
	return nil
}

func newTestService(repo Repository) *Service {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewService(repo, NewTokenManager("test-secret", 15*time.Minute), 7*24*time.Hour, log)
}

func TestServiceRegister(t *testing.T) {
	t.Run("creates account", func(t *testing.T) {
		repo := newFakeRepo()
		svc := newTestService(repo)
		resp, err := svc.Register(context.Background(), RegisterRequest{Email: "Test@Example.com", Password: "password123"}, RequestMeta{})
		if err != nil {
			t.Fatalf("register: %v", err)
		}
		if resp.Email != "test@example.com" {
			t.Errorf("email = %q, want normalized lowercase", resp.Email)
		}
		if len(repo.audits) != 1 || repo.audits[0].Action != ActionRegister {
			t.Errorf("expected one register audit, got %+v", repo.audits)
		}
	})

	t.Run("rejects duplicate email", func(t *testing.T) {
		repo := newFakeRepo()
		repo.addUser("dup@example.com", "password123", true)
		svc := newTestService(repo)
		_, err := svc.Register(context.Background(), RegisterRequest{Email: "dup@example.com", Password: "password123"}, RequestMeta{})
		if !errors.Is(err, ErrEmailTaken) {
			t.Errorf("want ErrEmailTaken, got %v", err)
		}
	})

	t.Run("rejects short password", func(t *testing.T) {
		svc := newTestService(newFakeRepo())
		_, err := svc.Register(context.Background(), RegisterRequest{Email: "a@b.com", Password: "short"}, RequestMeta{})
		if !errors.Is(err, ErrPasswordTooShort) {
			t.Errorf("want ErrPasswordTooShort, got %v", err)
		}
	})

	t.Run("rejects missing fields", func(t *testing.T) {
		svc := newTestService(newFakeRepo())
		_, err := svc.Register(context.Background(), RegisterRequest{Email: "", Password: ""}, RequestMeta{})
		if !errors.Is(err, ErrEmailRequired) {
			t.Errorf("want ErrEmailRequired, got %v", err)
		}
	})
}

func TestServiceLogin(t *testing.T) {
	t.Run("success issues tokens and records attempt", func(t *testing.T) {
		repo := newFakeRepo()
		repo.addUser("user@example.com", "password123", true)
		svc := newTestService(repo)

		result, err := svc.Login(context.Background(), LoginRequest{Email: "user@example.com", Password: "password123"}, RequestMeta{})
		if err != nil {
			t.Fatalf("login: %v", err)
		}
		if result.Response.AccessToken == "" || result.RefreshToken == "" {
			t.Error("expected non-empty access and refresh tokens")
		}
		if result.Response.TokenType != "bearer" || result.Response.ExpiresIn != 900 {
			t.Errorf("unexpected response %+v", result.Response)
		}
		if len(repo.sessions) != 1 {
			t.Errorf("expected one session, got %d", len(repo.sessions))
		}
		if n := len(repo.attempts); n != 1 || !repo.attempts[0].Success {
			t.Errorf("expected one successful attempt, got %+v", repo.attempts)
		}
	})

	t.Run("unknown email and bad password both yield invalid_credentials", func(t *testing.T) {
		repo := newFakeRepo()
		repo.addUser("user@example.com", "password123", true)
		svc := newTestService(repo)

		_, err := svc.Login(context.Background(), LoginRequest{Email: "nobody@example.com", Password: "password123"}, RequestMeta{})
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("unknown email: want ErrInvalidCredentials, got %v", err)
		}
		_, err = svc.Login(context.Background(), LoginRequest{Email: "user@example.com", Password: "wrongpass1"}, RequestMeta{})
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("bad password: want ErrInvalidCredentials, got %v", err)
		}
		for _, a := range repo.attempts {
			if a.Success {
				t.Error("failed logins must record success=false")
			}
		}
	})

	t.Run("inactive account is forbidden", func(t *testing.T) {
		repo := newFakeRepo()
		repo.addUser("inactive@example.com", "password123", false)
		svc := newTestService(repo)
		_, err := svc.Login(context.Background(), LoginRequest{Email: "inactive@example.com", Password: "password123"}, RequestMeta{})
		if !errors.Is(err, ErrInactiveUser) {
			t.Errorf("want ErrInactiveUser, got %v", err)
		}
	})
}

func TestServiceRefresh(t *testing.T) {
	setup := func() (*fakeRepo, *Service, string) {
		repo := newFakeRepo()
		repo.addUser("user@example.com", "password123", true)
		svc := newTestService(repo)
		result, err := svc.Login(context.Background(), LoginRequest{Email: "user@example.com", Password: "password123"}, RequestMeta{})
		if err != nil {
			t.Fatalf("login: %v", err)
		}
		return repo, svc, result.RefreshToken
	}

	t.Run("rotates and returns a new access token", func(t *testing.T) {
		repo, svc, refresh := setup()
		result, err := svc.Refresh(context.Background(), refresh, RequestMeta{})
		if err != nil {
			t.Fatalf("refresh: %v", err)
		}
		if result.RefreshToken == refresh {
			t.Error("refresh token was not rotated")
		}
		if repo.rotateCalls != 1 {
			t.Errorf("rotateCalls = %d, want 1", repo.rotateCalls)
		}
		// Old token must no longer work.
		if _, err := svc.Refresh(context.Background(), refresh, RequestMeta{}); !errors.Is(err, ErrInvalidRefresh) {
			t.Errorf("reused old token: want ErrInvalidRefresh, got %v", err)
		}
	})

	t.Run("empty token rejected", func(t *testing.T) {
		_, svc, _ := setup()
		if _, err := svc.Refresh(context.Background(), "", RequestMeta{}); !errors.Is(err, ErrInvalidRefresh) {
			t.Errorf("want ErrInvalidRefresh, got %v", err)
		}
	})

	t.Run("expired session rejected", func(t *testing.T) {
		repo := newFakeRepo()
		u := repo.addUser("user@example.com", "password123", true)
		svc := newTestService(repo)
		raw, _ := generateRefreshToken()
		repo.sessions[hashToken(raw)] = &RefreshSession{ID: uuid.New(), UserID: u.ID, ExpiresAt: time.Now().Add(-time.Minute)}
		if _, err := svc.Refresh(context.Background(), raw, RequestMeta{}); !errors.Is(err, ErrInvalidRefresh) {
			t.Errorf("want ErrInvalidRefresh, got %v", err)
		}
	})
}

func TestServiceLogout(t *testing.T) {
	repo := newFakeRepo()
	repo.addUser("user@example.com", "password123", true)
	svc := newTestService(repo)
	result, _ := svc.Login(context.Background(), LoginRequest{Email: "user@example.com", Password: "password123"}, RequestMeta{})

	if err := svc.Logout(context.Background(), result.RefreshToken, RequestMeta{}); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if repo.revokeCalls != 1 {
		t.Errorf("revokeCalls = %d, want 1", repo.revokeCalls)
	}
	// Refresh after logout must fail.
	if _, err := svc.Refresh(context.Background(), result.RefreshToken, RequestMeta{}); !errors.Is(err, ErrInvalidRefresh) {
		t.Errorf("refresh after logout: want ErrInvalidRefresh, got %v", err)
	}
	// Logout is idempotent.
	if err := svc.Logout(context.Background(), "", RequestMeta{}); err != nil {
		t.Errorf("idempotent logout: %v", err)
	}
}

func TestServiceAuthenticate(t *testing.T) {
	repo := newFakeRepo()
	user := repo.addUser("user@example.com", "password123", true)
	svc := newTestService(repo)
	token, _, _ := svc.tokens.MintAccessToken(user.ID, time.Now())

	t.Run("valid token returns active user", func(t *testing.T) {
		got, err := svc.Authenticate(context.Background(), token)
		if err != nil {
			t.Fatalf("authenticate: %v", err)
		}
		if got.ID != user.ID {
			t.Errorf("user = %v, want %v", got.ID, user.ID)
		}
	})

	t.Run("inactive user rejected", func(t *testing.T) {
		inactive := repo.addUser("inactive@example.com", "password123", false)
		tok, _, _ := svc.tokens.MintAccessToken(inactive.ID, time.Now())
		if _, err := svc.Authenticate(context.Background(), tok); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("want ErrInvalidToken, got %v", err)
		}
	})

	t.Run("unknown user rejected", func(t *testing.T) {
		tok, _, _ := svc.tokens.MintAccessToken(uuid.New(), time.Now())
		if _, err := svc.Authenticate(context.Background(), tok); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("want ErrInvalidToken, got %v", err)
		}
	})
}
