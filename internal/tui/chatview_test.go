package tui

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jensvandenreyt/iphone-backup/internal/backup"
	"github.com/jensvandenreyt/iphone-backup/internal/whatsapp"
)

func TestChatViewRendersOnlyWhatFits(t *testing.T) {
	v := testChatView(100000)
	lines, shown := v.lines()
	if len(lines) > v.avail() {
		t.Errorf("got %d lines, screen fits %d", len(lines), v.avail())
	}
	if shown > v.avail() {
		t.Errorf("rendered %d messages for a %d-line screen", shown, v.avail())
	}
	if !strings.Contains(strings.Join(lines, "\n"), "message 99999") {
		t.Errorf("newest message not shown:\n%s", strings.Join(lines, "\n"))
	}
}

func TestChatViewScrolling(t *testing.T) {
	v := testChatView(1000)
	last := len(v.messages) - 1
	steps := []struct {
		key  string
		want func(bottom int) bool
	}{
		{"up", func(b int) bool { return b == last-1 }},
		{"end", func(b int) bool { return b == last }},
		{"pgup", func(b int) bool { return b < last-1 && b > last-v.avail() }},
		{"home", func(b int) bool { return b > 0 && b < v.avail() }},
		{"up", func(b int) bool { return b >= 0 }},
		{"down", func(b int) bool { return b > 0 }},
	}
	for _, s := range steps {
		v.Update(keyMsg(s.key))
		if !s.want(v.bottom) {
			t.Fatalf("after %q: bottom = %d", s.key, v.bottom)
		}
	}
	v.Update(keyMsg("home"))
	lines, _ := v.lines()
	if !strings.Contains(lines[0], "Member") || strings.TrimSpace(lines[1]) != "message 0" {
		t.Errorf("home does not start at the first message:\n%s", strings.Join(lines, "\n"))
	}
}

func TestChatViewEmptyChat(t *testing.T) {
	v := testChatView(0)
	for _, k := range []string{"up", "down", "pgup", "pgdown", "home", "end"} {
		v.Update(keyMsg(k))
	}
	v.View()
}

func TestChatsLoadMessagesInBackground(t *testing.T) {
	s := &session{width: 80, height: 24, store: testStore(t, 3), backup: &backup.Backup{}}
	c, _ := newChats(s)

	next, cmd := c.Update(keyMsg("enter"))
	if next != c || cmd == nil {
		t.Fatalf("enter switched to %T synchronously, want a background load", next)
	}
	if !strings.Contains(c.View(), "Loading") {
		t.Errorf("view does not show loading state:\n%s", c.View())
	}
	next, _ = next.Update(cmd())
	v, ok := next.(*chatView)
	if !ok || len(v.messages) != 3 {
		t.Fatalf("after load: screen = %T", next)
	}
}

func testChatView(n int) *chatView {
	msgs := make([]whatsapp.Message, n)
	for i := range msgs {
		msgs[i] = whatsapp.Message{ID: int64(i), Date: time.Unix(int64(i), 0), Sender: "Member", Text: fmt.Sprintf("message %d", i)}
	}
	s := &session{width: 80, height: 20}
	return newChatView(s, whatsapp.Chat{Name: "Big"}, msgs, nil).(*chatView)
}

