package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/R037/ayca/internal/preview"
	"github.com/R037/ayca/internal/tree"
)

// adhoc is a document opened by following a link that has no node in the
// current tree. It is shown in the preview and never changes the tree.
type adhoc struct {
	content preview.Content
	name    string
	trail   []string // names of the documents followed so far, including this one
	topDir  string   // folder that bounds which links may be followed from here
}

// histEntry is a place to return to with esc/backspace.
type histEntry struct {
	view           viewID
	agent          string
	selID          string
	cursor, offset int
	adhoc          *adhoc
	yoff           int
	refIdx         int
}

func newFindInput() textinput.Model {
	ti := textinput.New()
	ti.Prompt = "find: "
	ti.Placeholder = "text in this document"
	return ti
}

// ---------------------------------------------------------------- documents

// document is what the preview currently shows as a file, if anything.
func (m *Model) document() (preview.Content, bool) {
	if m.panel != panelNone {
		return preview.Content{}, false
	}
	if m.adhoc != nil {
		return m.adhoc.content, true
	}
	if n := m.selected(); n != nil && n.Item != nil {
		return m.content(n.Item), true
	}
	return preview.Content{}, false
}

// topDir is the folder that bounds links followed from the current document.
func (m *Model) topDir() string {
	if m.adhoc != nil {
		return m.adhoc.topDir
	}
	chain := m.chainFor(m.selected())
	if len(chain) == 0 {
		return ""
	}
	top := chain[0]
	if top.Path == "" || top.Virtual {
		return ""
	}
	if top.IsDir {
		return top.Path
	}
	return filepath.Dir(top.Path)
}

