package utils

import (
	"log/slog"

	"github.com/zekihan/go-commons/log"
)

// LoggerConfig configures InitLogger.
//
// Deprecated: use github.com/zekihan/go-commons/log instead.
type LoggerConfig = log.LoggerConfig

// Format selects the log output encoding.
//
// Deprecated: use github.com/zekihan/go-commons/log instead.
type Format = log.Format

const (
	// FormatText renders human-readable tint text output.
	//
	// Deprecated: use github.com/zekihan/go-commons/log instead.
	FormatText = log.FormatText

	// FormatJSON renders structured JSON output.
	//
	// Deprecated: use github.com/zekihan/go-commons/log instead.
	FormatJSON = log.FormatJSON
)

// InitLogger initializes the global slog logger.
//
// Deprecated: use github.com/zekihan/go-commons/log instead.
func InitLogger(cfg *LoggerConfig) {
	log.InitLogger(cfg)
}

// LevelFromString parses a slog level name.
//
// Deprecated: use github.com/zekihan/go-commons/log instead.
func LevelFromString(input string) (slog.Level, error) {
	return log.LevelFromString(input) //nolint:wrapcheck // Thin re-export, return as-is.
}

// ParseFormat parses a format name.
//
// Deprecated: use github.com/zekihan/go-commons/log instead.
func ParseFormat(input string) (Format, error) {
	return log.ParseFormat(input) //nolint:wrapcheck // Thin re-export, return as-is.
}