func keyMsg(k string) tea.KeyMsg {
	switch k {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "pgup":
		return tea.KeyMsg{Type: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyMsg{Type: tea.KeyPgDown}
	case "home":
		return tea.KeyMsg{Type: tea.KeyHome}
	case "end":
		return tea.KeyMsg{Type: tea.KeyEnd}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

// testStore creates a ChatStorage.sqlite with chat Alice (n text messages, listed first) and two chats
// named Bob with 2 and 1 media messages.
func testStore(t *testing.T, n int) *whatsapp.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ChatStorage.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	stmts := []string{
		`CREATE TABLE ZWACHATSESSION (Z_PK INTEGER PRIMARY KEY, ZCONTACTJID VARCHAR, ZPARTNERNAME VARCHAR, ZSESSIONTYPE INTEGER, ZLASTMESSAGEDATE TIMESTAMP)`,
		`CREATE TABLE ZWAGROUPMEMBER (Z_PK INTEGER PRIMARY KEY, ZMEMBERJID VARCHAR, ZCONTACTNAME VARCHAR)`,
		`CREATE TABLE ZWAPROFILEPUSHNAME (Z_PK INTEGER PRIMARY KEY, ZJID VARCHAR, ZPUSHNAME VARCHAR)`,
		`CREATE TABLE ZWAMEDIAITEM (Z_PK INTEGER PRIMARY KEY, ZMEDIALOCALPATH VARCHAR, ZTITLE VARCHAR, ZFILESIZE INTEGER)`,
		`CREATE TABLE ZWAMESSAGE (Z_PK INTEGER PRIMARY KEY, ZCHATSESSION INTEGER, ZISFROMME INTEGER, ZMESSAGETYPE INTEGER,
			ZTEXT VARCHAR, ZMESSAGEDATE TIMESTAMP, ZGROUPMEMBER INTEGER, ZMEDIAITEM INTEGER)`,
		`INSERT INTO ZWACHATSESSION VALUES (1, 'alice@s.whatsapp.net', 'Alice', 0, 100), (2, 'bob@s.whatsapp.net', 'Bob', 0, 50),
			(3, 'bob2@s.whatsapp.net', 'Bob', 0, 40)`,
		`INSERT INTO ZWAMEDIAITEM VALUES (1, 'Media/bob/a.jpg', NULL, 10), (2, 'Media/bob/b.jpg', NULL, 20), (3, 'Media/bob2/c.jpg', NULL, 30)`,
		`INSERT INTO ZWAMESSAGE VALUES (1001, 2, 0, 1, NULL, 1, NULL, 1), (1002, 2, 0, 1, NULL, 2, NULL, 2), (1003, 3, 0, 1, NULL, 3, NULL, 3)`,
	}
	for i := 1; i <= n; i++ {
		stmts = append(stmts, fmt.Sprintf(`INSERT INTO ZWAMESSAGE VALUES (%d, 1, 0, 0, 'hi %d', %d, NULL, NULL)`, i, i, i))
	}
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	store, err := whatsapp.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestChatsSelection(t *testing.T) {
	s := &session{width: 80, height: 24, store: testStore(t, 3), backup: &backup.Backup{}}
	next, _ := newChats(s)
	c := next.(*chats)

	c.Update(keyMsg(" "))
	if len(c.selected) != 1 || !strings.Contains(c.list.Title, "1 selected") || !strings.Contains(c.list.View(), "[x]") {
		t.Fatalf("after space: selected=%v title=%q", c.selected, c.list.Title)
	}
	c.Update(keyMsg("a"))
	if len(c.selected) != 3 {
		t.Fatalf("after a: selected=%v, want all 3 chats", c.selected)
	}
	c.Update(keyMsg("a"))
	if len(c.selected) != 0 {
		t.Fatalf("after second a: selected=%v, want none", c.selected)
	}
}

func TestChatsExportAllMediaIntoFolders(t *testing.T) {
	s := &session{width: 80, height: 24, store: testStore(t, 3), backup: &backup.Backup{}}
	c, _ := newChats(s)

	next, cmd := c.Update(keyMsg("e"))
	if next != c || cmd == nil {
		t.Fatalf("e switched to %T synchronously, want a background load", next)
	}
	next, _ = next.Update(cmd())
	e, ok := next.(*exporter)
	if !ok {
		t.Fatalf("after load: screen = %T, want exporter", next)
	}
	folders := map[string]int{}
	for _, it := range e.items {
		folders[it.folder]++
	}
	want := map[string]int{"Bob (2)": 2, "Bob (3)": 1}
	if fmt.Sprint(folders) != fmt.Sprint(want) {
		t.Errorf("files per folder = %v, want %v", folders, want)
	}
	if e.input.Value() != "export" {
		t.Errorf("default target = %q, want export", e.input.Value())
	}
}

func TestChatFolders(t *testing.T) {
	got := chatFolders([]whatsapp.Chat{{ID: 1, Name: "Alice"}, {ID: 2, Name: "Work/Team"}, {ID: 3, Name: "Alice"}, {ID: 4, Name: ".."}})
	want := map[int64]string{1: "Alice (1)", 2: "Work_Team", 3: "Alice (3)", 4: "4"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("chatFolders = %v, want %v", got, want)
	}
}
