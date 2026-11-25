package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/zekihan/go-commons/sqlitestore"
)

func openLiveDB(t *testing.T) *sqlitestore.Store {
	t.Helper()

	st, err := sqlitestore.Open(t.Context(), filepath.Join(t.TempDir(), "live.db"), []string{
		`CREATE TABLE settings (id INTEGER PRIMARY KEY, value TEXT NOT NULL);
		 INSERT INTO settings (id, value) VALUES (1, 'kept');`,
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = st.Close() })

	return st
}

func backupNames(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		t.Fatal(err)
	}

	var names []string

	for _, entry := range entries {
		names = append(names, entry.Name())
	}

	return names
}

func TestCopyPublishesReadableDatabase(t *testing.T) {
	t.Parallel()

	st := openLiveDB(t)
	dir := filepath.Join(t.TempDir(), "backups")

	err := Copy(t.Context(), st.DB(), dir, 5, WithFilePrefix("app-"))
	if err != nil {
		t.Fatal(err)
	}

	names := backupNames(t, dir)
	if len(names) != 1 {
		t.Fatalf("backups = %v, want one file", names)
	}

	// The copy carries user_version 1; a one-entry list reads it without
	// applying anything.
	copied, err := sqlitestore.Open(t.Context(), filepath.Join(dir, names[0]),
		[]string{"SELECT 1;"})
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = copied.Close() }()

	var value string

	err = copied.DB().GetContext(t.Context(), &value, "SELECT value FROM settings WHERE id = 1")
	if err != nil || value != "kept" {
		t.Fatalf("copied value = %q, error %v", value, err)
	}
}

func TestCopyPrunesToKeep(t *testing.T) {
	t.Parallel()

	st := openLiveDB(t)
	dir := filepath.Join(t.TempDir(), "backups")
	now := time.Date(2026, time.November, 25, 10, 0, 0, 0, time.UTC)
	cfg := resolveOptions([]Option{WithFilePrefix("app-")})

	for offset := range 3 {
		err := copyDatabase(t.Context(), st.DB(), dir, 2,
			now.Add(time.Duration(offset)*time.Second), cfg)
		if err != nil {
			t.Fatal(err)
		}
	}

	names := backupNames(t, dir)
	if len(names) != 2 {
		t.Fatalf("backups after prune = %v, want 2 files", names)
	}

	// An unrelated file in the directory is left alone.
	other := filepath.Join(dir, "notes.txt")

	writeErr := os.WriteFile(other, []byte("keep me"), 0o600)
	if writeErr != nil {
		t.Fatal(writeErr)
	}

	err := Copy(t.Context(), st.DB(), dir, 1, WithFilePrefix("app-"))
	if err != nil {
		t.Fatal(err)
	}

	_, statErr := os.Stat(other)
	if statErr != nil {
		t.Fatalf("unrelated file removed: %v", statErr)
	}
}

func TestInvalidSchedule(t *testing.T) {
	t.Parallel()

	st := openLiveDB(t)

	err := Run(t.Context(), st.DB(), t.TempDir(), 0, 1)
	if !errors.Is(err, ErrInvalidSchedule) {
		t.Fatalf("Run interval 0 error = %v", err)
	}

	err = Copy(t.Context(), st.DB(), t.TempDir(), -1)
	if !errors.Is(err, ErrInvalidSchedule) {
		t.Fatalf("Copy keep -1 error = %v", err)
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	t.Parallel()

	st := openLiveDB(t)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := Run(ctx, st.DB(), t.TempDir(), time.Minute, 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run canceled error = %v", err)
	}
}

func TestRunCopiesImmediately(t *testing.T) {
	t.Parallel()

	st := openLiveDB(t)
	dir := filepath.Join(t.TempDir(), "backups")

	ctx, cancel := context.WithCancel(t.Context())

	done := make(chan error, 1)

	go func() {
		done <- Run(ctx, st.DB(), dir, time.Hour, 3)
	}()

	// The first copy happens before the first tick; wait for a published
	// backup (not the temporary file), then cancel.
	published := func() int {
		count := 0

		for _, name := range backupNames(t, dir) {
			if strings.HasPrefix(name, "backup-") && strings.HasSuffix(name, ".db") {
				count++
			}
		}

		return count
	}

	deadline := time.Now().Add(10 * time.Second)
	for published() == 0 && time.Now().Before(deadline) {
		runtime.Gosched()
	}

	cancel()

	err := <-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v", err)
	}

	if published() == 0 {
		t.Fatal("Run made no backup before cancel")
	}
}
