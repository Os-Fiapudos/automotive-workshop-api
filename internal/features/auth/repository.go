package auth

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrUserNotFound = errors.New("user not found")

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) FindByEmail(ctx context.Context, email string) (*User, error) {
	return r.findBy(ctx, "email = $1", email)
}

func (r *Repository) FindByID(ctx context.Context, id string) (*User, error) {
	return r.findBy(ctx, "id = $1", id)
}

func (r *Repository) findBy(ctx context.Context, where string, arg any) (*User, error) {
	var u User
	err := r.pool.QueryRow(ctx,
		"SELECT id, code, name, email, password_hash FROM users WHERE "+where, arg,
	).Scan(&u.ID, &u.Code, &u.Name, &u.Email, &u.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}
