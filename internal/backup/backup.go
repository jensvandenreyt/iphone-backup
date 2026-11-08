// Package backup reads iOS backups made by Finder, iTunes or Apple Configurator, encrypted or not.
package backup

import (
	"bytes"
	"database/sql"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"howett.net/plist"
	_ "modernc.org/sqlite"
)

var (
	ErrWrongPassword = errors.New("wrong password")
	ErrNotFound      = errors.New("file not found in backup")
)

// DefaultRoot returns the folder where Finder and Apple Configurator store backups.
func DefaultRoot() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Application Support", "MobileSync", "Backup")
}

type Info struct {
	DeviceName     string
	ProductType    string
	ProductVersion string
	LastBackup     time.Time
}

type Backup struct {
	Dir       string
	Info      Info
	Encrypted bool

	keybag      *keybag
	manifestKey []byte
	tmpDir      string
	db          *sql.DB
}

// File is an entry from the Files table of Manifest.db.
type File struct {
	ID           string
	Domain       string
	RelativePath string
	Size         int64

	protectionClass uint32
	encryptionKey   []byte
}

type manifestPlist struct {
	IsEncrypted  bool   `plist:"IsEncrypted"`
	BackupKeyBag []byte `plist:"BackupKeyBag"`
	ManifestKey  []byte `plist:"ManifestKey"`
}

type infoPlist struct {
	DeviceName     string    `plist:"Device Name"`
	ProductType    string    `plist:"Product Type"`
	ProductVersion string    `plist:"Product Version"`
	LastBackupDate time.Time `plist:"Last Backup Date"`
}

// IsBackup reports whether dir looks like a single backup rather than a folder of backups.
func IsBackup(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "Manifest.plist"))
	return err == nil
}

// List opens every backup directly inside root, newest first.
func List(root string) ([]*Backup, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var out []*Backup
	for _, e := range entries {
		dir := filepath.Join(root, e.Name())
		if !e.IsDir() || !IsBackup(dir) {
			continue
		}
		if b, err := Open(dir); err == nil {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Info.LastBackup.After(out[j].Info.LastBackup) })
	return out, nil
}

// Open reads the backup metadata. Call Unlock before accessing files.
func Open(dir string) (*Backup, error) {
	var m manifestPlist
	if err := readPlist(filepath.Join(dir, "Manifest.plist"), &m); err != nil {
		return nil, err
	}
	var info infoPlist
	if err := readPlist(filepath.Join(dir, "Info.plist"), &info); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if info.DeviceName == "" {
		info.DeviceName = filepath.Base(dir)
	}
	b := &Backup{
		Dir:         dir,
		Encrypted:   m.IsEncrypted,
		manifestKey: m.ManifestKey,
		Info: Info{
			DeviceName:     info.DeviceName,
			ProductType:    info.ProductType,
			ProductVersion: info.ProductVersion,
			LastBackup:     info.LastBackupDate,
		},
	}
	if m.IsEncrypted {
		kb, err := parseKeybag(m.BackupKeyBag)
		if err != nil {
			return nil, err
		}
		b.keybag = kb
	}
	return b, nil
}

func readPlist(path string, v any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return plist.NewDecoder(f).Decode(v)
}

// Unlock decrypts the manifest database into a private temp directory.
// The password is ignored for unencrypted backups.
func (b *Backup) Unlock(password string) error {
	if b.db != nil {
		return nil
	}
	if b.Encrypted {
		if err := b.keybag.unlock(password); err != nil {
			return err
		}
	}
	if b.tmpDir == "" {
		tmp, err := os.MkdirTemp("", "iphone-backup-")
		if err != nil {
			return err
		}
		b.tmpDir = tmp
	}
	path := b.TempPath("Manifest.db")
	if err := b.writeManifestDB(path); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	b.db = db
	return nil
}

func (b *Backup) writeManifestDB(path string) error {
	data, err := os.ReadFile(filepath.Join(b.Dir, "Manifest.db"))
	if err != nil {
		return err
	}
	if b.Encrypted {
		if data, err = b.decryptManifestDB(data); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o600)
}

func (b *Backup) decryptManifestDB(data []byte) ([]byte, error) {
	if len(b.manifestKey) < 4 {
		return nil, errors.New("Manifest.plist has no valid ManifestKey")
	}
	class := binary.LittleEndian.Uint32(b.manifestKey[:4])
	key, err := b.keybag.unwrapKey(class, b.manifestKey[4:])
	if err != nil {
		return nil, err
	}
	data, err = decryptCBC(data, key)
	if err != nil {
		return nil, err
	}
	return trimToPages(data)
}

// trimToPages drops trailing cipher padding by cutting the database to a whole number of pages.
func trimToPages(db []byte) ([]byte, error) {
	if len(db) < 100 || !bytes.HasPrefix(db, []byte("SQLite format 3\x00")) {
		return nil, errors.New("Manifest.db did not decrypt to a SQLite database")
	}
	pageSize := int(binary.BigEndian.Uint16(db[16:18]))
	if pageSize == 1 {
		pageSize = 65536
	}
	return db[:len(db)-len(db)%pageSize], nil
}

// TempPath returns a path inside the private temp directory created by Unlock.
func (b *Backup) TempPath(name string) string {
	return filepath.Join(b.tmpDir, name)
}

// FindFile looks up a regular file by domain and relative path.
func (b *Backup) FindFile(domain, relativePath string) (*File, error) {
	var id string
	var blob []byte
	err := b.db.QueryRow(
		`SELECT fileID, file FROM Files WHERE domain = ? AND relativePath = ? AND flags = 1`,
		domain, relativePath,
	).Scan(&id, &blob)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	f := &File{ID: id, Domain: domain, RelativePath: relativePath}
	if err := decodeMBFile(blob, f); err != nil {
		return nil, err
	}
	return f, nil
}

// Extract writes the plaintext content of f to dst.
func (b *Backup) Extract(f *File, dst io.Writer) error {
	src, err := os.Open(filepath.Join(b.Dir, f.ID[:2], f.ID))
	if err != nil {
		return err
	}
	defer src.Close()
	if !b.Encrypted {
		_, err := io.Copy(dst, src)
		return err
	}
	if f.encryptionKey == nil {
		if f.Size == 0 {
			return nil
		}
		return errors.New("file has no encryption key")
	}
	key, err := b.keybag.unwrapKey(f.protectionClass, f.encryptionKey)
	if err != nil {
		return err
	}
	return decryptStream(dst, src, key, f.Size)
}

// ExtractFile writes the plaintext content of f to a new file at path.
func (b *Backup) ExtractFile(f *File, path string) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	err = b.Extract(f, out)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path)
	}
	return err
}

// Close removes all decrypted temporary files.
func (b *Backup) Close() error {
	if b.db != nil {
		b.db.Close()
		b.db = nil
	}
	if b.tmpDir == "" {
		return nil
	}
	return os.RemoveAll(b.tmpDir)
}