func realOr(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

func within(dir, p string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}

// allowed returns the predicate that decides which existing link targets
// may be opened: the item's own folder, the project and the agents' config
// folders. Nothing else is ever read.
func (m *Model) allowed() func(string) bool {
	env := m.base.Env
	var roots []string
	for _, d := range []string{m.topDir(), env.ProjectRoot, env.ClaudeDir, env.CodexHome, env.CursorDir, env.AgentsDir, env.ManagedDir} {
		if d != "" {
			roots = append(roots, realOr(d))
		}
	}
	if d := filepath.Dir(env.ClaudeJSON); env.ClaudeJSON != "" && d != env.Home {
		roots = append(roots, realOr(d)) // a relocated CLAUDE_CONFIG_DIR
	}
	return func(abs string) bool {
		r := realOr(abs)
		for _, root := range roots {
			if within(root, r) {
				return true
			}
		}
		return false
	}
}

func (m *Model) computeRefs() {
	m.refs, m.refIdx = nil, -1
	c, ok := m.document()
	if !ok {
		return
	}
	m.refs = preview.References(c, preview.RefOptions{Home: m.base.Env.Home, Allowed: m.allowed()})
}

// breadcrumb is the location line above the preview.
func (m *Model) breadcrumb(width int) string {
	var parts []string
	switch {
	case m.panel != panelNone:
		parts = []string{panelTitle(m.panel)}
	default:
		parts = m.crumbs(m.selected())
		if m.adhoc != nil {
			parts = append(parts, m.adhoc.trail...)
		}
	}
	return mutedStyle.Render(preview.Breadcrumb(parts, width))
}

// statusLine shows the selected link, or a hint that links exist.
func (m *Model) statusLine(width int) string {
	if m.panel != panelNone || len(m.refs) == 0 {
		return ""
	}
	if m.refIdx < 0 {
		return mutedStyle.Render(fmt.Sprintf("[ ] links (%d)", len(m.refs)))
	}
	r := m.refs[m.refIdx]
	var verdict string
	switch r.Status {
	case preview.RefOK:
		verdict = "✓ (enter opens)"
	case preview.RefDir:
		verdict = "directory"
		if tree.FindPath(m.roots, r.Target) != "" {
			verdict = "directory (enter jumps to it)"
		}
	case preview.RefMissing:
		verdict = warnStyle.Render("✗ missing")
	case preview.RefOutside:
		verdict = warnStyle.Render("outside the skill and config folders — not opened")
	default:
		verdict = mutedStyle.Render("URL — AYCA never fetches")
	}
	// The verdict matters most: shorten the link text to make room for it.
	prefix := fmt.Sprintf("Link %d/%d → ", m.refIdx+1, len(m.refs))
	room := width - ansi.StringWidth(prefix) - ansi.StringWidth(verdict) - 1
	return prefix + truncateLeft(r.Text, max(room, 8)) + " " + verdict
}

// ---------------------------------------------------------------- links

func (m *Model) selectRef(delta int) {
	n := len(m.refs)
	if n == 0 {
		return
	}
	switch {
	case m.refIdx < 0 && delta < 0:
		m.refIdx = n - 1
	case m.refIdx < 0:
		m.refIdx = 0
	default:
		m.refIdx = (m.refIdx + delta + n) % n
	}
	m.applyDecor()
	if l := m.refLine(m.refs[m.refIdx]); l >= 0 {
		m.scrollTo(l)
	}
}

func (m *Model) refLine(r preview.Ref) int {
	if l := preview.LineOf(m.cur, r.Text); l >= 0 {
		return l
	}
	return preview.LineOf(m.cur, filepath.Base(strings.TrimPrefix(r.Text, "@")))
}

func (m *Model) scrollTo(line int) {
	if line < m.vp.YOffset || line >= m.vp.YOffset+m.vp.Height {
		m.vp.SetYOffset(max(line-m.vp.Height/3, 0))
	}
}

// applyDecor re-sets the viewport content with the current match or link
// highlighted, keeping the scroll position.
func (m *Model) applyDecor() {
	out, y := m.cur, m.vp.YOffset
	switch {
	case len(m.matches) > 0 && m.matchIdx >= 0 && m.matchIdx < len(m.matches):
		out = preview.HighlightLine(out, m.matches[m.matchIdx], selectedStyle)
	case m.refIdx >= 0 && m.refIdx < len(m.refs):
		if l := m.refLine(m.refs[m.refIdx]); l >= 0 {
			out = preview.HighlightLine(out, l, selectedStyle)
		}
	}
	m.vp.SetContent(out)
	m.vp.SetYOffset(y)
}

func (m *Model) pushHistory() {
	h := histEntry{view: m.view, agent: m.agent, cursor: m.cursor, offset: m.offset,
		adhoc: m.adhoc, yoff: m.vp.YOffset, refIdx: m.refIdx}
	if n := m.selected(); n != nil {
		h.selID = n.ID
	}
	m.history = append(m.history, h)
}

// openRef follows the selected link: to its node when the tree has one,
// otherwise as an ad-hoc document. Missing, outside and external targets
// are never opened; the status line says why.
func (m *Model) openRef() {
	if m.refIdx < 0 || m.refIdx >= len(m.refs) {
		return
	}
	r := m.refs[m.refIdx]
	switch r.Status {
	case preview.RefOK, preview.RefDir:
	default:
		return
	}
	trail := []string(nil)
	if m.adhoc != nil {
		trail = m.adhoc.trail
	}
	top := m.topDir()
	if id := tree.FindPath(m.roots, r.Target); id != "" {
		m.pushHistory()
		m.adhoc = nil
		m.state.ExpandTo(m.roots, id)
		m.refreshRows()
		i := tree.IndexOf(m.rows, id)
		if i < 0 && m.filter.Value() != "" {
			m.filter.SetValue("")
			m.refreshRows()
			i = tree.IndexOf(m.rows, id)
		}
		if i >= 0 {
			m.cursor = i
		}
		m.previewID = ""
		m.clampCursor()
		return
	}
	if r.Status == preview.RefDir {
		return
	}
	c := preview.LoadPath(r.Target)
	m.pushHistory()
	name := filepath.Base(r.Target)
	m.adhoc = &adhoc{content: c, name: name, trail: append(append([]string(nil), trail...), name), topDir: top}
	m.previewID = ""
	m.syncPreview()
}

// goBack pops the history and restores the place exactly. It reports
// whether there was anything to go back to.
func (m *Model) goBack() bool {
	if len(m.history) == 0 {
		return false
	}
	h := m.history[len(m.history)-1]
	m.history = m.history[:len(m.history)-1]
	m.panel = panelNone
	if h.view != m.view || (h.view.perAgent() && h.agent != m.agent) {
		m.switchTo(h.view, h.agent)
	}
	m.agent = h.agent
	m.adhoc = h.adhoc
	m.refreshRows()
	m.cursor, m.offset = h.cursor, h.offset
	if i := tree.IndexOf(m.rows, h.selID); i >= 0 {
		m.cursor = i
	}
	m.previewID = ""
	m.clampCursor()
	m.syncPreview()
	m.vp.SetYOffset(h.yoff)
	m.refIdx = min(h.refIdx, len(m.refs)-1)
	m.applyDecor()
	return true
}

// ---------------------------------------------------------------- find

func (m *Model) clearFind() {
	m.finding = false
	m.find.Blur()
	m.find.SetValue("")
	m.matches, m.matchIdx = nil, -1
}

func (m *Model) startFind() tea.Cmd {
	if _, ok := m.document(); !ok {
		return nil
	}
	m.finding = true
	m.findFrom = m.vp.YOffset
	return m.find.Focus()
}

func (m *Model) runFind() {
	m.matches = preview.FindLines(m.cur, m.find.Value())
	m.matchIdx = -1
	for i, l := range m.matches {
		if l >= m.findFrom {
			m.matchIdx = i
			break
		}
	}
	if m.matchIdx < 0 && len(m.matches) > 0 {
		m.matchIdx = 0
	}
	m.applyDecor()
	if m.matchIdx >= 0 {
		m.scrollTo(m.matches[m.matchIdx])
	}
}

func (m *Model) nextMatch(delta int) {
	n := len(m.matches)
	if n == 0 {
		return
	}
	m.matchIdx = (m.matchIdx + delta + n) % n
	m.applyDecor()
	m.scrollTo(m.matches[m.matchIdx])
}

// findStatus is shown in the footer while a search is active.
func (m Model) findStatus() string {
	if m.find.Value() == "" {
		return ""
	}
	if len(m.matches) == 0 {
		return "no match"
	}
	return fmt.Sprintf("match %d/%d", m.matchIdx+1, len(m.matches))
}

func (m Model) updateFind(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.clearFind()
		m.applyDecor()
		return m, nil
	case "enter":
		m.finding = false
		m.find.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	before := m.find.Value()
	m.find, cmd = m.find.Update(msg)
	if m.find.Value() != before {
		m.runFind()
	}
	return m, cmd
}

// ---------------------------------------------------------------- preview keys

func (m Model) updatePreview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.find.Value() != "" {
			m.clearFind()
			m.applyDecor()
			return m, nil
		}
		if !m.goBack() {
			m.focus = focusTree
		}
		return m, nil
	case "backspace":
		m.goBack()
		return m, nil
	case "left", "h":
		m.focus = focusTree
		return m, nil
	case "g", "home":
		m.vp.GotoTop()
		return m, nil
	case "G", "end":
		m.vp.GotoBottom()
		return m, nil
	case "]":
		m.selectRef(1)
		return m, nil
	case "[":
		m.selectRef(-1)
		return m, nil
	case "enter":
		m.openRef()
		return m, nil
	case "n":
		m.nextMatch(1)
		return m, nil
	case "N":
		m.nextMatch(-1)
		return m, nil
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}
