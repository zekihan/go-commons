package utils

import (
	"io"
	"log/slog"
	"os"
)

type LoggerConfig struct {
	Writer    io.Writer
	Level     slog.Level
	AddSource bool
}

// InitLogger initializes the global slog logger.
// Passing nil uses sane defaults.
func InitLogger(cfg *LoggerConfig) {
	c := resolveLoggerConfig(cfg)

	handler := slog.NewTextHandler(c.Writer, &slog.HandlerOptions{
		Level:     c.Level,
		AddSource: c.AddSource,
	})

	slog.SetDefault(slog.New(handler))
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

	return defaultCfg
}
