// Package tui is the full-screen terminal interface for browsing WhatsApp data in a backup.
package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vandenreyt/iphone-backup/internal/backup"
	"github.com/vandenreyt/iphone-backup/internal/whatsapp"
)

// screen is one view of the app. Update returns the screen to show next.
type screen interface {
	Update(tea.Msg) (screen, tea.Cmd)
	View() string
}

// session is the state shared between screens.
type session struct {
	width, height int
	backup        *backup.Backup
	store         *whatsapp.Store
}

func (s *session) close() {
	if s.store != nil {
		s.store.Close()
	}
	if s.backup != nil {
		s.backup.Close()
	}
}

type App struct {
	s    *session
	cur  screen
	init tea.Cmd
}

// New creates the app. dir may be empty (the default backup folder), a backup, or a folder of backups.
func New(dir string) *App {
	s := &session{}
	cur, cmd := newPicker(s, dir)
	return &App{s: s, cur: cur, init: cmd}
}

func (a *App) Init() tea.Cmd { return a.init }

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}
	case tea.WindowSizeMsg:
		a.s.width, a.s.height = msg.Width, msg.Height
	}
	var cmd tea.Cmd
	a.cur, cmd = a.cur.Update(msg)
	return a, cmd
}

func (a *App) View() string { return a.cur.View() }

// Close removes all decrypted temporary files.
func (a *App) Close() { a.s.close() }

func expandHome(p string) string {
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, rest)
	}
	return p
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}
