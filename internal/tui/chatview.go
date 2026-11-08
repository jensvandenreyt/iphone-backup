package tui

import (
	"fmt"
	"path"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/vandenreyt/iphone-backup/internal/whatsapp"
)

// chatView shows the messages of one chat in a scrollable viewport.
type chatView struct {
	s        *session
	chat     whatsapp.Chat
	back     screen
	messages []whatsapp.Message
	vp       viewport.Model
}

func newChatView(s *session, chat whatsapp.Chat, back screen) (screen, error) {
	msgs, err := s.store.Messages(chat.ID)
	if err != nil {
		return nil, err
	}
	v := &chatView{s: s, chat: chat, back: back, messages: msgs, vp: viewport.New(s.width, s.height-2)}
	v.vp.SetContent(v.render())
	v.vp.GotoBottom()
	return v, nil
}

func (v *chatView) render() string {
	body := lipgloss.NewStyle().Width(max(v.vp.Width-2, 20)).PaddingLeft(2)
	var b strings.Builder
	for _, m := range v.messages {
		sender := senderStyle.Render(m.Sender)
		if m.FromMe {
			sender = meStyle.Render(m.Sender)
		}
		fmt.Fprintf(&b, "%s  %s\n", dimStyle.Render(m.Date.Local().Format("2006-01-02 15:04")), sender)
		b.WriteString(body.Render(messageText(m)) + "\n\n")
	}
	return b.String()
}

func messageText(m whatsapp.Message) string {
	var parts []string
	if m.HasMedia() {
		parts = append(parts, dimStyle.Render(fmt.Sprintf("[%s: %s, %s]", m.Type, path.Base(m.MediaPath), humanSize(m.MediaSize))))
	}
	if m.Text != "" {
		text := m.Text
		if m.Type == whatsapp.System {
			text = dimStyle.Render(text)
		}
		parts = append(parts, text)
	}
	if len(parts) == 0 {
		return dimStyle.Render("[" + m.Type.String() + "]")
	}
	return strings.Join(parts, "\n")
}

func (v *chatView) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.vp.Width, v.vp.Height = msg.Width, msg.Height-2
		v.vp.SetContent(v.render())
		return v, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			return v.back, nil
		case "m":
			next, err := newMedia(v.s, v.chat, v)
			if err != nil {
				return v, nil
			}
			return next, nil
		}
	}
	var cmd tea.Cmd
	v.vp, cmd = v.vp.Update(msg)
	return v, cmd
}

func (v *chatView) View() string {
	title := titleStyle.Render(v.chat.Name) + dimStyle.Render(fmt.Sprintf("  %d messages", len(v.messages)))
	help := dimStyle.Render(fmt.Sprintf("%3.0f%% · ↑/↓ pgup/pgdn: scroll · m: media · esc: back", v.vp.ScrollPercent()*100))
	return title + "\n" + v.vp.View() + "\n" + help
}
