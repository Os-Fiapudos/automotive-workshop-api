package token

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestGenerateVerifyRoundTrip(t *testing.T) {
	m := NewManager("test-secret", time.Hour)
	tok, err := m.Generate("user-123")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	userID, err := m.Verify(tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if userID != "user-123" {
		t.Fatalf("userID = %q, want user-123", userID)
	}
}

func TestVerifyRejectsExpiredToken(t *testing.T) {
	m := NewManager("test-secret", -time.Minute) // already expired at issue time
	tok, err := m.Generate("user-123")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if _, err := m.Verify(tok); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("err = %v, want ErrInvalidToken", err)
	}
}

func TestVerifyRejectsWrongSecret(t *testing.T) {
	tok, err := NewManager("secret-a", time.Hour).Generate("user-123")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if _, err := NewManager("secret-b", time.Hour).Verify(tok); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("err = %v, want ErrInvalidToken", err)
	}
}

func TestVerifyRejectsGarbage(t *testing.T) {
	m := NewManager("test-secret", time.Hour)
	if _, err := m.Verify("not-a-jwt"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("err = %v, want ErrInvalidToken", err)
	}
}

func TestVerifyRejectsWrongSigningMethod(t *testing.T) {
	m := NewManager("test-secret", time.Hour)
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Subject:   "user-123",
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
	}

	t.Run("none algorithm", func(t *testing.T) {
		tok, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
		if err != nil {
			t.Fatalf("SignedString: %v", err)
		}
		if _, err := m.Verify(tok); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("err = %v, want ErrInvalidToken", err)
		}
	})

	t.Run("HS384 with same secret", func(t *testing.T) {
		tok, err := jwt.NewWithClaims(jwt.SigningMethodHS384, claims).SignedString(m.secret)
		if err != nil {
			t.Fatalf("SignedString: %v", err)
		}
		if _, err := m.Verify(tok); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("err = %v, want ErrInvalidToken", err)
		}
	})
}
