package applog

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenCreatesDirectoryAndAppends(t *testing.T) {
	defer slog.SetDefault(slog.Default())
	path := filepath.Join(t.TempDir(), "nested", "app.log")
	for _, msg := range []string{"first run", "second run"} {
		f, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		slog.Debug(msg, "key", "value")
		f.Close()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"level=DEBUG", `msg="first run"`, `msg="second run"`, "key=value"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("log does not contain %q:\n%s", want, data)
		}
	}
}

func TestDefaultPathPerOS(t *testing.T) {
	tests := []struct {
		goos, home, localAppData, want string
	}{
		{"darwin", "/Users/jens", "", filepath.Join("/Users/jens", "Library", "Logs", "iphone-backup", "iphone-backup.log")},
		{"windows", `C:\Users\jens`, `C:\Users\jens\AppData\Local`, filepath.Join(`C:\Users\jens\AppData\Local`, "iphone-backup", "iphone-backup.log")},
		{"windows", `C:\Users\jens`, "", filepath.Join(`C:\Users\jens`, "AppData", "Local", "iphone-backup", "iphone-backup.log")},
	}
	for _, tt := range tests {
		if got := defaultPath(tt.goos, tt.home, tt.localAppData); got != tt.want {
			t.Errorf("defaultPath(%s, %q) = %q, want %q", tt.goos, tt.localAppData, got, tt.want)
		}
	}
}
