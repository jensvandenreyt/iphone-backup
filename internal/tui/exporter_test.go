package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/vandenreyt/iphone-backup/internal/export"
	"github.com/vandenreyt/iphone-backup/internal/whatsapp"
)

func TestExporterLogKeepsLastTen(t *testing.T) {
	items := make([]whatsapp.Message, 12)
	for i := range items {
		items[i].MediaPath = fmt.Sprintf("Media/x/%d.jpg", i)
	}
	e := &exporter{items: items}
	for range items {
		e.logStart()
		e.record(exportStepMsg{dest: "/out/file.jpg"})
	}
	if len(e.log) != logSize {
		t.Fatalf("log has %d lines, want %d", len(e.log), logSize)
	}
	if !strings.Contains(e.log[0], "Message/Media/x/2.jpg") || !strings.Contains(e.log[9], "Message/Media/x/11.jpg") {
		t.Errorf("log = %q, want files 2 to 11", e.log)
	}
}

func TestExporterLogShowsOutcome(t *testing.T) {
	items := []whatsapp.Message{{MediaPath: "Media/x/a.jpg"}, {MediaPath: "Media/x/b.jpg"}, {MediaPath: "Media/x/c.jpg"}}
	e := &exporter{items: items}

	e.logStart()
	if !strings.Contains(e.log[0], "reading") {
		t.Errorf("pending line = %q, want reading", e.log[0])
	}
	e.record(exportStepMsg{dest: "/out/2024_a.jpg"})
	e.logStart()
	e.record(exportStepMsg{err: export.ErrNotInBackup})
	e.logStart()
	e.record(exportStepMsg{err: errors.New("encrypted file is truncated")})

	want := []string{"ok", "missing", "failed"}
	for i, w := range want {
		if !strings.Contains(e.log[i], w) {
			t.Errorf("line %d = %q, want %q", i, e.log[i], w)
		}
	}
	if !strings.Contains(e.log[0], "2024_a.jpg") || !strings.Contains(e.log[2], "truncated") {
		t.Errorf("log = %q, missing destination or error", e.log)
	}
}
