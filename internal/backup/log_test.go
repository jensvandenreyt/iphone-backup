package backup

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnlockLogsWithoutPassword(t *testing.T) {
	logs := captureLogs(t)
	b, err := Open(buildBackup(t, true))
	if err != nil {
		t.Fatal(err)
	}
	b.Unlock("hunter2")
	if err := b.Unlock(testPassword); err != nil {
		t.Fatal(err)
	}
	b.Close()
	out := logs.String()
	for _, want := range []string{"opened backup", "wrong password", "derived backup key", "decrypted Manifest.db", "removed temp dir"} {
		if !strings.Contains(out, want) {
			t.Errorf("log does not contain %q:\n%s", want, out)
		}
	}
	for _, secret := range []string{"hunter2", testPassword} {
		if strings.Contains(out, secret) {
			t.Errorf("log contains the password %q", secret)
		}
	}
}

func TestExtractFailureLogsFileDetails(t *testing.T) {
	dir := buildBackup(t, true)
	sum := sha1.Sum([]byte(testDomain + "-" + testPath))
	fileID := hex.EncodeToString(sum[:])
	if err := os.Truncate(filepath.Join(dir, fileID[:2], fileID), 16); err != nil {
		t.Fatal(err)
	}
	b, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Unlock(testPassword); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	f, err := b.FindFile(testDomain, testPath)
	if err != nil {
		t.Fatal(err)
	}

	logs := captureLogs(t)
	err = b.Extract(f, io.Discard)
	if err == nil || !strings.Contains(err.Error(), testPath) || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("err = %v, want truncated error naming %s", err, testPath)
	}
	for _, want := range []string{"level=ERROR", fileID, testDomain, testPath, "truncated"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("log does not contain %q:\n%s", want, logs)
		}
	}
}

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func TestTruncatedErrorReportsSizes(t *testing.T) {
	dir := buildBackup(t, true)
	sum := sha1.Sum([]byte(testDomain + "-" + testPath))
	fileID := hex.EncodeToString(sum[:])
	if err := os.Truncate(filepath.Join(dir, fileID[:2], fileID), 16); err != nil {
		t.Fatal(err)
	}
	b, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Unlock(testPassword); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	f, err := b.FindFile(testDomain, testPath)
	if err != nil {
		t.Fatal(err)
	}

	err = b.Extract(f, io.Discard)
	// testContent is 52 bytes, so at least 64 encrypted bytes are needed; only 16 remain on disk.
	for _, want := range []string{"decrypted 16 of 52 bytes", "encrypted file on disk is 16 bytes", "expected at least 64"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want it to contain %q", err, want)
		}
	}
}

// A file that changed while the backup ran can be shorter than the manifest says,
// yet complete: its last block is still valid padding.
func TestExtractUsesPaddingWhenManifestSizeIsStale(t *testing.T) {
	manifestSize := int64(len(testContent) + 100)
	b, err := Open(buildBackupWithSize(t, true, manifestSize))
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Unlock(testPassword); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	f, err := b.FindFile(testDomain, testPath)
	if err != nil {
		t.Fatal(err)
	}

	logs := captureLogs(t)
	var out bytes.Buffer
	if err := b.Extract(f, &out); err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !bytes.Equal(out.Bytes(), testContent) {
		t.Errorf("content = %q, want %q", out.Bytes(), testContent)
	}
	for _, want := range []string{"level=WARN", testPath, fmt.Sprintf("manifestSize=%d", manifestSize), fmt.Sprintf("size=%d", len(testContent))} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("log does not contain %q:\n%s", want, logs)
		}
	}
}

func TestPaths(t *testing.T) {
	b, err := Open(buildBackup(t, true))
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Unlock(testPassword); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })

	logs := captureLogs(t)
	tests := []struct {
		domain, prefix string
		want           int
	}{
		{testDomain, "Documents/", 1},
		{testDomain, "Other/", 0},
		{"OtherDomain", "Documents/", 0},
	}
	for _, tt := range tests {
		paths, err := b.Paths(tt.domain, tt.prefix)
		if err != nil {
			t.Fatal(err)
		}
		if len(paths) != tt.want {
			t.Errorf("Paths(%q, %q) = %v, want %d paths", tt.domain, tt.prefix, paths, tt.want)
		}
	}
	paths, _ := b.Paths(testDomain, "Documents/")
	if !paths[testPath] {
		t.Errorf("paths = %v, want %s", paths, testPath)
	}
	for _, want := range []string{`msg="listing files"`, `msg="listed files"`, "count=1", "took="} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("log does not contain %q:\n%s", want, logs)
		}
	}
}

func TestDefaultRoots(t *testing.T) {
	mac := defaultRoots("darwin", "/Users/jens", "")
	if want := filepath.Join("/Users/jens", "Library", "Application Support", "MobileSync", "Backup"); len(mac) != 1 || mac[0] != want {
		t.Errorf("darwin = %v, want [%s]", mac, want)
	}
	win := defaultRoots("windows", `C:\Users\jens`, `C:\Users\jens\AppData\Roaming`)
	want := []string{
		filepath.Join(`C:\Users\jens`, "Apple", "MobileSync", "Backup"),
		filepath.Join(`C:\Users\jens\AppData\Roaming`, "Apple Computer", "MobileSync", "Backup"),
	}
	if fmt.Sprint(win) != fmt.Sprint(want) {
		t.Errorf("windows = %v, want %v", win, want)
	}
}
