// Package backup periodically copies a live SQLite database with VACUUM INTO
// to publish consistent database files for volume snapshots.
package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	directoryMode = 0o700
	fileSuffix    = ".db"
	fileTime      = "20060102T150405.000000000Z"
	defaultPrefix = "backup-"
)

// ErrInvalidSchedule is returned when the backup interval is not positive or
// the retention count is negative.
var ErrInvalidSchedule = errors.New(
	"backup interval must be positive and retention must be nonnegative",
)

// Execer is the part of a database handle the backup needs. *sqlx.DB and
// *sql.DB both satisfy it.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type config struct {
	prefix string
}

// Option customizes a backup run.
type Option func(*config)

// WithFilePrefix sets the filename prefix for published backups. Pruning only
// considers files with this prefix, so applications should use a stable,
// application-specific prefix.
func WithFilePrefix(prefix string) Option {
	return func(cfg *config) {
		if prefix != "" {
			cfg.prefix = prefix
		}
	}
}

func resolveOptions(opts []Option) config {
	cfg := config{prefix: defaultPrefix}
	for _, opt := range opts {
		opt(&cfg)
	}

	return cfg
}

// Run copies the database into dir on startup and every interval, keeping
// the newest keep files, until ctx is done.
func Run(
	ctx context.Context,
	db Execer,
	dir string,
	interval time.Duration,
	keep int,
	opts ...Option,
) error {
	if interval <= 0 || keep < 0 {
		return ErrInvalidSchedule
	}

	cfg := resolveOptions(opts)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for ctx.Err() == nil {
		err := copyDatabase(ctx, db, dir, keep, time.Now(), cfg)
		if err != nil && ctx.Err() == nil {
			slog.ErrorContext(ctx, "Database backup failed", slog.Any("error", err))
		}

		select {
		case <-ctx.Done():
		case <-ticker.C:
		}
	}

	return fmt.Errorf("stop database backups: %w", ctx.Err())
}

// Copy publishes one consistent copy of the database into dir and prunes to
// the newest keep files.
func Copy(ctx context.Context, db Execer, dir string, keep int, opts ...Option) error {
	if keep < 0 {
		return ErrInvalidSchedule
	}

	return copyDatabase(ctx, db, dir, keep, time.Now(), resolveOptions(opts))
}

func copyDatabase(
	ctx context.Context,
	db Execer,
	dir string,
	keep int,
	now time.Time,
	cfg config,
) error {
	err := os.MkdirAll(dir, directoryMode)
	if err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}

	temp, err := os.CreateTemp(dir, "."+cfg.prefix+"*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary backup: %w", err)
	}

	name := temp.Name()
	defer os.Remove(name)

	err = temp.Close()
	if err != nil {
		return fmt.Errorf("close temporary backup: %w", err)
	}

	_, err = db.ExecContext(ctx, "VACUUM INTO ?", name)
	if err != nil {
		return fmt.Errorf("copy sqlite database: %w", err)
	}

	final := filepath.Join(dir, cfg.prefix+now.UTC().Format(fileTime)+fileSuffix)

	err = os.Rename(name, final)
	if err != nil {
		return fmt.Errorf("publish sqlite backup: %w", err)
	}

	return prune(dir, keep, cfg.prefix)
}

func prune(dir string, keep int, prefix string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("list backups: %w", err)
	}

	var names []string

	for _, entry := range entries {
		name := entry.Name()

		timestamp, ok := strings.CutPrefix(name, prefix)
		if !ok || !entry.Type().IsRegular() {
			continue
		}

		timestamp, ok = strings.CutSuffix(timestamp, fileSuffix)
		if !ok {
			continue
		}

		_, err = time.Parse(fileTime, timestamp)
		if err == nil {
			names = append(names, name)
		}
	}

	// ReadDir orders entries by filename, which is also timestamp order.
	for _, name := range names[:max(0, len(names)-keep)] {
		err = os.Remove(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("prune backup %q: %w", name, err)
		}
	}

	return nil
}
