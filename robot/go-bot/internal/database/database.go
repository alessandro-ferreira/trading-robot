package database

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Database context timeout is a last-resort safeguard against unexpected hangs.
// Normal operation should rely on operation-specific timeouts defined at the database level.
const databaseContextTimeout = 2 * time.Minute

// DBPool is an interface that abstracts the pgxpool.Pool, allowing for mocking in tests.
// It includes the methods from pgxpool.Pool that our application will use.
type DBPool interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Begin(ctx context.Context) (pgx.Tx, error)
	Ping(ctx context.Context) error
	Close()
}

// DB is a wrapper around the database pool that facilitates dependency injection.
type DB struct {
	Pool DBPool
}

// New creates a new DB instance with the provided connection pool.
// This function is useful for injecting a mock pool during testing.
func New(pool DBPool) *DB {
	return &DB{Pool: pool}
}

// Exec is a wrapper for the Pool's Exec method.
func (db *DB) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	ctx, cancel := context.WithTimeout(ctx, databaseContextTimeout)
	defer cancel()
	return db.Pool.Exec(ctx, sql, arguments...)
}

// Query is a wrapper for the Pool's Query method.
func (db *DB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return db.Pool.Query(ctx, sql, args...)
}

// QueryRow is a wrapper for the Pool's QueryRow method.
func (db *DB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return db.Pool.QueryRow(ctx, sql, args...)
}

// Begin is a wrapper for the Pool's Begin method.
func (db *DB) Begin(ctx context.Context) (pgx.Tx, error) {
	ctx, cancel := context.WithTimeout(ctx, databaseContextTimeout)
	defer cancel()
	return db.Pool.Begin(ctx)
}

// Close is a wrapper for the Pool's Close method.
func (db *DB) Close() {
	db.Pool.Close()
}

// Ping is a wrapper for the Pool's Ping method.
func (db *DB) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, databaseContextTimeout)
	defer cancel()
	return db.Pool.Ping(ctx)
}
