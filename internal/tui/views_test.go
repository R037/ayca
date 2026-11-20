package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/R037/ayca/internal/discovery"
	"github.com/R037/ayca/internal/preview"
	"github.com/R037/ayca/internal/testfixture"
	"github.com/R037/ayca/internal/tree"
)

// fx opens the realistic fixture in the TUI, started in the given project
// subdirectory ("" for the root).
func fx(t *testing.T, sub string, w, h int, agents ...string) (Model, *testfixture.Fixture) {
	t.Helper()
	f := testfixture.New(t)
	cwd := f.Project
	if sub != "" {
		cwd = f.P(sub)
	}
	env := f.Env(cwd)
	m := New(discovery.Discover(env, agents), agents, nil, preview.NewRendererStyle("dark"))
	// A static cursor has no blink timer, which would stall synchronous draining.
	m.find.Cursor.SetMode(cursor.CursorStatic)
	m.filter.Cursor.SetMode(cursor.CursorStatic)
	next, cmd := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return drain(next.(Model), m.Init(), cmd), f
}

// drain runs commands synchronously and feeds their messages back.
func drain(m Model, cmds ...tea.Cmd) Model {
	for len(cmds) > 0 {
		c := cmds[0]
		cmds = cmds[1:]
		if c == nil {
			continue
		}
		switch msg := runCmd(c).(type) {
		case tea.BatchMsg:
			cmds = append(cmds, msg...)
		case nil:
		default:
			next, cmd := m.Update(msg)
			m = next.(Model)
			cmds = append(cmds, cmd)
		}
	}
	return m
}

// runCmd runs a command but gives up on slow ones (cursor blink timers).
func runCmd(c tea.Cmd) tea.Msg {
	ch := make(chan tea.Msg, 1)
	go func() { ch <- c() }()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(2 * time.Second):
		return nil
	}
}

// key sends keys and runs the commands they return.
func key(m Model, keys ...string) Model {
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "down", "up", "left", "right", "enter", "tab", "esc", "backspace", "pgdown":
			msg = tea.KeyMsg{Type: map[string]tea.KeyType{"down": tea.KeyDown, "up": tea.KeyUp, "left": tea.KeyLeft,
				"right": tea.KeyRight, "enter": tea.KeyEnter, "tab": tea.KeyTab, "esc": tea.KeyEsc,
				"backspace": tea.KeyBackspace, "pgdown": tea.KeyPgDown}[k]}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		next, cmd := m.Update(msg)
		m = drain(next.(Model), cmd)
	}
	return m
}

func text(m Model) string { return ansi.Strip(m.View()) }

// selectNode moves the cursor to the first visible row matching pred.
func selectNode(t *testing.T, m Model, pred func(*tree.Node) bool) Model {
	t.Helper()
	m.refreshRows()
	for i, r := range m.rows {
		if pred(r.Node) {
			m.cursor = i
			m.clampCursor()
			return m
		}
	}
	t.Fatalf("no visible row matches; rows:\n%s", rowLabels(m))
	return m
}

func rowLabels(m Model) string {
	var b strings.Builder
	for _, r := range m.rows {
		b.WriteString(strings.Repeat("  ", r.Indent) + r.Node.Label + "  [" + r.Node.Badge + "]\n")
	}
	return b.String()
}

func itemNamed(name string) func(*tree.Node) bool {
	return func(n *tree.Node) bool { return n.Item != nil && n.Item.Name == name }
}

func labelContains(s string) func(*tree.Node) bool {
	return func(n *tree.Node) bool { return strings.Contains(n.Label, s) }
}

// expandAll opens every node of the current view.
func expandAll(m Model) Model {
	var walk func(ns []*tree.Node)
	walk = func(ns []*tree.Node) {
		for _, n := range ns {
			m.state.Set(n, true)
			walk(n.Children)
		}
	}
	walk(m.roots)
	m.refreshRows()
	return m
}

