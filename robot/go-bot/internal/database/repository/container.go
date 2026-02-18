package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DefaultUser is the system user for automated database operations.
const DefaultUser = "go-bot"

// DBExecutor defines the common interface for executing database queries.
// It is satisfied by *pgxpool.Pool, *pgx.Conn, and pgx.Tx.
type DBExecutor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Container holds all the repositories for the application.
// This allows services to access any repository through a single dependency.
type Container struct {
	Balances BalancesRepo
	// Orders   *OrdersRepo // Future repositories will be added here
}

// New creates a new repository container.
func New() *Container {
	return &Container{
		Balances: NewBalancesRepo(),
	}
}
