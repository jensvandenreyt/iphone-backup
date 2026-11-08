// Command iphone-backup browses and exports WhatsApp chats and media from an iPhone backup.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vandenreyt/iphone-backup/internal/backup"
	"github.com/vandenreyt/iphone-backup/internal/tui"
)

func main() {
	dir := flag.String("dir", "", "a backup, or a folder containing backups (default "+backup.DefaultRoot()+")")
	flag.Parse()
	if err := run(*dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(dir string) error {
	app := tui.New(dir)
	defer app.Close()
	_, err := tea.NewProgram(app, tea.WithAltScreen()).Run()
	return err
}
