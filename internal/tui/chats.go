package tui

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jensvandenreyt/iphone-backup/internal/whatsapp"
)

var (
	keyOpen  = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "messages"))
	keyMedia = key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "media"))
)

type chatItem struct {
	c        whatsapp.Chat
	selected map[int64]bool
}

func (i chatItem) Title() string {
	check := "[ ] "
	if i.selected[i.c.ID] {
		check = "[x] "
	}
	if i.c.IsGroup {
		return check + i.c.Name + " (group)"
	}
	return check + i.c.Name
}

func (i chatItem) Description() string {
	return fmt.Sprintf("%d messages · %d media · last %s",
		i.c.MessageCount, i.c.MediaCount, i.c.LastMessage.Local().Format("2006-01-02 15:04"))
}

func (i chatItem) FilterValue() string { return i.c.Name + " " + i.c.JID }

// chats lists the chats. Chats can be selected to export all their media at once.
type chats struct {
	s        *session
	list     list.Model
	selected map[int64]bool
	loading  string // status shown while a chat loads in the background
	err      error
}

func newChats(s *session) (screen, tea.Cmd) {
	c := &chats{s: s, selected: map[int64]bool{}}
	all, err := s.store.Chats()
	items := make([]list.Item, len(all))
	for i, chat := range all {
		items[i] = chatItem{chat, c.selected}
	}
	c.list = list.New(items, list.NewDefaultDelegate(), s.width, s.height-1)
	c.list.DisableQuitKeybindings()
	c.list.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{keyOpen, keyMedia, keySelect, keyAll, keyExport}
	}
	c.err = err
	c.updateTitle()
	return c, nil
}

func (c *chats) updateTitle() {
	c.list.Title = fmt.Sprintf("WhatsApp chats · %s · %d selected", c.s.backup.Info.DeviceName, len(c.selected))
}

func (c *chats) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		c.list.SetSize(msg.Width, msg.Height-1)
		return c, nil
	case messagesLoadedMsg:
		if c.loaded(msg.err) {
			return newChatView(c.s, msg.chat, msg.messages, c), nil
		}
		return c, nil
	case mediaLoadedMsg:
		if c.loaded(msg.err) {
			return newMedia(c.s, msg.chat, msg.items, c), nil
		}
		return c, nil
	case bulkLoadedMsg:
		if !c.loaded(msg.err) {
			return c, nil
		}
		if len(msg.items) == 0 {
			c.err = errors.New("the selected chats have no media")
			return c, nil
		}
		return newExporter(c.s, fmt.Sprintf("%d chats", msg.chats), "export", msg.items, c)
	case tea.KeyMsg:
		if c.list.FilterState() == list.Filtering {
			break
		}
		item, ok := c.list.SelectedItem().(chatItem)
		switch {
		case msg.String() == "q":
			return c, tea.Quit
		case c.loading != "" && (key.Matches(msg, keyOpen) || key.Matches(msg, keyMedia) || key.Matches(msg, keyExport)):
			return c, nil
		case ok && key.Matches(msg, keyOpen):
			c.loading = fmt.Sprintf("Loading %d messages from %s...", item.c.MessageCount, item.c.Name)
			return c, loadMessages(c.s, item.c)
		case ok && key.Matches(msg, keyMedia):
			c.loading = fmt.Sprintf("Loading %d media files from %s...", item.c.MediaCount, item.c.Name)
			return c, loadMedia(c.s, item.c, nil)
		case ok && key.Matches(msg, keySelect):
			c.toggle(item.c.ID, !c.selected[item.c.ID])
			c.list.CursorDown()
			c.updateTitle()
			return c, nil
		case key.Matches(msg, keyAll):
			c.toggleVisible()
			c.updateTitle()
			return c, nil
		case key.Matches(msg, keyExport):
			return c.startExport()
		}
	}
	var cmd tea.Cmd
	c.list, cmd = c.list.Update(msg)
	return c, cmd
}

func (c *chats) toggle(id int64, on bool) {
	if on {
		c.selected[id] = true
	} else {
		delete(c.selected, id)
	}
}

// toggleVisible selects every visible chat, or clears them when all are already selected.
func (c *chats) toggleVisible() {
	items := c.list.VisibleItems()
	allOn := true
	for _, it := range items {
		allOn = allOn && c.selected[it.(chatItem).c.ID]
	}
	for _, it := range items {
		c.toggle(it.(chatItem).c.ID, !allOn)
	}
}

// startExport loads the media of the selected chats, or of all visible chats when none are selected.
func (c *chats) startExport() (screen, tea.Cmd) {
	var picked []whatsapp.Chat
	for _, it := range c.list.Items() {
		if ch := it.(chatItem).c; c.selected[ch.ID] && ch.MediaCount > 0 {
			picked = append(picked, ch)
		}
	}
	if len(c.selected) == 0 {
		for _, it := range c.list.VisibleItems() {
			if ch := it.(chatItem).c; ch.MediaCount > 0 {
				picked = append(picked, ch)
			}
		}
	}
	if len(picked) == 0 {
		c.err = errors.New("the selected chats have no media")
		return c, nil
	}
	c.loading, c.err = fmt.Sprintf("Loading media from %d chats...", len(picked)), nil
	return c, loadBulkExport(c.s, picked)
}

// loaded ends the loading state and reports whether a background load this screen started succeeded.
func (c *chats) loaded(err error) bool {
	if c.loading == "" {
		return false
	}
	c.loading, c.err = "", err
	return err == nil
}

func (c *chats) View() string {
	v := c.list.View()
	switch {
	case c.loading != "":
		v += "\n" + c.loading
	case c.err != nil:
		v += "\n" + errorStyle.Render(c.err.Error())
	}
	return v
}
