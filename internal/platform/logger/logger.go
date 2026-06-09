// Package logger builds the application's structured logger from plain
// configuration values, keeping this platform package free of app config types.
package logger

import (
	"log/slog"
	"os"
	"strings"

	"github.com/lmittmann/tint"
)

// New builds the application logger.
//
// format "text" yields a colored, human-readable console handler (for local
// development); any other value yields a JSON handler (for production). level
// is one of debug|info|warn|error (case-insensitive) and defaults to info.
func New(format string, level string) *slog.Logger {
	lvl := parseLevel(level)

	var handler slog.Handler
	if strings.EqualFold(format, "text") {
		handler = tint.NewHandler(os.Stdout, &tint.Options{Level: lvl})
	} else {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	}

	return slog.New(handler)
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
