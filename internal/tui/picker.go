package tui

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/vandenreyt/iphone-backup/internal/backup"
)

// pickerItem is a backup, or the "open other directory" entry when b is nil.
type pickerItem struct{ b *backup.Backup }

func (i pickerItem) Title() string {
	if i.b == nil {
		return "Open other directory..."
	}
	return i.b.Info.DeviceName
}

func (i pickerItem) Description() string {
	if i.b == nil {
		return "Path to a backup, or to a folder containing backups"
	}
	lock := "not encrypted"
	if i.b.Encrypted {
		lock = "encrypted"
	}
	return fmt.Sprintf("iOS %s · %s · %s · %s",
		i.b.Info.ProductVersion, i.b.Info.ProductType, i.b.Info.LastBackup.Local().Format("2006-01-02 15:04"), lock)
}

func (i pickerItem) FilterValue() string { return i.Title() }

type picker struct {
	s      *session
	list   list.Model
	input  textinput.Model
	typing bool
	err    error
}

func newPicker(s *session, dir string) (screen, tea.Cmd) {
	p := &picker{s: s, input: textinput.New()}
	p.input.Prompt = "Directory: "
	p.input.Placeholder = backup.DefaultRoot()
	p.list = list.New([]list.Item{pickerItem{}}, list.NewDefaultDelegate(), s.width, s.height-3)
	p.list.DisableQuitKeybindings()
	if dir == "" {
		dir = backup.DefaultRoot()
	}
	return p.open(dir)
}

// open goes straight to the unlock screen when dir is a backup, and lists its backups otherwise.
func (p *picker) open(dir string) (screen, tea.Cmd) {
	p.list.Title = "iPhone backups in " + dir
	if backup.IsBackup(dir) {
		b, err := backup.Open(dir)
		if err != nil {
			p.err = err
			return p, nil
		}
		return newUnlock(p.s, b, p)
	}
	backups, err := backup.List(dir)
	p.err = err
	if err == nil && len(backups) == 0 {
		p.err = errors.New("no backups found in " + dir)
	}
	items := []list.Item{}
	for _, b := range backups {
		items = append(items, pickerItem{b})
	}
	items = append(items, pickerItem{})
	return p, p.list.SetItems(items)
}

func (p *picker) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.list.SetSize(msg.Width, msg.Height-3)
		return p, nil
	case tea.KeyMsg:
		if p.typing {
			return p.updateInput(msg)
		}
		if p.list.FilterState() == list.Filtering {
			break
		}
		switch msg.String() {
		case "q":
			return p, tea.Quit
		case "enter":
			return p.choose()
		}
	}
	var cmd tea.Cmd
	if p.typing {
		p.input, cmd = p.input.Update(msg)
	} else {
		p.list, cmd = p.list.Update(msg)
	}
	return p, cmd
}

func (p *picker) choose() (screen, tea.Cmd) {
	item, ok := p.list.SelectedItem().(pickerItem)
	if !ok {
		return p, nil
	}
	if item.b == nil {
		p.typing, p.err = true, nil
		return p, p.input.Focus()
	}
	return newUnlock(p.s, item.b, p)
}

func (p *picker) updateInput(msg tea.KeyMsg) (screen, tea.Cmd) {
	switch msg.String() {
	case "esc":
		p.typing = false
		p.input.Blur()
		return p, nil
	case "enter":
		p.typing = false
		p.input.Blur()
		return p.open(expandHome(strings.TrimSpace(p.input.Value())))
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	return p, cmd
}

func (p *picker) View() string {
	var b strings.Builder
	b.WriteString(p.list.View())
	if p.typing {
		b.WriteString("\n" + p.input.View())
	}
	if p.err != nil {
		b.WriteString("\n" + errorStyle.Render(errText(p.err)))
	}
	return b.String()
}

func errText(err error) string {
	if errors.Is(err, fs.ErrPermission) {
		return err.Error() + "\nGive your terminal Full Disk Access in System Settings > Privacy & Security, then restart it."
	}
	return err.Error()
}
