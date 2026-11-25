// Package log provides structured logging helpers built on log/slog.
package log

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
)

// Format selects the log output encoding.
type Format string

const (
	// FormatText renders human-readable tint text output.
	FormatText Format = "text"
	// FormatJSON renders structured JSON output.
	FormatJSON Format = "json"
)

// ErrUnknownFormat is returned when a format name is not recognized.
var ErrUnknownFormat = errors.New("unknown log format")

// ParseFormat parses a format name (text or json, case-insensitive).
// It returns an error wrapping ErrUnknownFormat on unknown input.
func ParseFormat(input string) (Format, error) {
	normalized := strings.ToLower(strings.TrimSpace(input))

	switch normalized {
	case string(FormatText):
		return FormatText, nil
	case string(FormatJSON):
		return FormatJSON, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownFormat, input)
	}
}

// ErrUnknownLevel is returned when a level name is not recognized.
var ErrUnknownLevel = errors.New("unknown log level")

// LevelFromString parses a slog level name (debug, info, warn, warning or
// error, case-insensitive, surrounding whitespace allowed).
// It returns an error wrapping ErrUnknownLevel on unknown input.
func LevelFromString(input string) (slog.Level, error) {
	normalized := strings.ToLower(strings.TrimSpace(input))

	switch normalized {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("%w: %q", ErrUnknownLevel, input)
	}
}

// LoggerConfig configures InitLogger.
type LoggerConfig struct {
	Writer    io.Writer
	Format    Format
	Level     slog.Level
	AddSource bool
}

// InitLogger initializes the global slog logger. Passing nil uses sane
// defaults. The empty Format and FormatText select tint text output;
// FormatJSON selects slog JSON output. Any other Format value falls back
// to text output.
func InitLogger(cfg *LoggerConfig) {
	resolved := resolveLoggerConfig(cfg)

	if resolved.Format == FormatJSON {
		slog.SetDefault(slog.New(slog.NewJSONHandler(resolved.Writer, &slog.HandlerOptions{
			Level:     resolved.Level,
			AddSource: resolved.AddSource,
		})))

		return
	}

	slog.SetDefault(slog.New(tint.NewTextHandler(resolved.Writer, &tint.Options{
		Level:      resolved.Level,
		AddSource:  resolved.AddSource,
		TimeFormat: time.RFC3339,
		NoColor:    !writerSupportsColor(resolved.Writer),
	})))
}

func writerSupportsColor(writer io.Writer) bool {
	file, ok := writer.(interface{ Fd() uintptr })
	if !ok {
		return false
	}

	return isatty.IsTerminal(file.Fd()) || isatty.IsCygwinTerminal(file.Fd())
}

func resolveLoggerConfig(cfg *LoggerConfig) LoggerConfig {
	defaultCfg := LoggerConfig{
		Level:     slog.LevelInfo,
		AddSource: false,
		Writer:    os.Stdout,
	}

	if cfg == nil {
		return defaultCfg
	}

	if cfg.Writer != nil {
		defaultCfg.Writer = cfg.Writer
	}

	defaultCfg.Level = cfg.Level
	defaultCfg.AddSource = cfg.AddSource
	defaultCfg.Format = cfg.Format

	return defaultCfg
}
