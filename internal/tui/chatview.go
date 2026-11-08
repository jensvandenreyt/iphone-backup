package tui

import (
	"fmt"
	"path"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jensvandenreyt/iphone-backup/internal/whatsapp"
)

// chatView shows the messages of one chat. Only the messages that fit on screen are rendered,
// so chats with hundreds of thousands of messages stay responsive.
type chatView struct {
	s        *session
	chat     whatsapp.Chat
	back     screen
	messages []whatsapp.Message
	bottom   int    // index of the message at the bottom of the screen
	loading  string // status shown while media loads in the background
	err      error
}

func newChatView(s *session, chat whatsapp.Chat, msgs []whatsapp.Message, back screen) screen {
	return &chatView{s: s, chat: chat, back: back, messages: msgs, bottom: len(msgs) - 1}
}

// avail is the number of lines between the title and the help line.
func (v *chatView) avail() int { return max(v.s.height-2, 1) }

// block renders message i as a header, its wrapped body and a blank separator line.
func (v *chatView) block(i int) []string {
	m := v.messages[i]
	sender := senderStyle.Render(m.Sender)
	if m.FromMe {
		sender = meStyle.Render(m.Sender)
	}
	header := dimStyle.Render(m.Date.Local().Format("2006-01-02 15:04")) + "  " + sender
	body := lipgloss.NewStyle().Width(max(v.s.width-2, 20)).PaddingLeft(2).Render(messageText(m))
	lines := append([]string{header}, strings.Split(body, "\n")...)
	return append(lines, "")
}

// lines returns the screen lines ending with message v.bottom and how many messages were rendered.
func (v *chatView) lines() ([]string, int) {
	var lines []string
	shown := 0
	for i := min(v.bottom, len(v.messages)-1); i >= 0 && len(lines) < v.avail(); i-- {
		lines = append(v.block(i), lines...)
		shown++
	}
	return lines[max(len(lines)-v.avail(), 0):], shown
}

// firstScreen returns the bottom index that shows the first message at the top of the screen.
func (v *chatView) firstScreen() int {
	height, bottom := 0, 0
	for i := range v.messages {
		height += len(v.block(i))
		if height > v.avail() {
			break
		}
		bottom = i
	}
	return bottom
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
	case mediaLoadedMsg:
		if v.loading == "" {
			return v, nil
		}
		v.loading = ""
		if msg.err != nil {
			v.err = msg.err
			return v, nil
		}
		return newMedia(v.s, v.chat, msg.items, v), nil
	case tea.KeyMsg:
		return v.handleKey(msg)
	}
	return v, nil
}

func (v *chatView) handleKey(msg tea.KeyMsg) (screen, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		return v.back, nil
	case "m":
		if v.loading == "" {
			v.loading = fmt.Sprintf("Checking %d media files...", v.chat.MediaCount)
			return v, loadMedia(v.s, v.chat, v.messages)
		}
	case "up", "k":
		v.bottom--
	case "down", "j":
		v.bottom++
	case "pgup", "b":
		_, shown := v.lines()
		v.bottom -= max(shown-1, 1)
	case "pgdown", "f", " ":
		_, shown := v.lines()
		v.bottom += max(shown-1, 1)
	case "home", "g":
		v.bottom = v.firstScreen()
	case "end", "G":
		v.bottom = len(v.messages) - 1
	}
	v.bottom = max(min(v.bottom, len(v.messages)-1), 0)
	return v, nil
}

func (v *chatView) View() string {
	lines, _ := v.lines()
	pad := make([]string, v.avail()-len(lines))
	title := titleStyle.Render(v.chat.Name) + dimStyle.Render(fmt.Sprintf("  %d messages", len(v.messages)))
	status := dimStyle.Render(fmt.Sprintf("message %d of %d · ↑/↓ pgup/pgdn home/end: scroll · m: media · esc: back",
		min(v.bottom+1, len(v.messages)), len(v.messages)))
	switch {
	case v.loading != "":
		status = v.loading
	case v.err != nil:
		status = errorStyle.Render(v.err.Error())
	}
	return title + "\n" + strings.Join(append(pad, lines...), "\n") + "\n" + status
}
