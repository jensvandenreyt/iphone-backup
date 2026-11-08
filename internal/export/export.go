// Package export copies WhatsApp media out of a backup into a directory on disk.
package export

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/vandenreyt/iphone-backup/internal/backup"
	"github.com/vandenreyt/iphone-backup/internal/whatsapp"
)

var ErrNotInBackup = errors.New("media file is not in the backup")

// InBackup reports whether the media of m can be exported.
func InBackup(b *backup.Backup, m whatsapp.Message) bool {
	_, err := b.FindFile(whatsapp.Domain, whatsapp.ManifestPath(m.MediaPath))
	return err == nil
}

// Media writes the media of m into dir and returns the path of the new file.
// The file's modification time is set to the message date.
func Media(b *backup.Backup, m whatsapp.Message, dir string) (string, error) {
	f, err := b.FindFile(whatsapp.Domain, whatsapp.ManifestPath(m.MediaPath))
	if errors.Is(err, backup.ErrNotFound) {
		return "", ErrNotInBackup
	}
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dest := uniquePath(filepath.Join(dir, FileName(m)))
	if err := b.ExtractFile(f, dest); err != nil {
		return "", err
	}
	return dest, os.Chtimes(dest, m.Date, m.Date)
}

// FileName builds "<date>_<time>_<sender>_<original name>" for an exported media file.
func FileName(m whatsapp.Message) string {
	name := path.Base(m.MediaPath)
	if m.Type == whatsapp.Document && filepath.Ext(m.MediaTitle) != "" {
		name = m.MediaTitle
	}
	return Sanitize(fmt.Sprintf("%s_%s_%s", m.Date.Local().Format("2006-01-02_15-04-05"), m.Sender, name))
}

// Sanitize replaces characters that are unsafe in file names.
func Sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || strings.ContainsRune(`/\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, s)
}

// uniquePath appends -1, -2, ... before the extension until p does not exist.
func uniquePath(p string) string {
	ext := filepath.Ext(p)
	base := strings.TrimSuffix(p, ext)
	candidate := p
	for i := 1; ; i++ {
		if _, err := os.Lstat(candidate); os.IsNotExist(err) {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d%s", base, i, ext)
	}
}
