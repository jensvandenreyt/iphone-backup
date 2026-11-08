package tui

import (
	"errors"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/vandenreyt/iphone-backup/internal/backup"
	"github.com/vandenreyt/iphone-backup/internal/whatsapp"
)

type unlockedMsg struct{ store *whatsapp.Store }

type unlockFailedMsg struct{ err error }

// unlock asks for the password of an encrypted backup, then decrypts it and loads WhatsApp.
// Unencrypted backups skip the prompt.
type unlock struct {
	s       *session
	b       *backup.Backup
	back    screen
	input   textinput.Model
	spinner spinner.Model
	busy    bool
	err     error
}

func newUnlock(s *session, b *backup.Backup, back screen) (screen, tea.Cmd) {
	u := &unlock{s: s, b: b, back: back, input: textinput.New(), spinner: spinner.New()}
	u.spinner.Spinner = spinner.Dot
	u.input.Prompt = "Password: "
	u.input.EchoMode = textinput.EchoPassword
	u.input.EchoCharacter = '•'
	if !b.Encrypted {
		return u, u.start("")
	}
	return u, u.input.Focus()
}

func (u *unlock) start(password string) tea.Cmd {
	if u.s.backup != nil && u.s.backup != u.b {
		u.s.backup.Close()
	}
	u.s.backup = u.b
	u.busy, u.err = true, nil
	b := u.b
	return tea.Batch(u.spinner.Tick, func() tea.Msg {
		if err := b.Unlock(password); err != nil {
			return unlockFailedMsg{err}
		}
		store, err := whatsapp.Load(b)
		if err != nil {
			return unlockFailedMsg{err}
		}
		return unlockedMsg{store}
	})
}

func (u *unlock) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case unlockedMsg:
		u.s.store = msg.store
		return newChats(u.s)
	case unlockFailedMsg:
		u.busy, u.err = false, msg.err
		u.input.SetValue("")
		return u, u.input.Focus()
	case spinner.TickMsg:
		if !u.busy {
			return u, nil
		}
		var cmd tea.Cmd
		u.spinner, cmd = u.spinner.Update(msg)
		return u, cmd
	case tea.KeyMsg:
		if u.busy {
			return u, nil
		}
		switch msg.String() {
		case "esc":
			return u.back, nil
		case "enter":
			return u, u.start(u.input.Value())
		}
	}
	var cmd tea.Cmd
	u.input, cmd = u.input.Update(msg)
	return u, cmd
}

func (u *unlock) View() string {
	info := u.b.Info
	lines := []string{
		titleStyle.Render(info.DeviceName),
		dimStyle.Render("iOS " + info.ProductVersion + " · " + u.b.Dir),
		"",
	}
	switch {
	case u.busy && u.b.Encrypted:
		lines = append(lines, u.spinner.View()+" Deriving the key and decrypting, this takes a few seconds...")
	case u.busy:
		lines = append(lines, u.spinner.View()+" Loading WhatsApp data...")
	case u.b.Encrypted:
		lines = append(lines, "This backup is encrypted.", "", u.input.View())
	}
	if u.err != nil {
		msg := u.err.Error()
		if errors.Is(u.err, backup.ErrWrongPassword) {
			msg = "Wrong password, try again."
		}
		lines = append(lines, "", errorStyle.Render(msg))
	}
	lines = append(lines, "", dimStyle.Render("enter: unlock · esc: back · ctrl+c: quit"))
	return pageStyle.Render(strings.Join(lines, "\n"))
}
