package log

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"strings"
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

func TestLoggerDoesNotWriteANSIColorCodes(t *testing.T) {
	var output bytes.Buffer
	InitLogger(&LoggerConfig{Writer: &output})

	slog.Info("Test log message")

	if strings.Contains(output.String(), "\x1b[") {
		t.Fatalf("log output contains ANSI color codes: %q", output.String())
	}
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

func TestInitLoggerJSONFormat(t *testing.T) {
	var output bytes.Buffer
	InitLogger(&LoggerConfig{Writer: &output, Format: FormatJSON, Level: slog.LevelDebug})

	slog.Info("JSON marker", slog.String("key", "value"))

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected one JSON line, got %d: %q", len(lines), output.String())
	}

	var record map[string]any

	err := json.Unmarshal([]byte(lines[0]), &record)
	if err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	if record["msg"] != "JSON marker" {
		t.Errorf("msg = %v, want %q", record["msg"], "JSON marker")
	}

	if _, ok := record["level"]; !ok {
		t.Errorf("missing level key in JSON output: %v", record)
	}

	if record["key"] != "value" {
		t.Errorf("key = %v, want %q", record["key"], "value")
	}
}

func TestInitLoggerTextFormat(t *testing.T) {
	var output bytes.Buffer
	InitLogger(&LoggerConfig{Writer: &output, Format: FormatText})

	slog.Info("Text marker")

	if !strings.Contains(output.String(), "Text marker") {
		t.Errorf("text output missing message: %q", output.String())
	}

	if strings.Contains(output.String(), "{\"msg\"") {
		t.Errorf("text output looks like JSON: %q", output.String())
	}
}

func TestLevelFromString(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    slog.Level
		wantErr bool
	}{
		{"debug lower", "debug", slog.LevelDebug, false},
		{"debug upper", "DEBUG", slog.LevelDebug, false},
		{"info", "info", slog.LevelInfo, false},
		{"warn", "warn", slog.LevelWarn, false},
		{"warning", "warning", slog.LevelWarn, false},
		{"error", "error", slog.LevelError, false},
		{"padded", "  warn  ", slog.LevelWarn, false},
		{"empty", "", slog.LevelInfo, true},
		{"unknown", "verbose", slog.LevelInfo, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LevelFromString(tt.input)
			if tt.wantErr {
				if !errors.Is(err, ErrUnknownLevel) {
					t.Errorf("LevelFromString(%q) error = %v, want ErrUnknownLevel", tt.input, err)
				}

				return
			}

			if err != nil {
				t.Fatalf("LevelFromString(%q) error = %v", tt.input, err)
			}

			if got != tt.want {
				t.Errorf("LevelFromString(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseFormat(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Format
		wantErr bool
	}{
		{"text lower", "text", FormatText, false},
		{"json upper", "JSON", FormatJSON, false},
		{"padded", "  json  ", FormatJSON, false},
		{"empty", "", "", true},
		{"unknown", "xml", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFormat(tt.input)
			if tt.wantErr {
				if !errors.Is(err, ErrUnknownFormat) {
					t.Errorf("ParseFormat(%q) error = %v, want ErrUnknownFormat", tt.input, err)
				}

				return
			}

			if err != nil {
				t.Fatalf("ParseFormat(%q) error = %v", tt.input, err)
			}

			if got != tt.want {
				t.Errorf("ParseFormat(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
