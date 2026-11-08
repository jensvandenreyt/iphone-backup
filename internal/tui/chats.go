package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/vandenreyt/iphone-backup/internal/whatsapp"
)

var (
	keyOpen  = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "messages"))
	keyMedia = key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "media"))
)

type chatItem struct{ c whatsapp.Chat }

func (i chatItem) Title() string {
	if i.c.IsGroup {
		return i.c.Name + " (group)"
	}
	return i.c.Name
}

func (i chatItem) Description() string {
	return fmt.Sprintf("%d messages · %d media · last %s",
		i.c.MessageCount, i.c.MediaCount, i.c.LastMessage.Local().Format("2006-01-02 15:04"))
}

func (i chatItem) FilterValue() string { return i.c.Name + " " + i.c.JID }

type chats struct {
	s    *session
	list list.Model
	err  error
}

func newChats(s *session) (screen, tea.Cmd) {
	all, err := s.store.Chats()
	items := make([]list.Item, len(all))
	for i, c := range all {
		items[i] = chatItem{c}
	}
	l := list.New(items, list.NewDefaultDelegate(), s.width, s.height-1)
	l.Title = "WhatsApp chats · " + s.backup.Info.DeviceName
	l.DisableQuitKeybindings()
	l.AdditionalShortHelpKeys = func() []key.Binding { return []key.Binding{keyOpen, keyMedia} }
	return &chats{s: s, list: l, err: err}, nil
}

func (c *chats) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		c.list.SetSize(msg.Width, msg.Height-1)
		return c, nil
	case tea.KeyMsg:
		if c.list.FilterState() == list.Filtering {
			break
		}
		item, ok := c.list.SelectedItem().(chatItem)
		switch {
		case msg.String() == "q":
			return c, tea.Quit
		case ok && key.Matches(msg, keyOpen):
			return c.show(newChatView(c.s, item.c, c))
		case ok && key.Matches(msg, keyMedia):
			return c.show(newMedia(c.s, item.c, c))
		}
	}
	var cmd tea.Cmd
	c.list, cmd = c.list.Update(msg)
	return c, cmd
}

// show switches to next, or stays here and shows err when building it failed.
func (c *chats) show(next screen, err error) (screen, tea.Cmd) {
	if err != nil {
		c.err = err
		return c, nil
	}
	return next, nil
}

func (c *chats) View() string {
	v := c.list.View()
	if c.err != nil {
		v += "\n" + errorStyle.Render(c.err.Error())
	}
	return v
}
