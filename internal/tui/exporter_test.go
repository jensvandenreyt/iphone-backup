package tui

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jensvandenreyt/iphone-backup/internal/export"
	"github.com/jensvandenreyt/iphone-backup/internal/whatsapp"
)

func TestExporterLogKeepsLastTen(t *testing.T) {
	items := make([]exportItem, 12)
	for i := range items {
		items[i].m.MediaPath = fmt.Sprintf("Media/x/%d.jpg", i)
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
	items := []exportItem{{m: whatsapp.Message{MediaPath: "Media/x/a.jpg"}}, {m: whatsapp.Message{MediaPath: "Media/x/b.jpg"}},
		{m: whatsapp.Message{MediaPath: "Media/x/c.jpg"}}, {m: whatsapp.Message{MediaPath: "Media/x/d.jpg"}}}
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
	e.logStart()
	e.record(exportStepMsg{dest: "/out/d.jpg", err: export.ErrAlreadyExported})

	want := []string{"ok", "missing", "failed", "skipped"}
	for i, w := range want {
		if !strings.Contains(e.log[i], w) {
			t.Errorf("line %d = %q, want %q", i, e.log[i], w)
		}
	}
	if !strings.Contains(e.log[0], "2024_a.jpg") || !strings.Contains(e.log[2], "truncated") {
		t.Errorf("log = %q, missing destination or error", e.log)
	}
}

func TestExporterCountsSkippedFiles(t *testing.T) {
	e := &exporter{items: []exportItem{{m: whatsapp.Message{MediaPath: "Media/x/a.jpg"}}}}
	e.logStart()
	e.record(exportStepMsg{dest: "/out/a.jpg", err: export.ErrAlreadyExported})
	if e.skipped != 1 || e.exported != 0 || len(e.failed) != 0 {
		t.Errorf("skipped=%d exported=%d failed=%d, want 1 skipped", e.skipped, e.exported, len(e.failed))
	}
	if !strings.Contains(strings.Join(e.summary(), "\n"), "1 files were already exported") {
		t.Errorf("summary = %q", e.summary())
	}
}

func TestExporterWritesIntoChatFolders(t *testing.T) {
	e := &exporter{dir: "/out", items: []exportItem{{folder: "Bob (2)"}, {}}}
	if got := e.destDir(0); got != filepath.Join("/out", "Bob (2)") {
		t.Errorf("destDir(0) = %q", got)
	}
	if got := e.destDir(1); got != filepath.Join("/out") {
		t.Errorf("destDir(1) = %q, want the target folder itself", got)
	}
}
