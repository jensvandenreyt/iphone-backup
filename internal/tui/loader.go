package tui

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jensvandenreyt/iphone-backup/internal/backup"
	"github.com/jensvandenreyt/iphone-backup/internal/export"
	"github.com/jensvandenreyt/iphone-backup/internal/whatsapp"
)

type messagesLoadedMsg struct {
	chat     whatsapp.Chat
	messages []whatsapp.Message
	err      error
}

type mediaLoadedMsg struct {
	chat  whatsapp.Chat
	items []mediaItem
	err   error
}

type bulkLoadedMsg struct {
	chats int
	items []exportItem
	err   error
}

// loadMessages queries the messages of chat in the background.
func loadMessages(s *session, chat whatsapp.Chat) tea.Cmd {
	store := s.store
	slog.Info("loading messages", "chat", chat.Name, "chatID", chat.ID, "expected", chat.MessageCount)
	return func() tea.Msg {
		msgs, err := store.Messages(chat.ID)
		return messagesLoadedMsg{chat, msgs, err}
	}
}

// loadMedia collects the media of chat in the background and checks which files are in the backup.
// When msgs is nil, the messages are queried first.
func loadMedia(s *session, chat whatsapp.Chat, msgs []whatsapp.Message) tea.Cmd {
	store, b := s.store, s.backup
	slog.Info("loading media", "chat", chat.Name, "chatID", chat.ID, "expected", chat.MediaCount)
	return func() tea.Msg {
		if msgs == nil {
			var err error
			if msgs, err = store.Media(chat.ID); err != nil {
				return mediaLoadedMsg{chat: chat, err: err}
			}
		}
		items, err := mediaItems(b, chat, msgs)
		return mediaLoadedMsg{chat, items, err}
	}
}

// loadBulkExport collects all media of chats in the background, with one export folder per chat.
func loadBulkExport(s *session, chats []whatsapp.Chat) tea.Cmd {
	store := s.store
	slog.Info("loading media for export", "chats", len(chats))
	return func() tea.Msg {
		start := time.Now()
		folders := chatFolders(chats)
		var items []exportItem
		for _, c := range chats {
			msgs, err := store.Media(c.ID)
			if err != nil {
				return bulkLoadedMsg{err: err}
			}
			for _, m := range msgs {
				items = append(items, exportItem{m, folders[c.ID]})
			}
		}
		slog.Info("loaded media for export", "chats", len(chats), "files", len(items),
			"took", time.Since(start).Round(time.Millisecond))
		return bulkLoadedMsg{len(chats), items, nil}
	}
}

// chatFolders names one export folder per chat. Chats that share a name get their ID appended.
func chatFolders(chats []whatsapp.Chat) map[int64]string {
	names := map[int64]string{}
	count := map[string]int{}
	for _, c := range chats {
		name := strings.TrimSpace(export.Sanitize(c.Name))
		if name == "" || name == "." || name == ".." {
			name = fmt.Sprint(c.ID)
		}
		names[c.ID] = name
		count[name]++
	}
	for id, name := range names {
		if count[name] > 1 {
			names[id] = fmt.Sprintf("%s (%d)", name, id)
		}
	}
	return names
}

func mediaItems(b *backup.Backup, chat whatsapp.Chat, msgs []whatsapp.Message) ([]mediaItem, error) {
	start := time.Now()
	slog.Info("checking media", "chat", chat.Name, "chatID", chat.ID, "messages", len(msgs))
	set, err := export.LoadMediaSet(b)
	if err != nil {
		return nil, err
	}
	var items []mediaItem
	available := 0
	for _, m := range msgs {
		if !m.HasMedia() {
			continue
		}
		it := mediaItem{m, set.Has(m)}
		if it.available {
			available++
		}
		items = append(items, it)
	}
	slog.Info("checked media", "chat", chat.Name, "chatID", chat.ID, "files", len(items), "inBackup", available,
		"took", time.Since(start).Round(time.Millisecond))
	return items, nil
}
