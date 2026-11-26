package sqlitestore

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"modernc.org/sqlite"

	sqlite3 "modernc.org/sqlite/lib"
)

var errTestRollback = errors.New("rollback requested")

var testMigrations = []string{
	`CREATE TABLE settings (id INTEGER PRIMARY KEY CHECK (id = 1), value TEXT NOT NULL);`,
	`CREATE TABLE entries (id TEXT PRIMARY KEY, body TEXT NOT NULL);`,
}

func TestOpenMigrateReopen(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "app #1.db")

	st, err := Open(t.Context(), path, testMigrations[:1])
	if err != nil {
		t.Fatal(err)
	}

	for pragma, want := range map[string]string{
		"PRAGMA user_version": "1",
		"PRAGMA journal_mode": "wal",
		"PRAGMA foreign_keys": "1",
		"PRAGMA busy_timeout": "5000",
	} {
		var got string

		err = st.DB().GetContext(t.Context(), &got, pragma)
		if err != nil || got != want {
			t.Fatalf("%s = %q, want %q, error %v", pragma, got, want, err)
		}
	}

	err = st.Write(t.Context(), func(ctx context.Context, tx *Tx) error {
		_, execErr := tx.Inner().ExecContext(ctx,
			"INSERT INTO settings (id, value) VALUES (1, 'one')")
		if execErr != nil {
			return fmt.Errorf("insert setting: %w", execErr)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	closeErr := st.Close()
	if closeErr != nil {
		t.Fatal(closeErr)
	}

	// Reopening applies only the remaining migration.
	st, err = Open(t.Context(), path, testMigrations)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = st.Close() })

	var version int

	versionErr := st.DB().GetContext(t.Context(), &version, "PRAGMA user_version")
	if versionErr != nil {
		t.Fatal(versionErr)
	}

	if version != len(testMigrations) {
		t.Fatalf("user_version = %d, want %d", version, len(testMigrations))
	}

	err = st.Read(t.Context(), func(ctx context.Context, tx *Tx) error {
		var value string

		getErr := tx.Inner().GetContext(ctx, &value, "SELECT value FROM settings WHERE id = 1")
		if getErr != nil {
			return fmt.Errorf("read setting: %w", getErr)
		}

		if value != "one" {
			t.Errorf("value = %q, want one", value)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestOpenRejectsNewerSchema(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "app.db")

	st, err := Open(t.Context(), path, testMigrations)
	if err != nil {
		t.Fatal(err)
	}

	closeErr := st.Close()
	if closeErr != nil {
		t.Fatal(closeErr)
	}

	_, err = Open(t.Context(), path, testMigrations[:1])
	if !errors.Is(err, ErrSchemaVersion) {
		t.Fatalf("Open with fewer migrations error = %v, want ErrSchemaVersion", err)
	}
}

func TestReadRejectsWrites(t *testing.T) {
	t.Parallel()

	for name, path := range map[string]string{
		"memory": ":memory:",
		"file":   filepath.Join(t.TempDir(), "app.db"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			st, err := Open(ctx, path, []string{"CREATE TABLE sample(v INTEGER)"})
			if err != nil {
				t.Fatal(err)
			}

			t.Cleanup(func() { _ = st.Close() })

			err = st.Read(ctx, func(ctx context.Context, tx *Tx) error {
				_, execErr := tx.Inner().ExecContext(ctx, "INSERT INTO sample VALUES (1)")

				return execErr //nolint:wrapcheck // Assert the SQLite error returned by the callback.
			})

			var sqliteErr *sqlite.Error
			if !errors.As(err, &sqliteErr) || sqliteErr.Code() != sqlite3.SQLITE_READONLY {
				t.Errorf("Read INSERT error = %v, want SQLITE_READONLY", err)
			}

			var count int

			err = st.DB().GetContext(ctx, &count, "SELECT COUNT(*) FROM sample")
			if err != nil {
				t.Fatal(err)
			}

			if count != 0 {
				t.Errorf("sample rows after Read = %d, want 0", count)
			}
		})
	}
}

func TestReadRestoresWriter(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		wantErr   error
		wantPanic any
		finish    func(context.CancelFunc) error
		name      string
	}{
		{name: "normal", finish: func(context.CancelFunc) error { return nil }},
		{
			name:    "error",
			finish:  func(context.CancelFunc) error { return errTestRollback },
			wantErr: errTestRollback,
		},
		{
			name: "cancellation",
			finish: func(cancel context.CancelFunc) error {
				cancel()

				return context.Canceled
			},
			wantErr: context.Canceled,
		},
		{
			name:      "panic",
			finish:    func(context.CancelFunc) error { panic(errTestRollback) },
			wantPanic: errTestRollback,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			st, err := Open(ctx, ":memory:", []string{"CREATE TABLE sample(v INTEGER)"})
			if err != nil {
				t.Fatal(err)
			}

			t.Cleanup(func() { _ = st.Close() })

			readCtx, cancelRead := context.WithCancel(ctx)
			defer cancelRead()

			var (
				readErr   error
				recovered any
			)

			func() {
				defer func() { recovered = recover() }()

				readErr = st.Read(readCtx, func(ctx context.Context, tx *Tx) error {
					var count int

					getErr := tx.Inner().
						GetContext(ctx, &count, "SELECT COUNT(*) FROM sample")
					if getErr != nil {
						t.Fatal(getErr)
					}

					if count != 0 {
						t.Fatalf("sample rows during Read = %d, want 0", count)
					}

					return tc.finish(cancelRead)
				})
			}()

			if recovered != tc.wantPanic || !errors.Is(readErr, tc.wantErr) {
				t.Fatalf("Read returned %v, panic %v, want %v, %v",
					readErr, recovered, tc.wantErr, tc.wantPanic)
			}

			err = st.Write(ctx, func(ctx context.Context, tx *Tx) error {
				_, execErr := tx.Inner().ExecContext(ctx, "INSERT INTO sample VALUES (1)")
				if execErr != nil {
					return fmt.Errorf("insert sample: %w", execErr)
				}

				return nil
			})
			if err != nil {
				t.Fatalf("Write after Read: %v", err)
			}
		})
	}
}

func TestWriteRollsBackOnError(t *testing.T) {
	t.Parallel()

	st, err := Open(t.Context(), ":memory:", testMigrations)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = st.Close() })

	err = st.Write(t.Context(), func(ctx context.Context, tx *Tx) error {
		_, execErr := tx.Inner().ExecContext(ctx,
			"INSERT INTO entries (id, body) VALUES ('1', 'x')")
		if execErr != nil {
			return fmt.Errorf("insert entry: %w", execErr)
		}

		return errTestRollback
	})
	if !errors.Is(err, errTestRollback) {
		t.Fatalf("Write error = %v, want %v", err, errTestRollback)
	}

	var count int

	countErr := st.DB().GetContext(t.Context(), &count, "SELECT COUNT(*) FROM entries")
	if countErr != nil {
		t.Fatal(countErr)
	}

	if count != 0 {
		t.Fatalf("entries after rollback = %d, want 0", count)
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	t.Parallel()

	st, err := Open(t.Context(), ":memory:", testMigrations)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = st.Close() })

	err = st.Write(t.Context(), func(ctx context.Context, tx *Tx) error {
		return Migrate(ctx, tx, testMigrations)
	})
	if err != nil {
		t.Fatal(err)
	}
}
