package utils

import (
	"log/slog"
	"os"
	"testing"
)

func TestLoggerInitializesWithNilWriter(t *testing.T) {
	cfg := &LoggerConfig{
		Level:     slog.LevelWarn,
		AddSource: true,
		Writer:    nil,
	}

	InitLogger(cfg)
}

func TestLoggerUsesDefaultConfigWhenNilPassed(t *testing.T) {
	InitLogger(nil)
}

func TestLoggerInitializesWithCustomLevelAndSource(t *testing.T) {
	cfg := &LoggerConfig{
		Level:     slog.LevelError,
		AddSource: true,
		Writer:    os.Stdout,
	}

	InitLogger(cfg)
}

func TestLoggerFailsWithInvalidWriter(t *testing.T) {
	cfg := &LoggerConfig{
		Level:     slog.LevelInfo,
		AddSource: false,
		Writer:    nil,
	}

	// Simulate invalid writer by passing a closed file
	file, _ := os.CreateTemp(t.TempDir(), "test")
	_ = file.Close()

	cfg.Writer = file

	InitLogger(cfg)
}
