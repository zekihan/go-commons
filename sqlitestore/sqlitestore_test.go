package sqlitestore

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
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
