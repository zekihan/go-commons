// Package sqlitestore provides the shared SQLite store scaffold: opening a
// database with WAL and foreign keys enabled, PRAGMA user_version ordered
// migrations, and single-connection read/write transaction helpers.
//
// Applications supply their own migration statements; the store applies them
// in order and records how many ran in PRAGMA user_version. Application
// queries run inside Read or Write callbacks through Tx.Inner, or through the
// raw handle from Store.DB for code that manages its own statements.
package sqlitestore

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"

	"github.com/jmoiron/sqlx"

	// SQLite driver for database/sql.
	_ "modernc.org/sqlite"
)

// ErrSchemaVersion is returned when a database records a schema version the
// supplied migrations do not cover.
var ErrSchemaVersion = errors.New("unsupported schema version")

// Store wraps the database. Its single connection serializes transactions,
// including writes to shared tables such as the oidcauth session tables.
type Store struct {
	db *sqlx.DB
}

// Tx is a single database transaction.
type Tx struct {
	tx *sqlx.Tx
}

// Inner returns the underlying transaction for application queries.
func (tx *Tx) Inner() *sqlx.Tx {
	return tx.tx
}

// Open opens (creating if needed) and migrates the database at path, enabling
// WAL and foreign keys. Migrations are applied in order inside one write
// transaction; a database already at a version beyond len(migrations) is
// rejected with ErrSchemaVersion.
func Open(ctx context.Context, path string, migrations []string) (*Store, error) {
	options := url.Values{
		"_pragma": {"busy_timeout(5000)", "foreign_keys(1)", "journal_mode(WAL)"},
		"_txlock": {"immediate"},
	}
	dsn := new(url.URL)
	dsn.Scheme = "file"
	dsn.RawQuery = options.Encode()

	if path == ":memory:" {
		dsn.Opaque = path
	} else {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("database path: %w", err)
		}

		dsn.Path = absolute
	}

	db, err := sqlx.Open("sqlite", dsn.String())
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	st := &Store{db: db}

	err = st.Write(ctx, func(ctx context.Context, tx *Tx) error {
		return Migrate(ctx, tx, migrations)
	})
	if err != nil {
		return nil, errors.Join(err, st.Close())
	}

	return st, nil
}

// Migrate applies any migrations beyond the version recorded in
// PRAGMA user_version, in order, updating the recorded version after each.
func Migrate(ctx context.Context, tx *Tx, migrations []string) error {
	var version int

	err := tx.tx.GetContext(ctx, &version, "PRAGMA user_version")
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	if version < 0 || version > len(migrations) {
		return fmt.Errorf("%w %d", ErrSchemaVersion, version)
	}

	for index := version; index < len(migrations); index++ {
		_, err = tx.tx.ExecContext(ctx, migrations[index])
		if err != nil {
			return fmt.Errorf("migration %d: %w", index+1, err)
		}

		_, err = tx.tx.ExecContext(ctx, "PRAGMA user_version = "+strconv.Itoa(index+1))
		if err != nil {
			return fmt.Errorf("set schema version: %w", err)
		}
	}

	return nil
}

// DB exposes the handle for queries outside Read/Write and for backups.
func (s *Store) DB() *sqlx.DB {
	return s.db
}

// Close closes the database.
func (s *Store) Close() error {
	err := s.db.Close()
	if err != nil {
		return fmt.Errorf("close sqlite: %w", err)
	}

	return nil
}

// Read runs run in a read-only transaction.
func (s *Store) Read(ctx context.Context, run func(context.Context, *Tx) error) (err error) {
	conn, err := s.db.Connx(ctx)
	if err != nil {
		return fmt.Errorf("reserve sqlite connection: %w", err)
	}
	defer conn.Close()

	var queryOnly int

	err = conn.GetContext(ctx, &queryOnly, "PRAGMA query_only")
	if err != nil {
		return fmt.Errorf("read sqlite query_only: %w", err)
	}

	// The driver's ReadOnly option selects the BEGIN mode. SQLite enforces
	// query_only on the connection, so restore it before releasing the connection.
	defer func() {
		_, resetErr := conn.ExecContext(
			context.WithoutCancel(ctx),
			"PRAGMA query_only = "+strconv.Itoa(queryOnly),
		)
		if resetErr != nil {
			//nolint:errcheck // ErrBadConn discards the connection before it returns to the pool.
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			err = errors.Join(err, fmt.Errorf("restore sqlite query_only: %w", resetErr))
		}
	}()

	_, err = conn.ExecContext(ctx, "PRAGMA query_only = ON")
	if err != nil {
		return fmt.Errorf("enable sqlite query_only: %w", err)
	}

	return transaction(ctx, conn.BeginTxx, &sql.TxOptions{ReadOnly: true}, run)
}

// Write runs run in a write transaction.
func (s *Store) Write(ctx context.Context, run func(context.Context, *Tx) error) error {
	return transaction(ctx, s.db.BeginTxx, nil, run)
}

func transaction(
	ctx context.Context,
	begin func(context.Context, *sql.TxOptions) (*sqlx.Tx, error),
	options *sql.TxOptions,
	run func(context.Context, *Tx) error,
) error {
	tx, err := begin(ctx, options)
	if err != nil {
		return fmt.Errorf("begin sqlite transaction: %w", err)
	}

	// Rollback is also needed on panic. It is a no-op after commit.
	defer tx.Rollback() //nolint:errcheck // The transaction is already done after a successful commit.

	err = run(ctx, &Tx{tx: tx})
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("commit sqlite transaction: %w", err)
	}

	return nil
}
