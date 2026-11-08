package tui

import (
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jensvandenreyt/iphone-backup/internal/export"
	"github.com/jensvandenreyt/iphone-backup/internal/whatsapp"
)

var (
	keySelect = key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "select"))
	keyAll    = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "select all"))
	keyType   = key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "type filter"))
	keyExport = key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "export"))
)

// typeFilters are cycled with "t". A negative value shows every type.
var typeFilters = []whatsapp.MessageType{-1, whatsapp.Image, whatsapp.Video, whatsapp.Audio, whatsapp.Document, whatsapp.GIF, whatsapp.Sticker}

type mediaItem struct {
	m         whatsapp.Message
	available bool
}

func (i mediaItem) FilterValue() string {
	return i.m.Sender + " " + i.m.Type.String() + " " + path.Base(i.m.MediaPath)
}

type mediaDelegate struct{ selected map[int64]bool }

func (d mediaDelegate) Height() int                         { return 1 }
func (d mediaDelegate) Spacing() int                        { return 0 }
func (d mediaDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (d mediaDelegate) Render(w io.Writer, l list.Model, index int, item list.Item) {
	it := item.(mediaItem)
	check := "[ ]"
	if d.selected[it.m.ID] {
		check = "[x]"
	}
	sender := []rune(it.m.Sender)
	if len(sender) > 18 {
		sender = append(sender[:17], '…')
	}
	line := fmt.Sprintf("%s %s  %-8s %-18s %9s  %s", check, it.m.Date.Local().Format("2006-01-02 15:04"),
		it.m.Type, string(sender), humanSize(it.m.MediaSize), path.Base(it.m.MediaPath))
	if !it.available {
		line = dimStyle.Render(line + "  (not in backup)")
	}
	if index == l.Index() {
		fmt.Fprint(w, cursorStyle.Render("> ")+line)
		return
	}
	fmt.Fprint(w, "  "+line)
}

// media lists the media of one chat and lets the user select items to export.
type media struct {
	s        *session
	chat     whatsapp.Chat
	back     screen
	all      []mediaItem
	selected map[int64]bool
	filter   int
	list     list.Model
}

func newMedia(s *session, chat whatsapp.Chat, items []mediaItem, back screen) screen {
	m := &media{s: s, chat: chat, back: back, all: items, selected: map[int64]bool{}}
	m.list = list.New(m.visible(), mediaDelegate{m.selected}, s.width, s.height)
	m.list.DisableQuitKeybindings()
	m.list.SetStatusBarItemName("file", "files")
	m.list.AdditionalShortHelpKeys = func() []key.Binding { return []key.Binding{keySelect, keyAll, keyType, keyExport} }
	m.updateTitle()
	return m
}

func (m *media) visible() []list.Item {
	want := typeFilters[m.filter]
	var items []list.Item
	for _, it := range m.all {
		if want < 0 || it.m.Type == want {
			items = append(items, it)
		}
	}
	return items
}

func (m *media) updateTitle() {
	filter := "all types"
	if t := typeFilters[m.filter]; t >= 0 {
		filter = t.String()
	}
	m.list.Title = fmt.Sprintf("Media · %s · %s · %d selected", m.chat.Name, filter, len(m.selected))
}

func (m *media) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height)
		return m, nil
	case tea.KeyMsg:
		if m.list.FilterState() == list.Filtering {
			break
		}
		if next, cmd, ok := m.handleKey(msg); ok {
			m.updateTitle()
			return next, cmd
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *media) handleKey(msg tea.KeyMsg) (screen, tea.Cmd, bool) {
	switch {
	case msg.String() == "esc" && m.list.FilterState() == list.Unfiltered, msg.String() == "q":
		return m.back, nil, true
	case key.Matches(msg, keySelect):
		if it, ok := m.list.SelectedItem().(mediaItem); ok {
			m.toggle(it.m.ID, !m.selected[it.m.ID])
			m.list.CursorDown()
		}
		return m, nil, true
	case key.Matches(msg, keyAll):
		m.toggleVisible()
		return m, nil, true
	case key.Matches(msg, keyType):
		m.filter = (m.filter + 1) % len(typeFilters)
		return m, m.list.SetItems(m.visible()), true
	case key.Matches(msg, keyExport):
		return m.startExport()
	}
	return nil, nil, false
}

func (m *media) toggle(id int64, on bool) {
	if on {
		m.selected[id] = true
	} else {
		delete(m.selected, id)
	}
}

// toggleVisible selects every visible item, or clears them when all are already selected.
func (m *media) toggleVisible() {
	items := m.list.VisibleItems()
	allOn := true
	for _, it := range items {
		allOn = allOn && m.selected[it.(mediaItem).m.ID]
	}
	for _, it := range items {
		m.toggle(it.(mediaItem).m.ID, !allOn)
	}
}

// startExport exports the selected items, or everything visible when nothing is selected.
func (m *media) startExport() (screen, tea.Cmd, bool) {
	var items []exportItem
	for _, it := range m.all {
		if m.selected[it.m.ID] {
			items = append(items, exportItem{m: it.m})
		}
	}
	if len(items) == 0 {
		for _, it := range m.list.VisibleItems() {
			items = append(items, exportItem{m: it.(mediaItem).m})
		}
	}
	if len(items) == 0 {
		return m, nil, true
	}
	next, cmd := newExporter(m.s, m.chat.Name, filepath.Join("export", export.Sanitize(m.chat.Name)), items, m)
	return next, cmd, true
}

func (m *media) View() string {
	if len(m.all) == 0 {
		return pageStyle.Render(strings.Join([]string{
			titleStyle.Render("Media · " + m.chat.Name), "", "This chat has no media.", "", dimStyle.Render("esc: back"),
		}, "\n"))
	}
	return m.list.View()
}