func TestDefaultsAndViewSwitching(t *testing.T) {
	m, _ := fx(t, "", 120, 40)
	if m.view != viewHierarchy || m.agent != discovery.AgentClaude {
		t.Fatalf("default view/agent = %v/%s", m.view, m.agent)
	}
	out := text(m)
	for _, want := range []string{"Agent: Claude Code (a)", "View: Hierarchy (1-4)", "Working dir: . (project root) (d)", "Project context", "Global context"} {
		if !strings.Contains(out, want) {
			t.Errorf("hierarchy view missing %q\n%s", want, out)
		}
	}
	// Each view keeps its own cursor.
	m = key(m, "j", "j", "j")
	hierCursor := m.cursor
	m = key(m, "2")
	if m.view != viewEffective || !strings.Contains(text(m), "Applicable") {
		t.Fatalf("effective view not shown:\n%s", text(m))
	}
	if m.cursor != 0 {
		t.Errorf("effective cursor = %d, want 0", m.cursor)
	}
	m = key(m, "j", "j")
	effCursor := m.cursor
	m = key(m, "3")
	if !strings.Contains(text(m), "Project — ") {
		t.Errorf("filesystem view missing root group:\n%s", text(m))
	}
	m = key(m, "4")
	if !strings.Contains(text(m), "Instructions (") {
		t.Errorf("inventory view missing categories:\n%s", text(m))
	}
	m = key(m, "1")
	if m.cursor != hierCursor {
		t.Errorf("hierarchy cursor = %d, want %d", m.cursor, hierCursor)
	}
	m = key(m, "2")
	if m.cursor != effCursor {
		t.Errorf("effective cursor = %d, want %d", m.cursor, effCursor)
	}
}

func TestAgentCycle(t *testing.T) {
	m, _ := fx(t, "", 120, 40)
	m = key(m, "a")
	if m.agent != discovery.AgentCodex {
		t.Fatalf("agent = %s", m.agent)
	}
	if !strings.Contains(text(m), "Parent directories (not read by Codex)") {
		t.Errorf("codex hierarchy should mark parent directories:\n%s", text(m))
	}
	m = key(m, "a")
	if m.agent != discovery.AgentCursor {
		t.Fatalf("agent = %s", m.agent)
	}
	m = expandAll(m)
	m = selectNode(t, m, itemNamed("pdf"))
	if n := m.selected(); !strings.Contains(n.Item.Path, ".cursor/skills/pdf") {
		t.Errorf("cursor's pdf skill should be its own copy, got %s", n.Item.Path)
	}
	m = key(m, "a")
	if m.agent != discovery.AgentClaude {
		t.Errorf("agent should wrap to claude, got %s", m.agent)
	}
}

func TestAgentFilterRestrictsCycle(t *testing.T) {
	m, _ := fx(t, "", 120, 40, discovery.AgentCodex)
	if m.agent != discovery.AgentCodex {
		t.Fatalf("agent = %s", m.agent)
	}
	m = key(m, "a")
	if m.agent != discovery.AgentCodex {
		t.Errorf("single agent must not cycle, got %s", m.agent)
	}
}

