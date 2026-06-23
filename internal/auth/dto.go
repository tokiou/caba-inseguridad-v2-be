package auth

import "github.com/google/uuid"

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type RegisterResponse struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResponse is the body returned by login and refresh. The refresh token is
// delivered out-of-band in an HttpOnly cookie, never in the body.
type LoginResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

type MeResponse struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
}

type LogoutResponse struct {
	Message string `json:"message"`
}

// authResult bundles the client-facing body with the raw refresh token the
// handler must set as a cookie. Internal to the package.
type authResult struct {
	Response     LoginResponse
	RefreshToken string
}
