package whatsapp

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestChats(t *testing.T) {
	chats, err := openFixture(t).Chats()
	if err != nil {
		t.Fatal(err)
	}
	if len(chats) != 2 {
		t.Fatalf("got %d chats, want 2 (empty chat excluded)", len(chats))
	}
	family, alice := chats[0], chats[1]
	if family.Name != "Family" || !family.IsGroup {
		t.Errorf("first chat = %+v, want group Family", family)
	}
	if alice.Name != "Alice" || alice.IsGroup || alice.MessageCount != 2 || alice.MediaCount != 1 {
		t.Errorf("second chat = %+v", alice)
	}
	if want := time.Date(2001, 1, 1, 0, 1, 40, 0, time.UTC); !alice.LastMessage.Equal(want) {
		t.Errorf("LastMessage = %v, want %v", alice.LastMessage, want)
	}
}

func TestMessagesResolveSenders(t *testing.T) {
	s := openFixture(t)
	tests := []struct {
		chat    int64
		senders []string
	}{
		{1, []string{"Alice", "Me"}},
		{2, []string{"Bob", "Carol"}},
	}
	for _, tt := range tests {
		msgs, err := s.Messages(tt.chat)
		if err != nil {
			t.Fatal(err)
		}
		if len(msgs) != len(tt.senders) {
			t.Fatalf("chat %d: got %d messages", tt.chat, len(msgs))
		}
		for i, m := range msgs {
			if m.Sender != tt.senders[i] {
				t.Errorf("chat %d message %d: sender = %q, want %q", tt.chat, i, m.Sender, tt.senders[i])
			}
		}
	}
}

func TestMessagesIncludeMedia(t *testing.T) {
	msgs, err := openFixture(t).Messages(1)
	if err != nil {
		t.Fatal(err)
	}
	img := msgs[1]
	if !img.HasMedia() || img.Type != Image || img.MediaSize != 1234 || !img.FromMe {
		t.Errorf("media message = %+v", img)
	}
	if msgs[0].HasMedia() {
		t.Errorf("text message has media: %+v", msgs[0])
	}
}

func TestManifestPath(t *testing.T) {
	tests := map[string]string{
		"Media/a@s.whatsapp.net/1/2/x.jpg":          "Message/Media/a@s.whatsapp.net/1/2/x.jpg",
		"/Media/a@s.whatsapp.net/1/2/x.jpg":         "Message/Media/a@s.whatsapp.net/1/2/x.jpg",
		"Message/Media/a@s.whatsapp.net/1/2/x.jpg":  "Message/Media/a@s.whatsapp.net/1/2/x.jpg",
		"/Message/Media/a@s.whatsapp.net/1/2/x.jpg": "Message/Media/a@s.whatsapp.net/1/2/x.jpg",
	}
	for in, want := range tests {
		if got := ManifestPath(in); got != want {
			t.Errorf("ManifestPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func openFixture(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), dbName)
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
		`INSERT INTO ZWACHATSESSION VALUES (1, 'alice@s.whatsapp.net', 'Alice', 0, 100), (2, '123-456@g.us', 'Family', 1, 200),
			(3, 'empty@s.whatsapp.net', 'Empty', 0, 300)`,
		`INSERT INTO ZWAGROUPMEMBER VALUES (1, 'bob@s.whatsapp.net', ''), (2, 'carol@s.whatsapp.net', 'Carol')`,
		`INSERT INTO ZWAPROFILEPUSHNAME VALUES (1, 'bob@s.whatsapp.net', 'Bob')`,
		`INSERT INTO ZWAMEDIAITEM VALUES (1, 'Media/alice@s.whatsapp.net/a/b/x.jpg', NULL, 1234), (2, '', 'report.pdf', 99)`,
		`INSERT INTO ZWAMESSAGE VALUES
			(1, 1, 0, 0, 'hi', 10, NULL, NULL),
			(2, 1, 1, 1, NULL, 20, NULL, 1),
			(3, 2, 0, 0, 'hello family', 30, 1, NULL),
			(4, 2, 0, 8, NULL, 40, 2, 2)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMediaReturnsOnlyMessagesWithFiles(t *testing.T) {
	s := openFixture(t)
	tests := []struct {
		chat int64
		want int
	}{
		{1, 1}, // one image, one text message
		{2, 0}, // a document without a local file, one text message
	}
	for _, tt := range tests {
		msgs, err := s.Media(tt.chat)
		if err != nil {
			t.Fatal(err)
		}
		if len(msgs) != tt.want {
			t.Errorf("chat %d: got %d media messages, want %d", tt.chat, len(msgs), tt.want)
		}
		for _, m := range msgs {
			if !m.HasMedia() {
				t.Errorf("chat %d: message %d has no media", tt.chat, m.ID)
			}
		}
	}
}
