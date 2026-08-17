package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type fakeFinder struct {
	byEmail map[string]*User
}

func (f fakeFinder) FindByEmail(_ context.Context, email string) (*User, error) {
	if u, ok := f.byEmail[email]; ok {
		return u, nil
	}
	return nil, ErrUserNotFound
}

func (f fakeFinder) FindByID(_ context.Context, id string) (*User, error) {
	for _, u := range f.byEmail {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, ErrUserNotFound
}

type fakeIssuer struct{}

func (fakeIssuer) Generate(userID string) (string, error) { return "token-for-" + userID, nil }
func (fakeIssuer) TTL() time.Duration                     { return time.Hour }

func newTestService(t *testing.T) *Service {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("right-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	finder := fakeFinder{byEmail: map[string]*User{
		"admin@workshop.local": {ID: "u-1", Code: 1, Name: "Admin", Email: "admin@workshop.local", PasswordHash: string(hash)},
	}}
	return NewService(finder, fakeIssuer{})
}

func TestLoginSuccess(t *testing.T) {
	svc := newTestService(t)
	result, err := svc.Login(context.Background(), "admin@workshop.local", "right-password")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if result.AccessToken != "token-for-u-1" {
		t.Fatalf("token = %q, want token-for-u-1", result.AccessToken)
	}
	if result.ExpiresIn != 3600 {
		t.Fatalf("expiresIn = %d, want 3600", result.ExpiresIn)
	}
}

func TestLoginUnknownEmail(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Login(context.Background(), "nobody@workshop.local", "whatever")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Login(context.Background(), "admin@workshop.local", "wrong-password")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
}
