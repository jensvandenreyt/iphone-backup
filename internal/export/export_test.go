package export

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vandenreyt/iphone-backup/internal/whatsapp"
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
	if got := uniquePath(p); got != p {
		t.Fatalf("uniquePath = %q, want %q", got, p)
	}
	for _, name := range []string{"a.jpg", "a-1.jpg"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := uniquePath(p), filepath.Join(dir, "a-2.jpg"); got != want {
		t.Errorf("uniquePath = %q, want %q", got, want)
	}
}
