// Package db provides a PostgreSQL connection pool.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var globalDB *sql.DB
var globalPool *Pool
var globalGorm *gorm.DB

// Pool wraps pgxpool for DB access.
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

// Connect establishes a connection to PostgreSQL and stores it globally.
// Returns a sql.DB for compatibility with existing code.
func Connect(dsn string) (*sql.DB, error) {
	// Create sql.DB connection
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)
	db.SetConnMaxIdleTime(10 * time.Minute)

	// Create pgxpool connection
	pool, err := New(context.Background(), dsn)
	if err != nil {
		db.Close()
		return nil, err
	}

	// Create GORM connection using the existing sql.DB
	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		Conn: db,
	}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent), // Silent in production, can be configurable
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
		// IMPORTANT: Do NOT use auto-migration
		// Schema is managed via SQL migrations in the repository migrations/ directory.
	})
	if err != nil {
		db.Close()
		pool.Close()
		return nil, fmt.Errorf("gorm.Open: %w", err)
	}

	globalDB = db
	globalPool = pool
	globalGorm = gormDB
	return db, nil
}

// Close closes the global database connections.
func Close() error {
	if globalPool != nil {
		globalPool.Close()
	}
	if globalDB != nil {
		return globalDB.Close()
	}
	return nil
}

// GetPool returns the global pgxpool connection.
func GetPool() *Pool {
	return globalPool
}

// GetGorm returns the global GORM database instance.
func GetGorm() *gorm.DB {
	return globalGorm
}
