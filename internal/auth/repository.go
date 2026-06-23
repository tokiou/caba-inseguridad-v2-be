package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	authdb "github.com/tokiou/caba-inseguridad-routes-go/internal/auth/db"
)

const uniqueViolation = "23505"

// Repository encapsulates all auth data access. sqlc-generated queries live
// behind this hand-written interface; generated types never leak past it.
type Repository interface {
	CreateUser(ctx context.Context, email, passwordHash string) (User, error)
	CredentialsByEmail(ctx context.Context, email string) (Credentials, error)
	UserByID(ctx context.Context, id uuid.UUID) (User, error)

	CreateSession(ctx context.Context, p SessionParams) (uuid.UUID, error)
	SessionByHash(ctx context.Context, tokenHash string) (RefreshSession, error)
	RevokeSession(ctx context.Context, id uuid.UUID) error
	// RotateSession revokes oldID and creates a new session atomically, linking
	// the old row to the new one via replaced_by.
	RotateSession(ctx context.Context, oldID uuid.UUID, p SessionParams) (uuid.UUID, error)

	RecordLoginAttempt(ctx context.Context, a LoginAttempt) error
	WriteAudit(ctx context.Context, e AuditEntry) error
}

type PostgresRepository struct {
	pool    *pgxpool.Pool
	queries *authdb.Queries
}

func NewRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool, queries: authdb.New(pool)}
}

func (r *PostgresRepository) CreateUser(ctx context.Context, email, passwordHash string) (User, error) {
	row, err := r.queries.CreateUser(ctx, authdb.CreateUserParams{Email: email, PasswordHash: passwordHash})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return User{}, ErrEmailTaken
		}
		return User{}, fmt.Errorf("auth: create user: %w", err)
	}
	return userFromRow(row), nil
}

func (r *PostgresRepository) CredentialsByEmail(ctx context.Context, email string) (Credentials, error) {
	row, err := r.queries.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Credentials{}, ErrUserNotFound
		}
		return Credentials{}, fmt.Errorf("auth: get user by email: %w", err)
	}
	return Credentials{User: userFromRow(row), PasswordHash: row.PasswordHash}, nil
}

func (r *PostgresRepository) UserByID(ctx context.Context, id uuid.UUID) (User, error) {
	row, err := r.queries.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrUserNotFound
		}
		return User{}, fmt.Errorf("auth: get user by id: %w", err)
	}
	return userFromRow(row), nil
}

func (r *PostgresRepository) CreateSession(ctx context.Context, p SessionParams) (uuid.UUID, error) {
	session, err := r.queries.CreateRefreshSession(ctx, sessionParams(p))
	if err != nil {
		return uuid.Nil, fmt.Errorf("auth: create refresh session: %w", err)
	}
	return session.ID, nil
}

func (r *PostgresRepository) SessionByHash(ctx context.Context, tokenHash string) (RefreshSession, error) {
	row, err := r.queries.GetRefreshSessionByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RefreshSession{}, ErrSessionNotFound
		}
		return RefreshSession{}, fmt.Errorf("auth: get refresh session: %w", err)
	}
	return RefreshSession{
		ID:        row.ID,
		UserID:    row.UserID,
		ExpiresAt: row.ExpiresAt,
		RevokedAt: timePtr(row.RevokedAt),
	}, nil
}

func (r *PostgresRepository) RevokeSession(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.RevokeRefreshSession(ctx, id); err != nil {
		return fmt.Errorf("auth: revoke refresh session: %w", err)
	}
	return nil
}

func (r *PostgresRepository) RotateSession(ctx context.Context, oldID uuid.UUID, p SessionParams) (uuid.UUID, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("auth: begin rotate tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.queries.WithTx(tx)

	session, err := qtx.CreateRefreshSession(ctx, sessionParams(p))
	if err != nil {
		return uuid.Nil, fmt.Errorf("auth: rotate create session: %w", err)
	}

	err = qtx.RevokeAndReplaceRefreshSession(ctx, authdb.RevokeAndReplaceRefreshSessionParams{
		ID:         oldID,
		ReplacedBy: pgtype.UUID{Bytes: session.ID, Valid: true},
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("auth: rotate revoke old session: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("auth: commit rotate tx: %w", err)
	}
	return session.ID, nil
}

func (r *PostgresRepository) RecordLoginAttempt(ctx context.Context, a LoginAttempt) error {
	err := r.queries.InsertLoginAttempt(ctx, authdb.InsertLoginAttemptParams{
		Email:     a.Email,
		IpAddress: a.IP,
		Success:   a.Success,
		Reason:    strPtr(a.Reason),
	})
	if err != nil {
		return fmt.Errorf("auth: record login attempt: %w", err)
	}
	return nil
}

func (r *PostgresRepository) WriteAudit(ctx context.Context, e AuditEntry) error {
	err := r.queries.InsertAuditLog(ctx, authdb.InsertAuditLogParams{
		UserID:    uuidToPg(e.UserID),
		Action:    e.Action,
		Metadata:  e.Metadata,
		IpAddress: e.IP,
		UserAgent: e.UserAgent,
	})
	if err != nil {
		return fmt.Errorf("auth: write audit log: %w", err)
	}
	return nil
}

func userFromRow(row authdb.User) User {
	return User{
		ID:        row.ID,
		Email:     row.Email,
		IsActive:  row.IsActive,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

func sessionParams(p SessionParams) authdb.CreateRefreshSessionParams {
	return authdb.CreateRefreshSessionParams{
		UserID:    p.UserID,
		TokenHash: p.TokenHash,
		UserAgent: p.UserAgent,
		IpAddress: p.IP,
		ExpiresAt: p.ExpiresAt,
	}
}

func timePtr(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	t := ts.Time
	return &t
}

func uuidToPg(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