func TestWorkingDirectoryPicker(t *testing.T) {
	m, _ := fx(t, "", 120, 40)
	m = key(m, "2", "d")
	if m.picker == nil {
		t.Fatal("d should open the picker")
	}
	view := text(m)
	for _, want := range []string{"Working directory", "services/api", "web", "launch dir"} {
		if !strings.Contains(view, want) {
			t.Errorf("picker missing %q\n%s", want, view)
		}
	}
	for i, d := range m.picker.dirs {
		if strings.HasSuffix(d, "services/api") {
			m.picker.cur = i
		}
	}
	m = key(m, "enter")
	if m.picker != nil || m.wd != m.base.Env.ProjectRoot+"/services/api" {
		t.Fatalf("wd = %s picker=%v", m.wd, m.picker != nil)
	}
	if !strings.Contains(text(m), "Working dir: services/api (d)") {
		t.Errorf("header should show the working dir:\n%s", text(m))
	}
	m = expandAll(m)
	group := func(label string) []string {
		var out []string
		for _, g := range m.roots {
			if strings.Contains(g.Label, label) {
				for _, c := range g.Children {
					out = append(out, c.Label)
				}
			}
		}
		return out
	}
	if got := group("Applicable"); !contains(got, "services/api/CLAUDE.md") {
		t.Errorf("Applicable = %v, want services/api/CLAUDE.md", got)
	}
	if got := group("Excluded"); !contains(got, "web/CLAUDE.md") {
		t.Errorf("Excluded = %v, want web/CLAUDE.md", got)
	}
	// Back at the root the nested file is conditional.
	m = key(m, "d")
	m.picker.cur = 0
	m = key(m, "enter")
	m = expandAll(m)
	if got := group("Conditional"); !contains(got, "services/api/CLAUDE.md") {
		t.Errorf("Conditional = %v, want services/api/CLAUDE.md", got)
	}
	// Esc cancels without changing anything.
	before := m.wd
	m = key(m, "d", "j", "esc")
	if m.picker != nil || m.wd != before {
		t.Error("esc should cancel the picker")
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func TestLaunchInSubdirectoryScansRootInBackground(t *testing.T) {
	m, _ := fx(t, "services/api", 120, 40)
	if !m.fullReady {
		t.Fatal("Init's command should have delivered the project-root scan")
	}
	if !strings.Contains(text(m), "Working dir: services/api (d)") {
		t.Errorf("header should show the launch dir:\n%s", text(m))
	}
	m = key(m, "2")
	m = expandAll(m)
	var excluded []string
	for _, g := range m.roots {
		if strings.Contains(g.Label, "Excluded") {
			for _, c := range g.Children {
				excluded = append(excluded, c.Label)
			}
		}
	}
	if !contains(excluded, "web/CLAUDE.md") {
		t.Errorf("Excluded = %v", excluded)
	}
}

func TestExplainStripAndPanel(t *testing.T) {
	m, _ := fx(t, "", 140, 40)
	m = expandAll(m)
	m = selectNode(t, m, itemNamed("frontend-design"))
	out := text(m)
	for _, want := range []string{"What", "From", "Loads", "Related", "Listed"} {
		if !strings.Contains(out, want) {
			t.Errorf("strip missing %q\n%s", want, out)
		}
	}
	m = key(m, "e")
	if !strings.Contains(text(m), "How does Claude Code discover it?") {
		t.Errorf("explain panel missing discovery section:\n%s", text(m))
	}
	m = key(m, "e")
	if strings.Contains(text(m), "How does Claude Code discover it?") {
		t.Error("e should toggle the panel off")
	}

	// Project AGENTS.md is shadowed by AGENTS.override.md for Codex.
	m = key(m, "a")
	m = expandAll(m)
	m = selectNode(t, m, func(n *tree.Node) bool {
		return n.Item != nil && n.Item.Path == filepath.Join(m.base.Env.ProjectRoot, "AGENTS.md")
	})
	if !strings.Contains(text(m), "Shadowed") {
		t.Errorf("Codex strip should say Shadowed for AGENTS.md:\n%s", text(m))
	}
	// With little height the strip disappears and the meta lines return.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 24})
	m = next.(Model)
	if strings.Contains(text(m), "Related") || !strings.Contains(text(m), "Source") {
		t.Errorf("short terminal should hide the strip and show Source:\n%s", text(m))
	}
}

// openSkillMD navigates to frontend-design/SKILL.md and focuses the preview.
func openSkillMD(t *testing.T, m Model) Model {
	t.Helper()
	m = key(m, "1")
	m = expandAll(m)
	m = selectNode(t, m, func(n *tree.Node) bool {
		return n.Item != nil && strings.HasSuffix(n.Item.Path, "/.claude/skills/frontend-design/SKILL.md") && !strings.Contains(n.Item.Path, "assets")
	})
	return key(m, "enter")
}

func selectLink(t *testing.T, m Model, text string) Model {
	t.Helper()
	for i := 0; i <= len(m.refs); i++ {
		m = key(m, "]")
		if m.refIdx >= 0 && m.refs[m.refIdx].Text == text {
			return m
		}
	}
	t.Fatalf("link %q not found among %v", text, m.refs)
	return m
}

func TestReaderFollowsLinksAndGoesBack(t *testing.T) {
	m, _ := fx(t, "", 140, 40)
	m = openSkillMD(t, m)
	if m.focus != focusPreview || len(m.refs) < 5 {
		t.Fatalf("focus=%v refs=%d", m.focus, len(m.refs))
	}
	if !strings.Contains(text(m), "[ ] links (") {
		t.Errorf("status line should hint at links:\n%s", text(m))
	}
	if !strings.Contains(text(m), "frontend-design") || !strings.Contains(text(m), "›") {
		t.Errorf("breadcrumb missing:\n%s", text(m))
	}
	m = key(m, "pgdown")
	m = key(m, "g")
	skillID := m.selected().ID
	m.vp.SetYOffset(3)
	off := m.vp.YOffset

	m = selectLink(t, m, "references/patterns.md")
	if !strings.Contains(text(m), "Link ") || !strings.Contains(text(m), "✓ (enter opens)") {
		t.Errorf("status should describe the link:\n%s", text(m))
	}
	m = key(m, "enter")
	if n := m.selected(); n == nil || n.Item == nil || n.Item.Name != "patterns.md" {
		t.Fatalf("tree cursor should land on patterns.md, got %+v", m.selected())
	}
	if m.focus != focusPreview {
		t.Error("preview should keep focus")
	}
	if !strings.Contains(text(m), "Patterns") {
		t.Errorf("preview should show patterns.md:\n%s", text(m))
	}
	m = key(m, "esc")
	if m.selected().ID != skillID || m.vp.YOffset != off || m.focus != focusPreview {
		t.Errorf("esc should restore SKILL.md at offset %d: id=%v off=%d focus=%v", off, m.selected().ID == skillID, m.vp.YOffset, m.focus)
	}

	// A missing link does not navigate.
	m = selectLink(t, m, "references/missing.md")
	before := m.selected().ID
	m = key(m, "enter")
	if m.selected().ID != before || m.adhoc != nil || !strings.Contains(text(m), "missing") {
		t.Errorf("missing link should only report:\n%s", text(m))
	}
	// An external URL is never fetched.
	m = selectLink(t, m, "https://example.com/design-guide")
	m = key(m, "enter")
	if m.adhoc != nil || !strings.Contains(text(m), "AYCA never fetches") {
		t.Errorf("URL should be refused:\n%s", text(m))
	}
	// With nothing to go back to, esc returns to the tree.
	m = key(m, "esc")
	if m.focus != focusTree {
		t.Error("esc with empty history should return to the tree")
	}
}

func TestReaderOpensAdHocDocument(t *testing.T) {
	m, f := fx(t, "", 140, 40)
	// docs/architecture.md is imported by the project CLAUDE.md; in the
	// Filesystem-free Effective view the @import is a node, so use a file
	// that no tree knows: a note the project README links to.
	notes := f.P("docs/notes.md")
	if err := os.WriteFile(notes, []byte("# Loose notes\n\nNot part of any agent context.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.P("CLAUDE.md"), []byte("# Project\n\nSee [notes](docs/notes.md) and [a](docs/architecture.md).\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m = key(m, "r") // rescan to pick the new CLAUDE.md up
	m = key(m, "1")
	m = selectNode(t, m, func(n *tree.Node) bool {
		return n.Item != nil && n.Item.Path == f.P("CLAUDE.md") && strings.HasPrefix(n.ID, "h:claude:")
	})
	m = key(m, "enter")
	m = selectLink(t, m, "docs/notes.md")
	m = key(m, "enter")
	if m.adhoc == nil || !strings.Contains(text(m), "Loose notes") {
		t.Fatalf("link outside the tree should open ad hoc:\n%s", text(m))
	}
	if !strings.Contains(text(m), "notes.md") || !strings.Contains(text(m), "Linked document") {
		t.Errorf("breadcrumb/strip should name the document:\n%s", text(m))
	}
	m = key(m, "backspace")
	if m.adhoc != nil || strings.Contains(text(m), "Loose notes") {
		t.Errorf("backspace should return to CLAUDE.md:\n%s", text(m))
	}
}

func TestLinksOutsideAllowedFoldersAreNotOpened(t *testing.T) {
	m, f := fx(t, "", 140, 40)
	secret := filepath.Join(f.Root, "elsewhere", "secret.md")
	os.MkdirAll(filepath.Dir(secret), 0o755)
	os.WriteFile(secret, []byte("# Secret\n"), 0o644)
	sk := f.H(".claude/skills/frontend-design/SKILL.md")
	b, _ := os.ReadFile(sk)
	os.WriteFile(sk, append(b, []byte("\nSee [out]("+secret+").\n")...), 0o644)
	m = key(m, "r")
	m = openSkillMD(t, m)
	m = selectLink(t, m, secret)
	m = key(m, "enter")
	if m.adhoc != nil || !strings.Contains(text(m), "not opened") {
		t.Errorf("outside link must not open:\n%s", text(m))
	}
}

func TestInDocumentSearch(t *testing.T) {
	m, _ := fx(t, "", 140, 40)
	m = openSkillMD(t, m)
	m = key(m, "/")
	if !m.finding {
		t.Fatal("/ in preview focus should open find")
	}
	m = key(m, "r", "e", "f", "e", "r", "e", "n", "c", "e", "s")
	if len(m.matches) < 2 {
		t.Fatalf("expected several matches for 'references', got %d", len(m.matches))
	}
	if !strings.Contains(text(m), "match 1/") {
		t.Errorf("footer should show match status:\n%s", text(m))
	}
	m = key(m, "enter")
	if m.finding {
		t.Error("enter should close the prompt")
	}
	first := m.matchIdx
	m = key(m, "n")
	if m.matchIdx == first {
		t.Error("n should advance")
	}
	m = key(m, "N")
	if m.matchIdx != first {
		t.Error("N should go back")
	}
	m = key(m, "esc")
	if len(m.matches) != 0 || m.focus != focusPreview {
		t.Error("esc should clear the search first")
	}
	m = key(m, "/", "z", "z", "z", "q")
	if !strings.Contains(text(m), "no match") {
		t.Errorf("expected 'no match':\n%s", text(m))
	}
	// In tree focus / still filters the tree.
	m = key(m, "esc", "esc")
	m = key(m, "/", "p", "d", "f", "enter")
	if m.filter.Value() != "pdf" {
		t.Errorf("filter = %q", m.filter.Value())
	}
}

func TestRenderingFitsSizes(t *testing.T) {
	for _, sz := range [][2]int{{120, 40}, {100, 30}, {80, 24}, {60, 20}, {40, 12}} {
		m, _ := fx(t, "", sz[0], sz[1])
		for _, views := range []string{"1", "2", "3", "4"} {
			m = key(m, views)
			m = expandAll(m)
			for step := 0; step < 3; step++ {
				for _, k := range []string{"", "enter", "?", "e", "c", "tab"} {
					if k != "" {
						m = key(m, k)
					}
					checkFit(t, m, sz[0], sz[1])
				}
				m = key(m, "j", "j", "j")
			}
		}
		m = key(m, "d")
		checkFit(t, m, sz[0], sz[1])
	}
}

func checkFit(t *testing.T, m Model, w, h int) {
	t.Helper()
	lines := strings.Split(m.View(), "\n")
	if len(lines) != h {
		t.Fatalf("%dx%d view %d: %d lines, want %d", w, h, m.view, len(lines), h)
	}
	for i, l := range lines {
		if ansi.StringWidth(l) > w {
			t.Fatalf("%dx%d view %d: line %d is %d wide: %q", w, h, m.view, i, ansi.StringWidth(l), ansi.Strip(l))
		}
	}
}

func TestSinglePaneMode(t *testing.T) {
	m, _ := fx(t, "", 60, 24)
	if strings.Contains(text(m), "│ Parent") { // sanity: no second pane
		t.Log("unexpected")
	}
	if m.previewInnerWidth() != 58 || m.treeWidth() != 60 {
		t.Fatalf("widths %d/%d", m.treeWidth(), m.previewInnerWidth())
	}
	if strings.Contains(text(m), "[ ] links") {
		t.Error("preview must be hidden while the tree has focus")
	}
	m = expandAll(m)
	m = selectNode(t, m, itemNamed("CLAUDE.md"))
	m = key(m, "enter")
	if m.focus != focusPreview || !strings.Contains(text(m), "›") {
		t.Errorf("enter should show the preview full width:\n%s", text(m))
	}
	m = key(m, "esc")
	if m.focus != focusTree {
		t.Error("esc should return to the tree")
	}
}

var secrets = []string{"hunter2", "ghp_FAKE", "sk-ant-api03-FAKE", "sk-live-FAKE"}

func TestNoSecretsAnywhere(t *testing.T) {
	m, _ := fx(t, "", 160, 50)
	seen := 0
	for _, v := range []string{"1", "2", "3", "4"} {
		for _, agent := range []int{0, 1, 2} {
			m = key(m, v)
			for a := 0; a < agent; a++ {
				m = key(m, "a")
			}
			m = expandAll(m)
			for i := range m.rows {
				n := m.rows[i].Node
				if n.Item == nil || (n.Item.Category != discovery.CategoryMCP && n.Item.Category != discovery.CategoryHook) {
					continue
				}
				m.cursor = i
				m.clampCursor()
				seen++
				for _, panel := range []string{"", "e"} {
					if panel != "" {
						m = key(m, panel)
					}
					all := text(m) + "\n" + ansi.Strip(m.cur) + "\n" + ansi.Strip(m.explainMarkdown()) + "\n" + strings.Join(m.stripLines(200), "\n")
					for _, s := range secrets {
						if strings.Contains(all, s) {
							t.Errorf("secret %q leaked for %s in view %d", s, n.Label, m.view)
						}
					}
					m = key(m, "v")
					if strings.Contains(ansi.Strip(m.cur), "hunter2") {
						t.Errorf("secret in raw view for %s", n.Label)
					}
					m = key(m, "v")
					if panel != "" {
						m = key(m, panel)
					}
				}
			}
			m = key(m, "a", "a", "a")
		}
	}
	if seen < 8 {
		t.Fatalf("only %d MCP/hook nodes exercised", seen)
	}
}

func snapshot(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil {
			out = append(out, p+"|"+info.ModTime().Format(time.RFC3339Nano)+"|"+info.Mode().String())
		}
		return nil
	})
	sort.Strings(out)
	return out
}

func TestReadOnly(t *testing.T) {
	m, f := fx(t, "", 140, 40)
	before := snapshot(t, f.Root)
	for _, v := range []string{"1", "2", "3", "4"} {
		m = key(m, v)
		m = expandAll(m)
		for i := range m.rows {
			if i%3 != 0 {
				continue
			}
			m.cursor = i
			m.clampCursor()
			_ = m.View()
			m = key(m, "enter", "]", "]", "enter", "e", "e", "esc", "esc")
			m.focus = focusTree
		}
	}
	m = key(m, "d", "j", "enter", "r")
	after := snapshot(t, f.Root)
	if strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Error("the fixture changed while browsing")
	}
}

func TestFollowLinkLandsOnDirectNode(t *testing.T) {
	m, _ := fx(t, "", 140, 40)
	m = openSkillMD(t, m)
	m = selectLink(t, m, "references/accessibility.md")
	m = key(m, "enter")
	n := m.selected()
	if n == nil || n.Item == nil || !strings.HasSuffix(n.Item.Path, "/.claude/skills/frontend-design/references/accessibility.md") {
		t.Fatalf("cursor = %+v", n)
	}
	if !strings.Contains(text(m), "frontend-design › references › accessibility.md") {
		t.Errorf("breadcrumb:\n%s", text(m))
	}
}

func TestCycleNodeRendersAsLeaf(t *testing.T) {
	m, _ := fx(t, "", 140, 40)
	m = key(m, "1")
	m = expandAll(m)
	m = selectNode(t, m, func(n *tree.Node) bool {
		return n.Item != nil && strings.HasSuffix(n.Item.Path, "frontend-design/assets/loop")
	})
	out := text(m)
	if strings.Contains(out, "loop/assets") {
		t.Errorf("cycle expanded:\n%s", out)
	}
	if !strings.Contains(out, "not followed (cycle)") {
		t.Errorf("note not visible:\n%s", out)
	}
}

func TestWorkingDirWording(t *testing.T) {
	m, _ := fx(t, "", 140, 40)
	if !strings.Contains(text(m), "Working dir: . (project root)") {
		t.Error("header should explain '.'")
	}
	if got := m.wdPhrase(); got != "the project root" {
		t.Errorf("root phrase = %q", got)
	}
	m, _ = fx(t, "services/api", 140, 40)
	if got := m.wdPhrase(); got != "`services/api/`" {
		t.Errorf("sub phrase = %q", got)
	}
}
