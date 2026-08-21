package database

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool opens a connection pool to databaseURL and verifies it with a
// Ping before returning, so callers fail fast on a bad connection string
// instead of discovering it on the first request.
func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}
