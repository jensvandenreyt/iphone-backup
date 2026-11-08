// Command iphone-backup browses and exports WhatsApp chats and media from an iPhone backup.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jensvandenreyt/iphone-backup/internal/applog"
	"github.com/jensvandenreyt/iphone-backup/internal/backup"
	"github.com/jensvandenreyt/iphone-backup/internal/tui"
)

func main() {
	dir := flag.String("dir", "", "a backup, or a folder containing backups (default "+backup.DefaultRoot()+")")
	logPath := flag.String("log", applog.DefaultPath(), "operation log file")
	flag.Parse()
	if err := run(*dir, *logPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(dir, logPath string) error {
	logFile, err := applog.Open(logPath)
	if err != nil {
		return fmt.Errorf("open log file (use -log to choose another path): %w", err)
	}
	defer logFile.Close()
	slog.Info("started", "dir", dir, "pid", os.Getpid())

	app := tui.New(dir)
	defer app.Close()
	_, err = tea.NewProgram(app, tea.WithAltScreen()).Run()
	if err != nil {
		slog.Error("program failed", "err", err)
	}
	slog.Info("exited")
	fmt.Fprintln(os.Stderr, "log written to", logPath)
	return err
}
