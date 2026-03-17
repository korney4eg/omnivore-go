// Package db provides a PostgreSQL connection pool for the queue processor.
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool wraps pgxpool for queue processor DB access.
type Pool struct{ *pgxpool.Pool }

// New creates a new connection pool and verifies connectivity.
func New(ctx context.Context, dsn string) (*Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db ping: %w", err)
	}
	return &Pool{pool}, nil
}
