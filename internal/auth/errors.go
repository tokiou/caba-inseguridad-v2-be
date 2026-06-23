package auth

import "errors"

var (
	// Validation / registration.
	ErrEmailRequired    = errors.New("email and password are required")
	ErrPasswordTooShort = errors.New("password too short")
	ErrEmailTaken       = errors.New("email already registered")

	// Login.
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInactiveUser       = errors.New("account inactive")

	// Access token (middleware).
	ErrInvalidToken = errors.New("invalid or expired access token")

	// Refresh / logout.
	ErrInvalidRefresh = errors.New("invalid refresh token")

	// Internal repository sentinels (mapped by the service, not surfaced raw).
	ErrUserNotFound    = errors.New("user not found")
	ErrSessionNotFound = errors.New("refresh session not found")
)
