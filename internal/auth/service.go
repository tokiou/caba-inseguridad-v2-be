package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const (
	bcryptCost     = 12
	passwordMinLen = 8
)

type Service struct {
	repo       Repository
	tokens     *TokenManager
	refreshTTL time.Duration
	log        *slog.Logger
}

func NewService(repo Repository, tokens *TokenManager, refreshTTL time.Duration, log *slog.Logger) *Service {
	return &Service{repo: repo, tokens: tokens, refreshTTL: refreshTTL, log: log}
}

// Register creates a new account and returns its public view.
func (s *Service) Register(ctx context.Context, req RegisterRequest, meta RequestMeta) (RegisterResponse, error) {
	email := normalizeEmail(req.Email)
	if email == "" || req.Password == "" {
		return RegisterResponse{}, ErrEmailRequired
	}
	if len(req.Password) < passwordMinLen {
		return RegisterResponse{}, ErrPasswordTooShort
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcryptCost)
	if err != nil {
		return RegisterResponse{}, fmt.Errorf("auth: hash password: %w", err)
	}

	user, err := s.repo.CreateUser(ctx, email, string(hash))
	if err != nil {
		return RegisterResponse{}, err // ErrEmailTaken passes through
	}

	s.audit(ctx, AuditEntry{UserID: &user.ID, Action: ActionRegister, IP: meta.IP, UserAgent: meta.UserAgent})
	return RegisterResponse{ID: user.ID, Email: user.Email}, nil
}

// Login verifies credentials and, on success, issues an access token plus a new
// refresh session. Credential failures are indistinguishable to avoid user
// enumeration.
func (s *Service) Login(ctx context.Context, req LoginRequest, meta RequestMeta) (authResult, error) {
	email := normalizeEmail(req.Email)
	if email == "" || req.Password == "" {
		return authResult{}, ErrEmailRequired
	}

	creds, err := s.repo.CredentialsByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			s.recordAttempt(ctx, email, meta, false, "unknown_email")
			return authResult{}, ErrInvalidCredentials
		}
		return authResult{}, err
	}

	if bcrypt.CompareHashAndPassword([]byte(creds.PasswordHash), []byte(req.Password)) != nil {
		s.recordAttempt(ctx, email, meta, false, "bad_password")
		return authResult{}, ErrInvalidCredentials
	}

	if !creds.IsActive {
		s.recordAttempt(ctx, email, meta, false, "inactive")
		return authResult{}, ErrInactiveUser
	}

	now := time.Now()
	raw, err := generateRefreshToken()
	if err != nil {
		return authResult{}, err
	}
	if _, err := s.repo.CreateSession(ctx, s.sessionParams(creds.ID, raw, meta, now)); err != nil {
		return authResult{}, err
	}

	s.recordAttempt(ctx, email, meta, true, "")
	s.audit(ctx, AuditEntry{UserID: &creds.ID, Action: ActionLogin, IP: meta.IP, UserAgent: meta.UserAgent})
	return s.buildResult(creds.ID, raw, now)
}

// Refresh validates the presented refresh token, rotates the session, and
// returns a fresh access token.
func (s *Service) Refresh(ctx context.Context, rawToken string, meta RequestMeta) (authResult, error) {
	if rawToken == "" {
		return authResult{}, ErrInvalidRefresh
	}

	now := time.Now()
	session, err := s.repo.SessionByHash(ctx, hashToken(rawToken))
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return authResult{}, ErrInvalidRefresh
		}
		return authResult{}, err
	}
	if session.IsRevoked() || !session.ExpiresAt.After(now) {
		return authResult{}, ErrInvalidRefresh
	}

	newRaw, err := generateRefreshToken()
	if err != nil {
		return authResult{}, err
	}
	if _, err := s.repo.RotateSession(ctx, session.ID, s.sessionParams(session.UserID, newRaw, meta, now)); err != nil {
		return authResult{}, err
	}

	s.audit(ctx, AuditEntry{UserID: &session.UserID, Action: ActionRefresh, IP: meta.IP, UserAgent: meta.UserAgent})
	return s.buildResult(session.UserID, newRaw, now)
}

// Logout revokes the session for the presented refresh token. It is idempotent:
// a missing or unknown token is not an error.
func (s *Service) Logout(ctx context.Context, rawToken string, meta RequestMeta) error {
	if rawToken == "" {
		return nil
	}

	session, err := s.repo.SessionByHash(ctx, hashToken(rawToken))
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return nil
		}
		return err
	}

	if err := s.repo.RevokeSession(ctx, session.ID); err != nil {
		return err
	}
	s.audit(ctx, AuditEntry{UserID: &session.UserID, Action: ActionLogout, IP: meta.IP, UserAgent: meta.UserAgent})
	return nil
}

// Authenticate validates an access token and returns the active user. Any token
// or unknown/inactive-user failure maps to ErrInvalidToken; other errors (e.g.
// the datastore) propagate so the middleware can return 500.
func (s *Service) Authenticate(ctx context.Context, accessToken string) (User, error) {
	userID, err := s.tokens.VerifyAccessToken(accessToken)
	if err != nil {
		return User{}, ErrInvalidToken
	}

	user, err := s.repo.UserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return User{}, ErrInvalidToken
		}
		return User{}, err
	}
	if !user.IsActive {
		return User{}, ErrInvalidToken
	}
	return user, nil
}

func (s *Service) sessionParams(userID uuid.UUID, rawToken string, meta RequestMeta, now time.Time) SessionParams {
	return SessionParams{
		UserID:    userID,
		TokenHash: hashToken(rawToken),
		UserAgent: meta.UserAgent,
		IP:        meta.IP,
		ExpiresAt: now.Add(s.refreshTTL),
	}
}

func (s *Service) buildResult(userID uuid.UUID, rawToken string, now time.Time) (authResult, error) {
	access, expiresIn, err := s.tokens.MintAccessToken(userID, now)
	if err != nil {
		return authResult{}, err
	}
	return authResult{
		Response:     LoginResponse{AccessToken: access, TokenType: "bearer", ExpiresIn: expiresIn},
		RefreshToken: rawToken,
	}, nil
}

func (s *Service) recordAttempt(ctx context.Context, email string, meta RequestMeta, success bool, reason string) {
	attempt := LoginAttempt{Email: email, IP: meta.IP, Success: success, Reason: reason}
	if err := s.repo.RecordLoginAttempt(ctx, attempt); err != nil {
		s.log.WarnContext(ctx, "auth: failed to record login attempt", "err", err)
	}
}

func (s *Service) audit(ctx context.Context, e AuditEntry) {
	if err := s.repo.WriteAudit(ctx, e); err != nil {
		s.log.WarnContext(ctx, "auth: failed to write audit log", "err", err, "action", e.Action)
	}
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
