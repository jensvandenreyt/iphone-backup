package tui

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jensvandenreyt/iphone-backup/internal/export"
	"github.com/jensvandenreyt/iphone-backup/internal/whatsapp"
)

// logSize is the number of recent file accesses shown in the export log window.
const logSize = 10

type exportStepMsg struct {
	dest string
	err  error
}

type exportItem struct {
	m      whatsapp.Message
	folder string // subfolder of the target folder, empty when exporting a single chat
}

// exporter asks for a target directory and exports media one file at a time, showing progress.
type exporter struct {
	s        *session
	from     string // what is exported, shown in the title: a chat name or "3 chats"
	back     screen
	items    []exportItem
	input    textinput.Model
	progress progress.Model
	dir      string
	running  bool
	done     int
	exported int
	skipped  int
	missing  int
	failed   []string
	log      []string
}

func newExporter(s *session, from, dir string, items []exportItem, back screen) (screen, tea.Cmd) {
	e := &exporter{s: s, from: from, items: items, back: back, input: textinput.New(), progress: progress.New(progress.WithDefaultGradient())}
	e.input.Prompt = "Export to: "
	e.input.SetValue(dir)
	return e, e.input.Focus()
}

func (e *exporter) finished() bool { return e.done == len(e.items) }

func (e *exporter) step() tea.Cmd {
	e.logStart()
	b, m, dir := e.s.backup, e.items[e.done].m, e.destDir(e.done)
	return func() tea.Msg {
		dest, err := export.Media(b, m, dir)
		return exportStepMsg{dest, err}
	}
}

// logStart adds a pending log line for the file about to be read.
func (e *exporter) logStart() {
	e.log = append(e.log, "reading  "+e.source())
	if len(e.log) > logSize {
		e.log = e.log[len(e.log)-logSize:]
	}
}

// source is the backup path of the file currently being exported.
func (e *exporter) source() string {
	return whatsapp.ManifestPath(e.items[e.done].m.MediaPath)
}

// destDir is the folder that item i is exported to.
func (e *exporter) destDir(i int) string {
	return filepath.Join(e.dir, e.items[i].folder)
}

func (e *exporter) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case exportStepMsg:
		e.record(msg)
		if e.finished() {
			slog.Info("export finished", "from", e.from, "dir", e.dir,
				"exported", e.exported, "skipped", e.skipped, "missing", e.missing, "failed", len(e.failed))
			return e, nil
		}
		return e, e.step()
	case tea.KeyMsg:
		return e.handleKey(msg)
	}
	var cmd tea.Cmd
	e.input, cmd = e.input.Update(msg)
	return e, cmd
}

// record counts the outcome of a step and replaces its pending log line.
func (e *exporter) record(msg exportStepMsg) {
	var line string
	switch err := msg.err; {
	case errors.Is(err, export.ErrNotInBackup):
		e.missing++
		line = dimStyle.Render("missing  " + e.source())
	case errors.Is(err, export.ErrAlreadyExported):
		e.skipped++
		line = dimStyle.Render("skipped  " + e.source() + " (already exported as " + filepath.Base(msg.dest) + ")")
	case err != nil:
		e.failed = append(e.failed, err.Error())
		line = errorStyle.Render("failed   " + e.source() + ": " + err.Error())
	default:
		e.exported++
		line = "ok       " + e.source() + " -> " + filepath.Base(msg.dest)
	}
	e.log[len(e.log)-1] = line
	e.done++
}

func (e *exporter) handleKey(msg tea.KeyMsg) (screen, tea.Cmd) {
	k := msg.String()
	switch {
	case e.running && !e.finished():
		return e, nil
	case e.running && (k == "esc" || k == "enter"), !e.running && k == "esc":
		return e.back, nil
	case e.running:
		return e, nil
	case k == "enter":
		dir, err := filepath.Abs(expandHome(strings.TrimSpace(e.input.Value())))
		if err != nil {
			slog.Error("resolve export dir failed", "input", e.input.Value(), "err", err)
			return e, nil
		}
		slog.Info("export started", "from", e.from, "dir", dir, "files", len(e.items))
		e.dir, e.running = dir, true
		e.input.Blur()
		return e, e.step()
	}
	var cmd tea.Cmd
	e.input, cmd = e.input.Update(msg)
	return e, cmd
}

func (e *exporter) View() string {
	lines := []string{titleStyle.Render(fmt.Sprintf("Export %d files from %s", len(e.items), e.from)), ""}
	switch {
	case !e.running:
		lines = append(lines, e.input.View(), "", dimStyle.Render("enter: start · esc: back"))
	case !e.finished():
		lines = append(lines, e.progress.ViewAs(float64(e.done)/float64(len(e.items))),
			fmt.Sprintf("%d / %d", e.done, len(e.items)), "", e.logView())
	default:
		lines = append(lines, e.summary()...)
		lines = append(lines, "", e.logView(), "", dimStyle.Render("enter/esc: back"))
	}
	return pageStyle.Render(strings.Join(lines, "\n"))
}

// logView renders the last logSize file accesses in a fixed-height box.
func (e *exporter) logView() string {
	width := max(e.s.width-8, 40)
	line := lipgloss.NewStyle().MaxWidth(width - logStyle.GetHorizontalPadding())
	lines := make([]string, logSize)
	for i, l := range e.log {
		lines[i] = line.Render(l)
	}
	return logStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func (e *exporter) summary() []string {
	lines := []string{fmt.Sprintf("Exported %d files to %s", e.exported, e.dir)}
	if e.skipped > 0 {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("%d files were already exported", e.skipped)))
	}
	if e.missing > 0 {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("%d files were not in the backup", e.missing)))
	}
	if len(e.failed) > 0 {
		lines = append(lines, errorStyle.Render(fmt.Sprintf("%d files failed:", len(e.failed))))
		for _, f := range e.failed[:min(len(e.failed), 5)] {
			lines = append(lines, errorStyle.Render("  "+f))
		}
	}
	return lines
}
