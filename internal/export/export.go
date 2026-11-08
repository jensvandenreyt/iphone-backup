// Package export copies WhatsApp media out of a backup into a directory on disk.
package export

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/jensvandenreyt/iphone-backup/internal/applog"
	"github.com/jensvandenreyt/iphone-backup/internal/backup"
	"github.com/jensvandenreyt/iphone-backup/internal/whatsapp"
)

var (
	ErrNotInBackup     = errors.New("media file is not in the backup")
	ErrAlreadyExported = errors.New("media file was already exported")
)

// maxNameBytes keeps exported names under the 255-byte file name limit of macOS,
// leaving room for the "-N" suffix added on collisions.
const maxNameBytes = 240

// MediaSet holds the backup paths of all WhatsApp media files.
type MediaSet map[string]bool

// LoadMediaSet lists every WhatsApp media file in the backup with a single manifest query.
func LoadMediaSet(b *backup.Backup) (MediaSet, error) {
	return b.Paths(whatsapp.Domain, whatsapp.ManifestPath(""))
}

// Has reports whether the media of m is in the backup and can be exported.
func (s MediaSet) Has(m whatsapp.Message) bool {
	return s[whatsapp.ManifestPath(m.MediaPath)]
}

// Media writes the media of m into dir and returns the path of the new file.
// The file's modification time is set to the message date. When a file with the same name and size
// already exists, it returns that path and ErrAlreadyExported.
func Media(b *backup.Backup, m whatsapp.Message, dir string) (string, error) {
	src := whatsapp.ManifestPath(m.MediaPath)
	f, err := b.FindFile(whatsapp.Domain, src)
	if errors.Is(err, backup.ErrNotFound) {
		slog.Warn("media not in backup", "message", m.ID, "path", src)
		return "", ErrNotInBackup
	}
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", applog.Fail("create export dir", err, "dir", dir)
	}
	dest, done, err := target(dir, FileName(m), f.Size)
	if err != nil {
		return "", applog.Fail("choose export file name", err, "message", m.ID, "name", FileName(m))
	}
	if done {
		slog.Info("media already exported", "message", m.ID, "path", src, "size", f.Size, "dest", dest)
		return dest, ErrAlreadyExported
	}
	slog.Info("exporting media", "message", m.ID, "type", m.Type, "path", src, "fileID", f.ID, "size", f.Size, "dest", dest)
	if err := b.ExtractFile(f, dest); err != nil {
		return "", err
	}
	if err := os.Chtimes(dest, m.Date, m.Date); err != nil {
		return dest, applog.Fail("set file time", err, "path", dest)
	}
	slog.Info("exported media", "message", m.ID, "path", src, "fileID", f.ID, "size", f.Size, "dest", dest)
	return dest, nil
}

// FileName builds "<date>_<time>_<sender>_<original name>" for an exported media file.
func FileName(m whatsapp.Message) string {
	name := path.Base(m.MediaPath)
	if m.Type == whatsapp.Document && filepath.Ext(m.MediaTitle) != "" {
		name = m.MediaTitle
	}
	name = Sanitize(fmt.Sprintf("%s_%s_%s", m.Date.Local().Format("2006-01-02_15-04-05"), m.Sender, name))
	return truncateName(name, maxNameBytes)
}

// truncateName shortens name to at most max bytes, keeping its extension and whole UTF-8 characters.
func truncateName(name string, max int) string {
	if len(name) <= max {
		return name
	}
	ext := filepath.Ext(name)
	if len(ext) > 10 {
		ext = "" // a dot inside a long title, not a real extension
	}
	base := strings.TrimSuffix(name, ext)
	cut := max - len(ext)
	for cut > 0 && !utf8.RuneStart(base[cut]) {
		cut--
	}
	return base[:cut] + ext
}

// Sanitize makes s safe as a file name on macOS and Windows. It replaces characters that are not
// allowed, drops trailing dots and spaces, which Windows silently removes, and prefixes names
// that Windows reserves for devices.
func Sanitize(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || strings.ContainsRune(`/\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, s)
	s = strings.TrimRight(s, ". ")
	if isReservedName(s) {
		s = "_" + s
	}
	return s
}

// isReservedName reports whether Windows reserves name for a device, with or without an extension.
func isReservedName(name string) bool {
	base, _, _ := strings.Cut(strings.ToUpper(name), ".")
	switch base {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	return len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) &&
		base[3] >= '1' && base[3] <= '9'
}

// target returns the path to export a file of size bytes to. done reports that a file with the
// same name and size is already there; a different size gets a new name from uniquePath.
func target(dir, name string, size int64) (path string, done bool, err error) {
	path = filepath.Join(dir, name)
	if fi, err := os.Stat(path); err == nil && fi.Mode().IsRegular() && fi.Size() == size {
		return path, true, nil
	}
	path, err = uniquePath(path)
	return path, false, err
}

// uniquePath appends -1, -2, ... before the extension until p does not exist.
// Errors other than "does not exist", such as a name that is too long, are returned.
func uniquePath(p string) (string, error) {
	ext := filepath.Ext(p)
	base := strings.TrimSuffix(p, ext)
	candidate := p
	for i := 1; ; i++ {
		_, err := os.Lstat(candidate)
		if os.IsNotExist(err) {
			return candidate, nil
		}
		if err != nil {
			return "", err
		}
		candidate = fmt.Sprintf("%s-%d%s", base, i, ext)
	}
}
