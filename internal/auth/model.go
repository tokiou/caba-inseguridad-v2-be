package auth

import (
	"net/netip"
	"time"

	"github.com/google/uuid"
)

// User is the public account model — it never carries the password hash.
type User struct {
	ID        uuid.UUID
	Email     string
	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Credentials is User plus the stored bcrypt hash, used only inside the service
// to verify a password. It is never serialized to a client.
type Credentials struct {
	User
	PasswordHash string
}

// RefreshSession is the subset of a refresh_sessions row the service reasons
// about when validating a refresh token.
type RefreshSession struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	ExpiresAt time.Time
	RevokedAt *time.Time
}

// IsRevoked reports whether the session has been revoked.
func (s RefreshSession) IsRevoked() bool { return s.RevokedAt != nil }

// SessionParams describes a refresh session to persist.
type SessionParams struct {
	UserID    uuid.UUID
	TokenHash string
	UserAgent *string
	IP        *netip.Addr
	ExpiresAt time.Time
}

// LoginAttempt is one recorded login outcome.
type LoginAttempt struct {
	Email   string
	IP      *netip.Addr
	Success bool
	Reason  string
}

// AuditEntry is one user-attributable auth event.
type AuditEntry struct {
	UserID    *uuid.UUID
	Action    string
	Metadata  []byte
	IP        *netip.Addr
	UserAgent *string
}

// RequestMeta carries the request-scoped client metadata recorded with auth
// events. Both fields are optional.
type RequestMeta struct {
	IP        *netip.Addr
	UserAgent *string
}

// Audit action names.
const (
	ActionRegister = "register"
	ActionLogin    = "login"
	ActionRefresh  = "refresh"
	ActionLogout   = "logout"
)
