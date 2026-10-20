package auth

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// Administrative user bootstrap (specs/database-migrations/requirements.md
// BR10, design.md §7): creates the first administrator on a freshly migrated
// database, where — unlike a local one — no development seed runs. Used by
// cmd/bootstrap-admin.

const (
	minAdminPasswordLength = 12
	// bcrypt only uses the first 72 bytes of a password; longer ones are
	// rejected rather than silently truncated.
	maxAdminPasswordLength = 72
	defaultAdminName       = "Administrator"
)

var (
	// ErrInvalidAdminEmail and ErrInvalidAdminPassword never carry the
	// rejected value, so they are safe to log (BR5).
	ErrInvalidAdminEmail    = errors.New("admin e-mail must be a plain address such as admin@example.com")
	ErrInvalidAdminPassword = fmt.Errorf("admin password must be %d to %d bytes long", minAdminPasswordLength, maxAdminPasswordLength)
)

// AdminCreator persists a user unless the e-mail is already taken.
// *Repository implements it.
type AdminCreator interface {
	CreateIfAbsent(ctx context.Context, name, email, passwordHash string) (code int64, created bool, err error)
}

type AdminBootstrapInput struct {
	Name     string // optional, defaults to "Administrator"
	Email    string
	Password string
}

type AdminBootstrapResult struct {
	Created bool  // false: a user with this e-mail already existed and was left unchanged
	Code    int64 // the new user's code; zero when Created is false
}

// BootstrapAdmin validates the input, hashes the password in-process (only
// the bcrypt hash reaches the database) and creates the user if no user has
// that e-mail. An existing user is never modified — in particular its
// password is not replaced.
func BootstrapAdmin(ctx context.Context, users AdminCreator, input AdminBootstrapInput) (AdminBootstrapResult, error) {
	email := strings.TrimSpace(input.Email)
	if !isPlainEmailAddress(email) {
		return AdminBootstrapResult{}, ErrInvalidAdminEmail
	}
	if len(input.Password) < minAdminPasswordLength || len(input.Password) > maxAdminPasswordLength {
		return AdminBootstrapResult{}, ErrInvalidAdminPassword
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = defaultAdminName
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return AdminBootstrapResult{}, fmt.Errorf("hashing the admin password: %w", err)
	}

	code, created, err := users.CreateIfAbsent(ctx, name, email, string(hash))
	if err != nil {
		return AdminBootstrapResult{}, fmt.Errorf("creating the admin user: %w", err)
	}
	return AdminBootstrapResult{Created: created, Code: code}, nil
}

// isPlainEmailAddress accepts "user@domain" only — no display name, no angle
// brackets — so the stored value is exactly what the login form will send.
func isPlainEmailAddress(email string) bool {
	if email == "" {
		return false
	}
	parsed, err := mail.ParseAddress(email)
	return err == nil && parsed.Address == email
}
