package auth

import (
	"context"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ErrInvalidCredentials is returned for unknown email AND wrong password so
// clients cannot enumerate users (BR4).
var ErrInvalidCredentials = errors.New("invalid credentials")

type UserFinder interface {
	FindByEmail(ctx context.Context, email string) (*User, error)
	FindByID(ctx context.Context, id string) (*User, error)
}

type TokenIssuer interface {
	Generate(userID string) (string, error)
	TTL() time.Duration
}

type LoginResult struct {
	AccessToken string
	ExpiresIn   int64 // seconds
}

type Service struct {
	users  UserFinder
	tokens TokenIssuer
}

func NewService(users UserFinder, tokens TokenIssuer) *Service {
	return &Service{users: users, tokens: tokens}
}

func (s *Service) Login(ctx context.Context, email, password string) (LoginResult, error) {
	user, err := s.users.FindByEmail(ctx, email)
	if errors.Is(err, ErrUserNotFound) {
		return LoginResult{}, ErrInvalidCredentials
	}
	if err != nil {
		return LoginResult{}, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return LoginResult{}, ErrInvalidCredentials
	}
	tokenString, err := s.tokens.Generate(user.ID)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{
		AccessToken: tokenString,
		ExpiresIn:   int64(s.tokens.TTL().Seconds()),
	}, nil
}

func (s *Service) UserByID(ctx context.Context, id string) (*User, error) {
	return s.users.FindByID(ctx, id)
}
