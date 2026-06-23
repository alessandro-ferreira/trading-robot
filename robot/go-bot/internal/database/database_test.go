//go:build unit

package database

import (
	"context"
	"regexp"
	"testing"

	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/require"
)

// newMockPool creates a new mock pool with ping monitoring enabled.
func newMockPool(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	return mock
}

func TestDB_Ping(t *testing.T) {
	mock := newMockPool(t)
	defer mock.Close()

	db := New(mock)

	mock.ExpectPing()

	err := db.Ping(context.Background())
	require.NoError(t, err)

	require.NoError(t, mock.ExpectationsWereMet(), "there were unfulfilled expectations")
}

func TestDB_Query(t *testing.T) {
	mock := newMockPool(t)
	defer mock.Close()

	db := New(mock)

	rows := pgxmock.NewRows([]string{"id", "name"}).AddRow(1, "test_name")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, name FROM users WHERE id = $1")).
		WithArgs(1).
		WillReturnRows(rows)

	_, err := db.Query(context.Background(), "SELECT id, name FROM users WHERE id = $1", 1)
	require.NoError(t, err)

	require.NoError(t, mock.ExpectationsWereMet(), "there were unfulfilled expectations")
}

func TestDB_QueryRow(t *testing.T) {
	mock := newMockPool(t)
	defer mock.Close()

	db := New(mock)

	row := pgxmock.NewRows([]string{"name"}).AddRow("test_name")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT name FROM users WHERE id = $1")).
		WithArgs(1).
		WillReturnRows(row)

	var name string
	err := db.QueryRow(context.Background(), "SELECT name FROM users WHERE id = $1", 1).Scan(&name)
	require.NoError(t, err)
	require.Equal(t, "test_name", name)

	require.NoError(t, mock.ExpectationsWereMet(), "there were unfulfilled expectations")
}

func TestDB_Exec(t *testing.T) {
	mock := newMockPool(t)
	defer mock.Close()

	db := New(mock)

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO users (name) VALUES ($1)")).
		WithArgs("test_name").
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	_, err := db.Exec(context.Background(), "INSERT INTO users (name) VALUES ($1)", "test_name")
	require.NoError(t, err)

	require.NoError(t, mock.ExpectationsWereMet(), "there were unfulfilled expectations")
}

func TestDB_Transaction(t *testing.T) {
	mock := newMockPool(t)
	defer mock.Close()

	db := New(mock)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE users SET name = $1 WHERE id = $2")).
		WithArgs("new_name", 1).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit()

	tx, err := db.Begin(context.Background())
	require.NoError(t, err)

	_, err = tx.Exec(context.Background(), "UPDATE users SET name = $1 WHERE id = $2", "new_name", 1)
	require.NoError(t, err)

	err = tx.Commit(context.Background())
	require.NoError(t, err)

	require.NoError(t, mock.ExpectationsWereMet(), "there were unfulfilled expectations")
}

func TestDB_TransactionRollback(t *testing.T) {
	mock := newMockPool(t)
	defer mock.Close()

	db := New(mock)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM users WHERE id = $1")).
		WithArgs(1).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectRollback()

	tx, err := db.Begin(context.Background())
	require.NoError(t, err)

	_, err = tx.Exec(context.Background(), "DELETE FROM users WHERE id = $1", 1)
	require.NoError(t, err)

	err = tx.Rollback(context.Background())
	require.NoError(t, err)

	require.NoError(t, mock.ExpectationsWereMet(), "there were unfulfilled expectations")
}

func TestDB_Close(t *testing.T) {
	mock := newMockPool(t)
	defer mock.Close()

	db := New(mock)

	mock.ExpectClose()
	db.Close()

	require.NoError(t, mock.ExpectationsWereMet(), "there were unfulfilled expectations")
}
