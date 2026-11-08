package export

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jensvandenreyt/iphone-backup/internal/whatsapp"
)

func TestFileName(t *testing.T) {
	date := time.Date(2024, 3, 1, 14, 32, 5, 0, time.Local)
	tests := []struct {
		name string
		msg  whatsapp.Message
		want string
	}{
		{
			"image uses original name",
			whatsapp.Message{Date: date, Sender: "Alice", Type: whatsapp.Image, MediaPath: "Media/a@s.whatsapp.net/1/2/abc.jpg"},
			"2024-03-01_14-32-05_Alice_abc.jpg",
		},
		{
			"document uses title",
			whatsapp.Message{Date: date, Sender: "Me", Type: whatsapp.Document, MediaPath: "Media/x/uuid.pdf", MediaTitle: "Invoice 12.pdf"},
			"2024-03-01_14-32-05_Me_Invoice 12.pdf",
		},
		{
			"unsafe characters replaced",
			whatsapp.Message{Date: date, Sender: "A/B: C", Type: whatsapp.Image, MediaPath: "Media/x/y.jpg"},
			"2024-03-01_14-32-05_A_B_ C_y.jpg",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FileName(tt.msg); got != tt.want {
				t.Errorf("FileName = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUniquePath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.jpg")
	if got, err := uniquePath(p); got != p || err != nil {
		t.Fatalf("uniquePath = %q, %v, want %q", got, err, p)
	}
	for _, name := range []string{"a.jpg", "a-1.jpg"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := uniquePath(p); got != filepath.Join(dir, "a-2.jpg") {
		t.Errorf("uniquePath = %q, want a-2.jpg", got)
	}
}

func TestMediaSetHas(t *testing.T) {
	set := MediaSet{"Message/Media/a@s.whatsapp.net/1/2/x.jpg": true}
	if !set.Has(whatsapp.Message{MediaPath: "Media/a@s.whatsapp.net/1/2/x.jpg"}) {
		t.Error("Has = false for a file in the set")
	}
	if set.Has(whatsapp.Message{MediaPath: "Media/a@s.whatsapp.net/1/2/y.jpg"}) {
		t.Error("Has = true for a file not in the set")
	}
}

func TestUniquePathReturnsErrorInsteadOfLooping(t *testing.T) {
	p := filepath.Join(t.TempDir(), strings.Repeat("x", 300)+".pdf")
	done := make(chan error, 1)
	go func() {
		_, err := uniquePath(p)
		done <- err
	}()
	select {
	case err := <-done:
		// Windows may report an over-long name as not existing; creating the file then fails instead.
		if err == nil && runtime.GOOS != "windows" {
			t.Error("err = nil for a name longer than the file system allows")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("uniquePath did not return")
	}
}

func TestFileNameFitsFileSystemLimit(t *testing.T) {
	title := strings.Repeat("Événement très important ", 20) + ".pdf"
	m := whatsapp.Message{Date: time.Now(), Sender: "Alice", Type: whatsapp.Document, MediaPath: "Media/x/uuid.pdf", MediaTitle: title}
	name := FileName(m)
	if len(name) > maxNameBytes || !strings.HasSuffix(name, ".pdf") || !utf8.ValidString(name) {
		t.Errorf("FileName = %q (%d bytes), want at most %d valid UTF-8 bytes ending in .pdf", name, len(name), maxNameBytes)
	}
	if _, err := os.Create(filepath.Join(t.TempDir(), name)); err != nil {
		t.Errorf("cannot create file with the generated name: %v", err)
	}
}

func TestTargetSkipsFilesAlreadyExported(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.jpg"), []byte("12345"), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		size     int64
		wantPath string
		wantDone bool
	}{
		{"b.jpg", 5, "b.jpg", false},   // nothing there yet
		{"a.jpg", 5, "a.jpg", true},    // same name and size: already exported
		{"a.jpg", 9, "a-1.jpg", false}, // same name, other size: a different file
	}
	for _, tt := range tests {
		path, done, err := target(dir, tt.name, tt.size)
		if err != nil {
			t.Fatal(err)
		}
		if path != filepath.Join(dir, tt.wantPath) || done != tt.wantDone {
			t.Errorf("target(%s, %d) = %s, %v, want %s, %v", tt.name, tt.size, filepath.Base(path), done, tt.wantPath, tt.wantDone)
		}
	}
}

func TestSanitizeAvoidsNamesWindowsRejects(t *testing.T) {
	tests := map[string]string{
		"CON":        "_CON",
		"nul":        "_nul",
		"com1.txt":   "_com1.txt",
		"LPT9":       "_LPT9",
		"COM0":       "COM0",
		"Console":    "Console",
		"Family. ":   "Family",
		"a:b|c?":     "a_b_c_",
		"Mom & Dad":  "Mom & Dad",
		"photo.jpg.": "photo.jpg",
	}
	for in, want := range tests {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}
