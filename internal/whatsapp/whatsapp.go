// Package whatsapp reads chats and media references from WhatsApp's ChatStorage.sqlite.
package whatsapp

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/vandenreyt/iphone-backup/internal/backup"
)

// Domain is the backup domain of WhatsApp's shared app group container.
const Domain = "AppDomainGroup-group.net.whatsapp.WhatsApp.shared"

const dbName = "ChatStorage.sqlite"

// coreDataEpoch is 2001-01-01 UTC, the reference date for Core Data timestamps.
const coreDataEpoch = 978307200

var ErrNotFound = errors.New("WhatsApp data not found in this backup")

type MessageType int

const (
	Text     MessageType = 0
	Image    MessageType = 1
	Video    MessageType = 2
	Audio    MessageType = 3
	Contact  MessageType = 4
	Location MessageType = 5
	System   MessageType = 6
	Link     MessageType = 7
	Document MessageType = 8
	GIF      MessageType = 11
	Sticker  MessageType = 15
)

func (t MessageType) String() string {
	switch t {
	case Text:
		return "text"
	case Image:
		return "image"
	case Video:
		return "video"
	case Audio:
		return "audio"
	case Contact:
		return "contact"
	case Location:
		return "location"
	case System:
		return "system"
	case Link:
		return "link"
	case Document:
		return "document"
	case GIF:
		return "gif"
	case Sticker:
		return "sticker"
	}
	return "other"
}

type Chat struct {
	ID           int64
	Name         string
	JID          string
	IsGroup      bool
	LastMessage  time.Time
	MessageCount int
	MediaCount   int
}

type Message struct {
	ID         int64
	Date       time.Time
	FromMe     bool
	Sender     string
	Type       MessageType
	Text       string
	MediaPath  string
	MediaTitle string
	MediaSize  int64
}

func (m Message) HasMedia() bool { return m.MediaPath != "" }

type Store struct {
	db *sql.DB
}

// Load extracts ChatStorage.sqlite (and its WAL, if present) from an unlocked backup and opens it.
func Load(b *backup.Backup) (*Store, error) {
	for _, name := range []string{dbName, dbName + "-wal"} {
		f, err := b.FindFile(Domain, name)
		if errors.Is(err, backup.ErrNotFound) {
			if name == dbName {
				return nil, ErrNotFound
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		if err := b.ExtractFile(f, b.TempPath(name)); err != nil {
			return nil, err
		}
	}
	return Open(b.TempPath(dbName))
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Chats returns all chats that have messages, most recently active first.
func (s *Store) Chats() ([]Chat, error) {
	rows, err := s.db.Query(`
		SELECT s.Z_PK, COALESCE(s.ZPARTNERNAME, ''), COALESCE(s.ZCONTACTJID, ''),
			COALESCE(s.ZSESSIONTYPE, 0), COALESCE(s.ZLASTMESSAGEDATE, 0),
			COUNT(m.Z_PK), COUNT(NULLIF(mi.ZMEDIALOCALPATH, ''))
		FROM ZWACHATSESSION s
		JOIN ZWAMESSAGE m ON m.ZCHATSESSION = s.Z_PK
		LEFT JOIN ZWAMEDIAITEM mi ON mi.Z_PK = m.ZMEDIAITEM
		GROUP BY s.Z_PK
		ORDER BY s.ZLASTMESSAGEDATE DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var chats []Chat
	for rows.Next() {
		var c Chat
		var sessionType int
		var last float64
		if err := rows.Scan(&c.ID, &c.Name, &c.JID, &sessionType, &last, &c.MessageCount, &c.MediaCount); err != nil {
			return nil, err
		}
		c.IsGroup = sessionType == 1
		c.LastMessage = coreDataTime(last)
		if c.Name == "" {
			c.Name = c.JID
		}
		chats = append(chats, c)
	}
	return chats, rows.Err()
}

// Messages returns the messages of a chat in chronological order.
func (s *Store) Messages(chatID int64) ([]Message, error) {
	rows, err := s.db.Query(`
		SELECT m.Z_PK, COALESCE(m.ZMESSAGEDATE, 0), COALESCE(m.ZISFROMME, 0),
			COALESCE(m.ZMESSAGETYPE, 0), COALESCE(m.ZTEXT, ''),
			CASE WHEN m.ZISFROMME = 1 THEN 'Me' ELSE COALESCE(
				NULLIF(gm.ZCONTACTNAME, ''), NULLIF(pn.ZPUSHNAME, ''), gm.ZMEMBERJID, s.ZPARTNERNAME, ''
			) END,
			COALESCE(mi.ZMEDIALOCALPATH, ''), COALESCE(mi.ZTITLE, ''), COALESCE(mi.ZFILESIZE, 0)
		FROM ZWAMESSAGE m
		JOIN ZWACHATSESSION s ON s.Z_PK = m.ZCHATSESSION
		LEFT JOIN ZWAGROUPMEMBER gm ON gm.Z_PK = m.ZGROUPMEMBER
		LEFT JOIN ZWAPROFILEPUSHNAME pn ON pn.ZJID = gm.ZMEMBERJID
		LEFT JOIN ZWAMEDIAITEM mi ON mi.Z_PK = m.ZMEDIAITEM
		WHERE m.ZCHATSESSION = ?
		ORDER BY m.ZMESSAGEDATE, m.Z_PK`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var msgs []Message
	for rows.Next() {
		var m Message
		var date float64
		if err := rows.Scan(&m.ID, &date, &m.FromMe, &m.Type, &m.Text, &m.Sender, &m.MediaPath, &m.MediaTitle, &m.MediaSize); err != nil {
			return nil, err
		}
		m.Date = coreDataTime(date)
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// ManifestPath maps a ZMEDIALOCALPATH value to its relativePath in the backup manifest.
func ManifestPath(localPath string) string {
	p := strings.TrimPrefix(localPath, "/")
	if strings.HasPrefix(p, "Message/") {
		return p
	}
	return "Message/" + p
}

func coreDataTime(secs float64) time.Time {
	return time.Unix(coreDataEpoch, 0).Add(time.Duration(secs * float64(time.Second)))
}
