package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestMintAndVerifyAccessToken(t *testing.T) {
	tm := NewTokenManager("secret", 15*time.Minute)
	uid := uuid.New()

	token, expiresIn, err := tm.MintAccessToken(uid, time.Now())
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if expiresIn != 900 {
		t.Errorf("expiresIn = %d, want 900", expiresIn)
	}

	got, err := tm.VerifyAccessToken(token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got != uid {
		t.Errorf("subject = %v, want %v", got, uid)
	}
}

func TestVerifyAccessTokenRejects(t *testing.T) {
	tm := NewTokenManager("secret", 15*time.Minute)
	uid := uuid.New()

	t.Run("expired", func(t *testing.T) {
		expired, _, _ := tm.MintAccessToken(uid, time.Now().Add(-time.Hour))
		if _, err := tm.VerifyAccessToken(expired); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("want ErrInvalidToken, got %v", err)
		}
	})

	t.Run("wrong signature", func(t *testing.T) {
		valid, _, _ := tm.MintAccessToken(uid, time.Now())
		other := NewTokenManager("different", 15*time.Minute)
		if _, err := other.VerifyAccessToken(valid); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("want ErrInvalidToken, got %v", err)
		}
	})

	t.Run("wrong typ", func(t *testing.T) {
		claims := accessClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   uid.String(),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
			Typ: "refresh",
		}
		signed, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("secret"))
		if _, err := tm.VerifyAccessToken(signed); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("want ErrInvalidToken, got %v", err)
		}
	})

	t.Run("garbage", func(t *testing.T) {
		if _, err := tm.VerifyAccessToken("not.a.jwt"); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("want ErrInvalidToken, got %v", err)
		}
	})
}

func TestHashToken(t *testing.T) {
	first, second := hashToken("abc"), hashToken("abc")
	if first != second {
		t.Error("hashToken is not deterministic")
	}
	if hashToken("abc") == hashToken("abd") {
		t.Error("distinct inputs hashed equal")
	}
}

func TestGenerateRefreshToken(t *testing.T) {
	a, err := generateRefreshToken()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	b, _ := generateRefreshToken()
	if a == b {
		t.Error("generated tokens are not unique")
	}
	if len(a) < 40 {
		t.Errorf("token length = %d, want >= 40", len(a))
	}
}
