package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/R037/ayca/internal/discovery"
	"github.com/R037/ayca/internal/preview"
)

func setup(t *testing.T) Model {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	proj := filepath.Join(root, "proj")
	for p, s := range map[string]string{
		filepath.Join(proj, ".git", "HEAD"):                         "ref",
		filepath.Join(proj, "CLAUDE.md"):                            "# Project Instructions\n\nUse **Go**.",
		filepath.Join(home, ".claude", "skills", "pdf", "SKILL.md"): "---\nname: pdf\n---\n# PDF",
	} {
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(s), 0o644)
	}
	env := discovery.NewEnvWith(proj, home, func(string) string { return "" })
	m := New(discovery.Discover(env, nil), nil, nil, preview.NewRendererStyle("dark"))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return next.(Model)
}

func press(m Model, keys ...string) Model {
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "down", "up", "left", "right", "enter", "tab", "esc":
			msg = tea.KeyMsg{Type: map[string]tea.KeyType{"down": tea.KeyDown, "up": tea.KeyUp, "left": tea.KeyLeft,
				"right": tea.KeyRight, "enter": tea.KeyEnter, "tab": tea.KeyTab, "esc": tea.KeyEsc}[k]}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

func TestViewFitsTerminal(t *testing.T) {
	m := setup(t)
	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) != 30 {
		t.Errorf("view has %d lines, want 30", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w > 100 {
			t.Errorf("line %d is %d wide", i, w)
		}
	}
	if !strings.Contains(view, "AYCA — All Your Context Are Belong To Us") {
		t.Error("missing title")
	}
}

func TestNavigateToFileShowsPreview(t *testing.T) {
	m := setup(t)
	// Rows: Instructions, Project, CLAUDE.md, ...
	m = press(m, "j", "j")
	if n := m.selected(); n == nil || n.Item == nil || n.Item.Name != "CLAUDE.md" {
		t.Fatalf("selected %+v", m.selected())
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{"Project Instructions", "Use Go.", "instruction", "project", "claude"} {
		if !strings.Contains(strings.ToLower(view), strings.ToLower(want)) {
			t.Errorf("view missing %q", want)
		}
	}
	m = press(m, "enter")
	if m.focus != focusPreview {
		t.Error("enter should focus preview")
	}
	m = press(m, "v")
	if !strings.Contains(ansi.Strip(m.View()), "Use **Go**.") {
		t.Error("raw view should show markdown source")
	}
	m = press(m, "esc")
	if m.focus != focusTree {
		t.Error("esc should return to tree")
	}
}

func TestCollapseExpandAndFilter(t *testing.T) {
	m := setup(t)
	n := len(m.rows)
	m = press(m, "h") // collapse Instructions
	if len(m.rows) >= n {
		t.Fatal("collapse did not hide rows")
	}
	m = press(m, "l")
	if len(m.rows) != n {
		t.Fatal("expand did not restore rows")
	}
	m = press(m, "/", "p", "d", "f", "enter")
	if s := m.selected(); s == nil || s.Item == nil || s.Item.Name != "pdf" {
		t.Fatalf("filter should select pdf, got %+v", s)
	}
	m = press(m, "l") // expand the skill
	if !strings.Contains(m.View(), "SKILL.md") {
		t.Error("expanded skill should list SKILL.md")
	}
	m = press(m, "esc")
	if m.filter.Value() != "" || len(m.rows) <= 4 {
		t.Error("esc should clear the filter")
	}
}

func TestQuit(t *testing.T) {
	m := setup(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("q should return a quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q should quit")
	}
}

func TestBatchedRunesStartSearch(t *testing.T) {
	m := setup(t)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/pdf")})
	m = next.(Model)
	if !m.filtering || m.filter.Value() != "pdf" {
		t.Fatalf("filtering=%v value=%q", m.filtering, m.filter.Value())
	}
}

func TestBudgetPanel(t *testing.T) {
	m := setup(t)
	if !strings.Contains(ansi.Strip(m.View()), "startup context on disk: claude") {
		t.Error("header should show the startup context percentage")
	}
	m = press(m, "c")
	view := ansi.Strip(m.View())
	for _, want := range []string{"Startup context", "of 200k", "Not counted"} {
		if !strings.Contains(view, want) {
			t.Errorf("budget panel missing %q", want)
		}
	}
	m = press(m, "c")
	if strings.Contains(ansi.Strip(m.View()), "Not counted") {
		t.Error("c should toggle the panel off")
	}
}
