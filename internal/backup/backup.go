// Package backup reads iOS backups made by Finder, iTunes or Apple Configurator, encrypted or not.
package backup

import (
	"bytes"
	"crypto/aes"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"howett.net/plist"
	_ "modernc.org/sqlite"

	"github.com/jensvandenreyt/iphone-backup/internal/applog"
)

var (
	ErrWrongPassword = errors.New("wrong password")
	ErrNotFound      = errors.New("file not found in backup")
)

// DefaultRoot returns the folder where backups are stored on this computer: by Finder and Apple
// Configurator on macOS, by iTunes or the Apple Devices app on Windows.
func DefaultRoot() string {
	home, _ := os.UserHomeDir()
	roots := defaultRoots(runtime.GOOS, home, os.Getenv("APPDATA"))
	for _, r := range roots {
		if _, err := os.Stat(r); err == nil {
			return r
		}
	}
	return roots[0]
}

// defaultRoots lists the possible backup folders for an operating system, most likely first.
func defaultRoots(goos, home, appData string) []string {
	if goos == "windows" {
		return []string{
			// The Apple Devices app and iTunes from the Microsoft Store.
			filepath.Join(home, "Apple", "MobileSync", "Backup"),
			// iTunes downloaded from apple.com.
			filepath.Join(appData, "Apple Computer", "MobileSync", "Backup"),
		}
	}
	return []string{filepath.Join(home, "Library", "Application Support", "MobileSync", "Backup")}
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
		return nil, applog.Fail("list backups", err, "root", root)
	}
	var out []*Backup
	for _, e := range entries {
		dir := filepath.Join(root, e.Name())
		if !e.IsDir() || !IsBackup(dir) {
			continue
		}
		b, err := Open(dir)
		if err != nil {
			slog.Warn("skipping unreadable backup", "dir", dir, "err", err)
			continue
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Info.LastBackup.After(out[j].Info.LastBackup) })
	slog.Info("listed backups", "root", root, "count", len(out))
	return out, nil
}

// Open reads the backup metadata. Call Unlock before accessing files.
func Open(dir string) (*Backup, error) {
	var m manifestPlist
	if err := readPlist(filepath.Join(dir, "Manifest.plist"), &m); err != nil {
		return nil, applog.Fail("read Manifest.plist", err, "dir", dir)
	}
	var info infoPlist
	if err := readPlist(filepath.Join(dir, "Info.plist"), &info); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, applog.Fail("read Info.plist", err, "dir", dir)
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
			return nil, applog.Fail("parse keybag", err, "dir", dir)
		}
		b.keybag = kb
		slog.Debug("parsed keybag", "dir", dir, "classes", len(kb.classes), "iter", kb.iter, "dpic", kb.dpic)
	}
	slog.Info("opened backup", "dir", dir, "device", b.Info.DeviceName, "product", b.Info.ProductType,
		"ios", b.Info.ProductVersion, "lastBackup", b.Info.LastBackup, "encrypted", b.Encrypted)
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
	slog.Info("unlocking backup", "dir", b.Dir, "encrypted", b.Encrypted)
	if b.Encrypted {
		if err := b.unlockKeybag(password); err != nil {
			return err
		}
	}
	if b.tmpDir == "" {
		tmp, err := os.MkdirTemp("", "iphone-backup-")
		if err != nil {
			return applog.Fail("create temp dir", err)
		}
		b.tmpDir = tmp
		slog.Debug("created temp dir", "path", tmp)
	}
	path := b.TempPath("Manifest.db")
	if err := b.writeManifestDB(path); err != nil {
		return applog.Fail("prepare Manifest.db", err, "dir", b.Dir)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return applog.Fail("open Manifest.db", err, "path", path)
	}
	b.db = db
	slog.Info("unlocked backup", "dir", b.Dir)
	return nil
}

func (b *Backup) unlockKeybag(password string) error {
	start := time.Now()
	err := b.keybag.unlock(password)
	took := time.Since(start).Round(time.Millisecond)
	if errors.Is(err, ErrWrongPassword) {
		slog.Warn("wrong password", "dir", b.Dir, "took", took)
		return err
	}
	if err != nil {
		return applog.Fail("derive backup key", err, "dir", b.Dir)
	}
	slog.Info("derived backup key", "took", took)
	return nil
}

