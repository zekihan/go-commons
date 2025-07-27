package utils

import (
	"fmt"
	"os"
	"time"

	"github.com/snowzach/rotatefilehook"

	log "github.com/sirupsen/logrus"
)

const (
	defaultLogFileSize   = 50 // megabytes
	defaultLogMaxBackups = 3
	defaultLogMaxAge     = 28 // days
)

type LoggerConfig struct {
	Path          string
	Level         log.Level
	AddMethodName bool
}

func InitLogger(cfg *LoggerConfig) error {
	log.SetLevel(cfg.Level)
	log.SetFormatter(&log.TextFormatter{
		ForceColors:     true,
		FullTimestamp:   true,
		TimestampFormat: time.RFC822,
	})
	log.SetReportCaller(cfg.AddMethodName)
	log.SetOutput(os.Stdout)

	if cfg.Path != "" {
		err := AddFileHook(cfg)
		if err != nil {
			return fmt.Errorf("failed to add file hook: %w", err)
		}
	}

	return nil
}

func AddFileHook(cfg *LoggerConfig) error {
	rotateFileHook, err := rotatefilehook.NewRotateFileHook(rotatefilehook.RotateFileConfig{
		Filename:   cfg.Path,
		MaxSize:    defaultLogFileSize, // megabytes
		MaxBackups: defaultLogMaxBackups,
		MaxAge:     defaultLogMaxAge, // days
		Level:      cfg.Level,
		Formatter: &log.JSONFormatter{
			TimestampFormat: time.RFC822,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create rotate file hook: %w", err)
	}

	log.AddHook(rotateFileHook)

	return nil
}
