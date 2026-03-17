// Package db provides a PostgreSQL connection pool for the queue processor.
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
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

// AuthTrx runs fn inside a transaction with RLS claims set for userId.
// All writes to RLS-protected tables (library_item, subscriptions, etc.)
// must go through this to satisfy PostgreSQL row-level security policies.
func (p *Pool) AuthTrx(ctx context.Context, userId string, fn func(tx pgx.Tx) error) error {
	tx, err := p.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, "SELECT omnivore.set_claims($1, 'omnivore_user')", userId); err != nil {
		return fmt.Errorf("set_claims: %w", err)
	}

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
