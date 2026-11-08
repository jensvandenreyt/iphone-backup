// Package applog sets up the operation log file.
package applog

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
)

// DefaultPath returns the log file location for this operating system: ~/Library/Logs on macOS,
// %LOCALAPPDATA% on Windows.
func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return defaultPath(runtime.GOOS, home, os.Getenv("LOCALAPPDATA"))
}

func defaultPath(goos, home, localAppData string) string {
	switch goos {
	case "darwin":
		return filepath.Join(home, "Library", "Logs", "iphone-backup", "iphone-backup.log")
	case "windows":
		if localAppData == "" {
			localAppData = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(localAppData, "iphone-backup", "iphone-backup.log")
	}
	return filepath.Join(home, ".local", "state", "iphone-backup", "iphone-backup.log")
}

// Open appends debug-level logs to path and makes it the default slog logger.
func Open(path string) (io.Closer, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug})))
	return f, nil
}

// Fail logs a failed operation with its context and returns err wrapped with the operation name.
func Fail(op string, err error, attrs ...any) error {
	slog.Error(op+" failed", append(attrs, "err", err)...)
	return fmt.Errorf("%s: %w", op, err)
}