func (b *Backup) writeManifestDB(path string) error {
	data, err := os.ReadFile(filepath.Join(b.Dir, "Manifest.db"))
	if err != nil {
		return err
	}
	if b.Encrypted {
		encrypted := len(data)
		if data, err = b.decryptManifestDB(data); err != nil {
			return err
		}
		slog.Debug("decrypted Manifest.db", "encryptedBytes", encrypted, "bytes", len(data))
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
		return nil, fmt.Errorf("unwrap manifest key (class %d): %w", class, err)
	}
	data, err = decryptCBC(data, key)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
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
		return nil, applog.Fail("look up file", err, "domain", domain, "path", relativePath)
	}
	f := &File{ID: id, Domain: domain, RelativePath: relativePath}
	if err := decodeMBFile(blob, f); err != nil {
		return nil, applog.Fail("decode file metadata", err, "domain", domain, "path", relativePath, "fileID", id)
	}
	return f, nil
}

// Paths returns the relative paths of all regular files in domain that start with prefix.
// It reads the manifest once, which is much faster than calling FindFile per file.
func (b *Backup) Paths(domain, prefix string) (map[string]bool, error) {
	start := time.Now()
	slog.Debug("listing files", "domain", domain, "prefix", prefix)
	rows, err := b.db.Query(
		`SELECT relativePath FROM Files WHERE domain = ? AND flags = 1 AND substr(relativePath, 1, ?) = ?`,
		domain, len(prefix), prefix,
	)
	if err != nil {
		return nil, applog.Fail("list files", err, "domain", domain, "prefix", prefix)
	}
	defer rows.Close()
	paths := map[string]bool{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, applog.Fail("list files", err, "domain", domain, "prefix", prefix)
		}
		paths[p] = true
	}
	if err := rows.Err(); err != nil {
		return nil, applog.Fail("list files", err, "domain", domain, "prefix", prefix)
	}
	slog.Debug("listed files", "domain", domain, "prefix", prefix, "count", len(paths),
		"took", time.Since(start).Round(time.Millisecond))
	return paths, nil
}

// Extract writes the plaintext content of f to dst.
func (b *Backup) Extract(f *File, dst io.Writer) error {
	if err := b.extract(f, dst); err != nil {
		return applog.Fail("extract "+f.RelativePath, err, "domain", f.Domain, "path", f.RelativePath,
			"fileID", f.ID, "size", f.Size, "class", f.protectionClass)
	}
	slog.Debug("extracted file", "domain", f.Domain, "path", f.RelativePath, "fileID", f.ID, "size", f.Size)
	return nil
}

func (b *Backup) extract(f *File, dst io.Writer) error {
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
		return fmt.Errorf("unwrap file key: %w", err)
	}
	n, err := decryptStream(dst, src, key, f.Size)
	if fi, serr := src.Stat(); errors.Is(err, errTruncated) && serr == nil {
		minSize := (f.Size + aes.BlockSize - 1) / aes.BlockSize * aes.BlockSize
		err = fmt.Errorf("%w; encrypted file on disk is %d bytes, expected at least %d", err, fi.Size(), minSize)
	}
	if err == nil && n != f.Size {
		slog.Warn("file is smaller than the manifest says, used its padding to find the end",
			"domain", f.Domain, "path", f.RelativePath, "fileID", f.ID, "manifestSize", f.Size, "size", n)
	}
	return err
}

// ExtractFile writes the plaintext content of f to a new file at path.
func (b *Backup) ExtractFile(f *File, path string) error {
	out, err := os.Create(path)
	if err != nil {
		return applog.Fail("create file", err, "path", path)
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
	if err := os.RemoveAll(b.tmpDir); err != nil {
		return applog.Fail("remove temp dir", err, "path", b.tmpDir)
	}
	slog.Debug("removed temp dir", "path", b.tmpDir)
	return nil
}
