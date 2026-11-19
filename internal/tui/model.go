// Package tui is the Bubble Tea interface: a context tree on the left and a
// document reader on the right, with four perspectives on the same context
// (Hierarchy, Effective, Filesystem, Inventory), an explain strip and
// in-document links and search. It is read-only: it never executes or
// fetches anything it finds.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/R037/ayca/internal/budget"
	"github.com/R037/ayca/internal/discovery"
	"github.com/R037/ayca/internal/explain"
	"github.com/R037/ayca/internal/preview"
	"github.com/R037/ayca/internal/tree"
)

type focus int

const (
	focusTree focus = iota
	focusPreview
)

// scanDoneMsg carries a full rescan: the launch directory, the project
// root and the selected working directory.
type scanDoneMsg struct{ base, full, at discovery.Result }

// panel is a full-pane view that replaces the selected item's preview.
type panel string

const (
	panelNone     panel = ""
	panelWarnings panel = "warnings"
	panelBudget   panel = "budget"
	panelExplain  panel = "explain"
	panelHelp     panel = "help"
)

// singlePaneWidth is the terminal width below which only one pane is shown.
const singlePaneWidth = 72

// Model is the root Bubble Tea model.
type Model struct {
	env     discovery.Env // the launch environment
	agents  []string
	windows map[string]int
	budget  budget.Report

	// base is the startup scan (launch directory); full is the scan at the
	// project root; at is the scan at the selected working directory.
	base, full, at discovery.Result
	fullReady      bool
	wd             string
	atLoading      bool

	view   viewID
	agent  string
	mem    map[string]viewMem
	roots  []*tree.Node
	chains map[string][]*discovery.Item // item chain behind Effective nodes
	state  *tree.State
	rows   []tree.Row

	cursor, offset int
	focus          focus

	filter    textinput.Model
	filtering bool
	picker    *picker

	raw      bool
	panel    panel
	scanning bool

	vp        viewport.Model
	renderer  *preview.Renderer
	contents  map[string]preview.Content
	rendered  map[string]string
	explCache map[string]explain.Explanation
	previewID string
	cur       string // rendered text of the current document

	// reader
	adhoc    *adhoc
	history  []histEntry
	refs     []preview.Ref
	refIdx   int
	find     textinput.Model
	finding  bool
	findFrom int
	matches  []int
	matchIdx int

	width, height int
}

// New creates the model from an initial discovery result. windows overrides
// the assumed context window size per agent (nil uses the defaults).
func New(res discovery.Result, agents []string, windows map[string]int, renderer *preview.Renderer) Model {
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.Placeholder = "filter by name or path"
	m := Model{
		env:       res.Env,
		agents:    agents,
		windows:   windows,
		state:     tree.NewState(),
		mem:       map[string]viewMem{},
		filter:    ti,
		find:      newFindInput(),
		vp:        viewport.New(0, 0),
		renderer:  renderer,
		base:      res,
		at:        res,
		wd:        res.Env.Cwd,
		fullReady: res.Env.Cwd == res.Env.ProjectRoot,
		refIdx:    -1,
		matchIdx:  -1,
		contents:  map[string]preview.Content{},
		rendered:  map[string]string{},
		explCache: map[string]explain.Explanation{},
	}
	if m.fullReady {
		m.full = res
	}
	m.agent = defaultAgent(res, m.enabledAgents())
	m.budget = budget.Estimate(res, agents, windows)
	m.rebuild()
	m.refreshRows()
	m.clampCursor()
	return m
}

// WithView starts in the named view ("hierarchy", "effective", "filesystem"
// or "inventory").
func (m Model) WithView(name string) (Model, error) {
	v, ok := parseView(name)
	if !ok {
		return m, fmt.Errorf("unknown view %q (supported: %s)", name, strings.Join(ViewNames(), ", "))
	}
	if v != m.view {
		m.switchTo(v, m.agent)
	}
	return m, nil
}

// dataChanged rebuilds everything derived from the scans, keeping the
// selection where possible.
func (m *Model) dataChanged() {
	var keepID string
	if n := m.selected(); n != nil {
		keepID = n.ID
	}
	m.budget = budget.Estimate(m.base, m.agents, m.windows)
	m.contents = map[string]preview.Content{}
	m.rendered = map[string]string{}
	m.explCache = map[string]explain.Explanation{}
	m.adhoc, m.history = nil, nil
	m.previewID = ""
	m.rebuild()
	m.refreshRows()
	if i := tree.IndexOf(m.rows, keepID); i >= 0 {
		m.cursor = i
	}
	m.clampCursor()
}

func (m *Model) setAt(res discovery.Result) {
	m.at = res
	m.dataChanged()
}

func (m *Model) refreshRows() {
	m.rows = tree.Visible(m.roots, m.state, m.filter.Value())
}

func (m Model) selected() *tree.Node {
	if m.cursor >= 0 && m.cursor < len(m.rows) {
		return m.rows[m.cursor].Node
	}
	return nil
}

// Init implements tea.Model. When the app was started in a subdirectory,
// the project-root scan (for Excluded and the directory picker) runs in the
// background.
func (m Model) Init() tea.Cmd {
	if m.fullReady {
		return nil
	}
	env, agents := m.env, m.agents
	return func() tea.Msg {
		return fullDoneMsg(discovery.Discover(env.WithCwd(env.ProjectRoot), agents))
	}
}

func (m Model) rescan() tea.Cmd {
	env, agents, wd := m.env, m.agents, m.wd
	return func() tea.Msg {
		base := discovery.Discover(env, agents)
		full, at := base, base
		if env.Cwd != env.ProjectRoot {
			full = discovery.Discover(env.WithCwd(env.ProjectRoot), agents)
		}
		if wd != env.Cwd {
			at = discovery.Discover(env.WithCwd(wd), agents)
		}
		return scanDoneMsg{base: base, full: full, at: at}
	}
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.rendered = map[string]string{}
		m.previewID = ""
		m.layout()
		return m, nil

	case scanDoneMsg:
		m.scanning = false
		m.base, m.full, m.at, m.fullReady = msg.base, msg.full, msg.at, true
		m.dataChanged()
		m.layout()
		return m, nil

	case fullDoneMsg:
		m.full, m.fullReady = discovery.Result(msg), true
		m.dataChanged()
		return m, nil

	case atDoneMsg:
		if msg.dir != m.wd {
			return m, nil // superseded by a later choice
		}
		m.atLoading = false
		m.setAt(msg.res)
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.picker != nil {
			return m.updatePicker(msg)
		}
		if m.filtering {
			return m.updateFilter(msg)
		}
		if m.finding {
			return m.updateFind(msg)
		}
		// Fast typing or pasting can deliver several runes in one message;
		// replay them one by one so e.g. "/foo" starts a search for "foo".
		if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 && !msg.Paste {
			var model tea.Model = m
			var cmds []tea.Cmd
			for _, r := range msg.Runes {
				var cmd tea.Cmd
				model, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				cmds = append(cmds, cmd)
			}
			return model, tea.Batch(cmds...)
		}
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "tab", "shift+tab":
			m.toggleFocus()
			return m, nil
		case "r":
			if !m.scanning {
				m.scanning = true
				return m, m.rescan()
			}
			return m, nil
		case "v":
			m.raw = !m.raw
			m.previewID = ""
			m.syncPreview()
			return m, nil
		case "w":
			m.togglePanel(panelWarnings)
			return m, nil
		case "c":
			m.togglePanel(panelBudget)
			return m, nil
		case "e":
			m.togglePanel(panelExplain)
			return m, nil
		case "?":
			m.togglePanel(panelHelp)
			return m, nil
		case "1", "2", "3", "4":
			m.panel = panelNone
			v := viewID(msg.Runes[0] - '1')
			if v != m.view {
				m.history = nil
				m.switchTo(v, m.agent)
			}
			return m, nil
		case "a":
			m.cycleAgent()
			return m, nil
		case "d":
			m.openPicker()
			return m, nil
		case "/":
			if m.focus == focusPreview {
				return m, m.startFind()
			}
			m.filtering = true
			return m, m.filter.Focus()
		}
		if m.focus == focusPreview {
			return m.updatePreview(msg)
		}
		return m.updateTree(msg)
	}
	return m, nil
}

func (m *Model) toggleFocus() {
	if m.focus == focusTree {
		m.focus = focusPreview
	} else {
		m.focus = focusTree
	}
}

func (m Model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filtering = false
		m.filter.Blur()
		m.filter.SetValue("")
		m.afterFilterChange()
		return m, nil
	case "enter":
		m.filtering = false
		m.filter.Blur()
		return m, nil
	case "up", "down":
		return m.updateTree(msg)
	}
	var cmd tea.Cmd
	before := m.filter.Value()
	m.filter, cmd = m.filter.Update(msg)
	if m.filter.Value() != before {
		m.afterFilterChange()
	}
	return m, cmd
}

// afterFilterChange recomputes rows and moves the cursor to the first match.
func (m *Model) afterFilterChange() {
	m.refreshRows()
	m.cursor = 0
	if m.filter.Value() != "" {
		for i, r := range m.rows {
			if r.Match {
				m.cursor = i
				break
			}
		}
	}
	m.clampCursor()
}

func (m Model) updateTree(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	page := max(m.treeHeight()-1, 1)
	switch msg.String() {
	case "up", "k":
		m.cursor--
	case "down", "j":
		m.cursor++
	case "pgup", "ctrl+u":
		m.cursor -= page
	case "pgdown", "ctrl+d":
		m.cursor += page
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.cursor = len(m.rows) - 1
	case "right", "l":
		if r, ok := m.row(); ok && r.Node.HasChildren() {
			if !r.Expanded {
				m.state.Set(r.Node, true)
				m.refreshRows()
			} else {
				m.cursor++
			}
		}
	case "left", "h":
		if r, ok := m.row(); ok {
			if r.Expanded {
				m.state.Set(r.Node, false)
				m.refreshRows()
			} else if p := tree.Parent(m.rows, m.cursor); p >= 0 {
				m.cursor = p
			}
		}
	case " ":
		if r, ok := m.row(); ok && r.Node.HasChildren() {
			m.state.Set(r.Node, !r.Expanded)
			m.refreshRows()
		}
	case "enter":
		if r, ok := m.row(); ok {
			if r.Node.Kind == tree.KindItem || !r.Node.HasChildren() {
				m.focus = focusPreview
			} else {
				m.state.Set(r.Node, !r.Expanded)
				m.refreshRows()
			}
		}
	case "esc":
		if m.filter.Value() != "" {
			m.filter.SetValue("")
			m.refreshRows()
		} else if m.panel != panelNone {
			m.panel = panelNone
		}
	}
	m.clampCursor()
	return m, nil
}

func (m Model) row() (tree.Row, bool) {
	if m.cursor >= 0 && m.cursor < len(m.rows) {
		return m.rows[m.cursor], true
	}
	return tree.Row{}, false
}

func (m *Model) clampCursor() {
	if len(m.rows) == 0 {
		m.cursor, m.offset = 0, 0
		m.syncPreview()
		return
	}
	m.cursor = max(0, min(m.cursor, len(m.rows)-1))
	h := m.treeHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if h > 0 && m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	m.offset = max(0, min(m.offset, max(len(m.rows)-h, 0)))
	m.syncPreview()
}

// ---------------------------------------------------------------- layout

const (
	headerLines = 2
	footerLines = 1
)

func (m Model) singlePane() bool { return m.width < singlePaneWidth }

func (m Model) treeWidth() int {
	if m.singlePane() {
		return m.width
	}
	return max(28, min(m.width*2/5, 64))
}

func (m Model) bodyHeight() int {
	h := m.height - headerLines - footerLines
	if m.stripVisible() {
		h -= stripHeight
	}
	return max(h, 3)
}
func (m Model) treeHeight() int { return m.bodyHeight() - 2 }

func (m Model) previewInnerWidth() int {
	if m.singlePane() {
		return max(m.width-2, 10)
	}
	return max(m.width-m.treeWidth()-2, 10)
}

// headLines are the breadcrumb and status lines above the document.
const headLines = 2

func (m *Model) viewportHeight() int {
	return max(m.treeHeight()-headLines-len(m.metaLines()), 1)
}

func (m *Model) layout() {
	m.vp.Width = m.previewInnerWidth()
	m.vp.Height = m.viewportHeight()
	m.clampCursor()
}

// ---------------------------------------------------------------- preview

func (m *Model) togglePanel(p panel) {
	if m.panel == p {
		m.panel = panelNone
	} else {
		m.panel = p
	}
	m.previewID = ""
	m.syncPreview()
}

func (m *Model) previewKey() string {
	k := fmt.Sprintf("%d|%s|", m.view, m.agent)
	n := m.selected()
	switch {
	case m.panel == panelExplain:
		k += "panel:explain:"
		if n != nil {
			k += n.ID
		}
	case m.panel != panelNone:
		k += "panel:" + string(m.panel)
	case m.adhoc != nil:
		k += "adhoc:" + m.adhoc.content.SourcePath
	case n != nil:
		k += n.ID
	default:
		k += "none"
	}
	return k
}

func (m *Model) syncPreview() {
	if m.width == 0 {
		return
	}
	id := m.previewKey()
	if id == m.previewID {
		return
	}
	m.previewID = id
	m.clearFind()
	m.vp.Width = m.previewInnerWidth()
	key := fmt.Sprintf("%s|%d|%v", id, m.vp.Width, m.raw)
	out, ok := m.rendered[key]
	if !ok {
		out = m.renderPreview()
		m.rendered[key] = out
	}
	m.cur = out
	m.computeRefs()
	m.vp.Height = m.viewportHeight()
	m.vp.SetContent(out)
	m.vp.GotoTop()
}

func (m *Model) content(it *discovery.Item) preview.Content {
	c, ok := m.contents[it.ID]
	if !ok {
		c = preview.Load(it)
		m.contents[it.ID] = c
	}
	return c
}

func (m *Model) renderPreview() string {
	w := m.vp.Width - 1
	md := func(s string) string {
		return m.renderer.Render(preview.Content{Text: s, Format: preview.FormatMarkdown}, w, false)
	}
	switch m.panel {
	case panelWarnings:
		return md(m.warningsMarkdown())
	case panelBudget:
		return md(m.budgetMarkdown())
	case panelExplain:
		return md(m.explainMarkdown())
	case panelHelp:
		return md(helpMarkdown)
	}
	if m.adhoc != nil {
		return m.renderer.Render(m.adhoc.content, w, m.raw)
	}
	n := m.selected()
	if n == nil {
		return md(emptyMarkdown)
	}
	if n.Item == nil {
		return md(m.groupPreview(n))
	}
	return m.renderer.Render(m.content(n.Item), w, m.raw)
}

// groupPreview is the Markdown shown for a group, folder or category node.
func (m Model) groupPreview(n *tree.Node) string {
	if n.Kind == tree.KindCategory || n.Kind == tree.KindScope {
		return m.groupMarkdown(n)
	}
	var b strings.Builder
	b.WriteString("# " + countSuffix.ReplaceAllString(n.Label, "") + "\n\n")
	if n.Help != "" {
		b.WriteString(n.Help)
	} else if n.Kind == tree.KindDir {
		b.WriteString("A directory.")
	}
	if m.view == viewEffective && strings.HasPrefix(n.ID, "e:") {
		b.WriteString(legendMarkdown())
		b.WriteString(projectionFooter())
	} else {
		b.WriteString(honesty)
	}
	return b.String()
}

// metaLines describe the selected item: path, size and, when the explain
// strip is hidden, category, scope, agents and startup cost.
func (m *Model) metaLines() []string {
	w := m.previewInnerWidth()
	var lines []string
	if c, ok := m.document(); ok {
		lines = m.docMeta(c, w)
	}
	if len(lines) == 0 {
		return nil
	}
	// Leave room for at least three lines of document.
	room := m.treeHeight() - headLines - 3 - 1
	if room <= 0 {
		return nil
	}
	if len(lines) > room {
		lines = lines[:room]
	}
	return append(lines, mutedStyle.Render(strings.Repeat("─", w)))
}

func (m *Model) docMeta(c preview.Content, w int) []string {
	field := func(label, value string) string {
		return labelStyle.Render(label) + value
	}
	// Paths keep their tail (the file name) when they don't fit.
	pathField := func(label, p string) string {
		return field(label, truncateLeft(m.env.Tilde(p), w-9))
	}
	var lines []string
	if m.adhoc != nil {
		if c.SourcePath != "" {
			lines = append(lines, pathField("Path", c.SourcePath))
		}
		lines = append(lines, field("File", m.fileDetails(c)))
		return truncateAll(lines, w)
	}
	n := m.selected()
	if n == nil || n.Item == nil {
		return nil
	}
	it := n.Item
	path := it.Path
	if c.SourcePath != "" && c.SourcePath != it.Path {
		path = c.SourcePath
	}
	if path != "" {
		lines = append(lines, pathField("Path", path))
	}
	if it.Target != "" {
		lines = append(lines, pathField("Links to", it.Target))
	}
	for _, a := range it.Aliases {
		lines = append(lines, pathField("Also at", a))
	}
	if it.Info == "" && !it.Missing {
		lines = append(lines, field("File", m.fileDetails(c)))
	}
	if !m.stripVisible() {
		src := fmt.Sprintf("%s · %s", tree.CategoryLabel(it.Category), scopeStyle(it.Scope).Render(string(it.Scope)))
		if len(it.Agents) > 0 {
			src += " · " + strings.Join(it.Agents, ", ")
		}
		lines = append(lines, field("Source", src))
		if en, ok := m.budget.ByID[it.ID]; ok {
			lines = append(lines, field("Startup", fmt.Sprintf("~%s tokens in context at session start (%s)", budget.Human(en.Tokens), en.How)))
		} else if it.DeferredForAll() {
			lines = append(lines, field("Startup", "not loaded at session start"))
		}
		if it.Note != "" {
			lines = append(lines, field("Note", it.Note))
		}
	}
	return truncateAll(lines, w)
}

func (m *Model) fileDetails(c preview.Content) string {
	var details []string
	if c.Size >= 0 {
		details = append(details, humanSize(c.Size))
	}
	if t := c.Tokens(); t > 0 {
		details = append(details, fmt.Sprintf("~%s tokens (est.)", humanCount(t)))
	}
	mode := "rendered"
	if m.raw {
		mode = "raw"
	}
	details = append(details, mode+" view")
	return strings.Join(details, " · ")
}

func truncateAll(lines []string, w int) []string {
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, w, "…")
	}
	return lines
}

// ---------------------------------------------------------------- view

// View implements tea.Model.
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	parts := []string{m.headerView(), m.bodyView()}
	if m.stripVisible() {
		parts = append(parts, m.stripView())
	}
	parts = append(parts, m.footerView())
	return fit(lipgloss.JoinVertical(lipgloss.Left, parts...), m.width, m.height)
}

// fit guarantees exactly height lines, none wider than width.
func fit(s string, width, height int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	for i, l := range lines {
		if ansi.StringWidth(l) > width {
			lines[i] = ansi.Truncate(l, width, "")
		}
	}
	return strings.Join(lines, "\n")
}

type headerPart struct {
	text string
	drop int // higher drops first; 0 never drops
}

func (m Model) headerView() string {
	title := titleStyle.Render("AYCA — All Your Context Are Belong To Us") + "  " + taglineStyle.Render("Know what your agents know.")

	parts := []headerPart{
		{mutedStyle.Render("Project: ") + m.env.Tilde(m.base.Env.ProjectRoot), 4},
		{mutedStyle.Render("Agent: ") + accentStyle.Render(explain.AgentName(m.agent)) + mutedStyle.Render(" (a)"), 0},
		{mutedStyle.Render("View: ") + accentStyle.Render(viewNames[m.view]) + mutedStyle.Render(" (1-4)"), 0},
	}
	wd := mutedStyle.Render("Working dir: ") + m.wdHeader() + mutedStyle.Render(" (d)")
	if m.atLoading {
		wd += " " + warnStyle.Render("loading…")
	}
	parts = append(parts, headerPart{wd, 1})
	if s := m.budgetSummary(); s != "" {
		parts = append(parts, headerPart{mutedStyle.Render("startup context on disk: ") + s + mutedStyle.Render(" (c)"), 5})
	}
	if n := len(m.base.Warnings); n > 0 {
		parts = append(parts, headerPart{warnStyle.Render(fmt.Sprintf("⚠ %d warning%s (w)", n, plural(n))), 2})
	}
	if m.scanning {
		parts = append(parts, headerPart{mutedStyle.Render("rescanning…"), 0})
	}
	const sep = "  ·  "
	join := func(ps []headerPart) string {
		var ss []string
		for _, p := range ps {
			ss = append(ss, p.text)
		}
		return strings.Join(ss, mutedStyle.Render(sep))
	}
	info := join(parts)
	for ansi.StringWidth(info) > m.width {
		worst := -1
		for i, p := range parts {
			if p.drop > 0 && (worst < 0 || p.drop > parts[worst].drop) {
				worst = i
			}
		}
		if worst < 0 {
			break
		}
		parts = append(parts[:worst], parts[worst+1:]...)
		info = join(parts)
	}
	return ansi.Truncate(title, m.width, "…") + "\n" + ansi.Truncate(info, m.width, "…")
}

func (m Model) bodyView() string {
	th := m.treeHeight()
	showTree := !m.singlePane() || m.focus == focusTree
	showPreview := !m.singlePane() || m.focus == focusPreview

	var boxes []string
	if showTree {
		boxes = append(boxes, m.treeBox(th))
	}
	if showPreview {
		boxes = append(boxes, m.previewBox(th))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, boxes...)
}

func (m Model) treeBox(th int) string {
	inner := m.treeWidth() - 2
	var lines []string
	switch {
	case m.picker != nil:
		lines = m.pickerView(inner, th)
	default:
		end := min(m.offset+th, len(m.rows))
		for i := m.offset; i < end; i++ {
			top := true
			if p := tree.Parent(m.rows, i); p >= 0 && m.rows[p].Node.Kind == tree.KindItem {
				top = false
			}
			lines = append(lines, m.rowView(m.rows[i], i == m.cursor, inner, top))
		}
		if len(m.rows) == 0 {
			lines = append(lines, mutedStyle.Render("No matches."))
		}
	}
	return paneStyle(m.focus == focusTree).Width(inner).Height(th).Render(strings.Join(lines, "\n"))
}

func (m Model) previewBox(th int) string {
	pw := m.previewInnerWidth()
	mm := m
	head := []string{ansi.Truncate(mm.breadcrumb(pw), pw, "…"), ansi.Truncate(mm.statusLine(pw), pw, "…")}
	all := append(head, mm.metaLines()...)
	all = append(all, strings.Split(m.vp.View(), "\n")...)
	if len(all) > th {
		all = all[:th]
	}
	return paneStyle(m.focus == focusPreview).Width(pw).Height(th).Render(strings.Join(all, "\n"))
}

func (m Model) rowView(r tree.Row, selected bool, width int, top bool) string {
	n := r.Node
	indent := strings.Repeat("  ", r.Indent)
	var plain, styled string
	switch n.Kind {
	case tree.KindCategory, tree.KindScope, tree.KindGroup:
		arrow := "▶ "
		if r.Expanded {
			arrow = "▼ "
		}
		if !n.HasChildren() {
			arrow = "  "
		}
		plain = indent + arrow + n.Label
		label := n.Label
		if !n.HasChildren() {
			label += " — none found"
			plain += " — none found"
		}
		switch {
		case !n.HasChildren():
			styled = indent + arrow + mutedStyle.Render(label)
		case r.Indent == 0:
			styled = indent + arrow + categoryStyle.Render(label)
		case n.Kind == tree.KindScope:
			styled = indent + arrow + scopeStyle(n.Scope).Render(label)
		default:
			styled = indent + arrow + mutedStyle.Render(label)
		}
	case tree.KindDir:
		icon := "▸ "
		if r.Expanded {
			icon = "▾ "
		}
		if !n.HasChildren() {
			icon = "▫ "
		}
		badge := ""
		if n.Badge != "" {
			badge = " " + n.Badge
		}
		plain = indent + r.Prefix + icon + n.Label + badge
		lbl := dirStyle.Render(n.Label)
		if r.Match {
			lbl = matchStyle.Render(lbl)
		}
		styled = indent + mutedStyle.Render(r.Prefix) + dirStyle.Render(icon) + lbl + mutedStyle.Render(badge)
	default:
		it := n.Item
		icon := "• "
		if n.HasChildren() {
			icon = "▸ "
			if r.Expanded {
				icon = "▾ "
			}
		}
		agentsTag := ""
		if top && (m.view == viewInventory || m.view == viewFilesystem) && len(it.Agents) > 0 {
			agentsTag = " " + strings.Join(it.Agents, "·")
		}
		alias := ""
		if len(it.Aliases) > 0 && !strings.Contains(n.Badge, "⇆") {
			alias = fmt.Sprintf(" ⇆%d", len(it.Aliases))
		}
		badge := n.Badge
		label := n.Label
		switch {
		case it.Missing:
			icon = "✗ "
			badge = "missing"
		case it.Info != "":
			icon = "◇ "
		case n.Link:
			icon = "↪ "
		}
		if badge != "" {
			badge = " " + badge
		}
		plain = indent + r.Prefix + icon + label + agentsTag + alias + badge
		lbl := label
		if r.Match {
			lbl = matchStyle.Render(label)
		}
		if it.Missing {
			lbl = warnStyle.Render(label)
		}
		bs := mutedStyle
		if warnBadge(badge) {
			bs = warnStyle
		}
		styled = indent + mutedStyle.Render(r.Prefix) + scopeStyle(it.Scope).Render(icon) + lbl +
			mutedStyle.Render(agentsTag+alias) + bs.Render(badge)
	}
	if selected {
		return selectedStyle.Width(width).Render(ansi.Truncate(plain, width, "…"))
	}
	return ansi.Truncate(styled, width, "…")
}

func warnBadge(b string) bool {
	b = strings.ToLower(b)
	for _, w := range []string{"missing", "disabled", "shadowed", "overridden"} {
		if strings.Contains(b, w) {
			return true
		}
	}
	return false
}

func (m Model) footerView() string {
	if m.filtering {
		return ansi.Truncate(m.filter.View(), m.width, "…")
	}
	if m.finding {
		s := m.find.View()
		if st := m.findStatus(); st != "" {
			s += "  " + warnStyle.Render(st)
		}
		return ansi.Truncate(s, m.width, "…")
	}
	keys := [][2]string{{"↑↓", "move"}, {"←→", "fold"}, {"enter", "read"}, {"1-4", "view"}, {"a", "agent"},
		{"d", "dir"}, {"e", "explain"}, {"/", "filter"}, {"?", "help"}, {"q", "quit"}}
	if m.picker != nil {
		keys = [][2]string{{"↑↓", "choose"}, {"enter", "select"}, {"esc", "cancel"}}
	} else if m.focus == focusPreview {
		keys = [][2]string{{"↑↓", "scroll"}, {"[ ]", "links"}, {"enter", "open"}, {"esc", "back"}, {"/", "find"},
			{"n/N", "next"}, {"v", "raw"}, {"e", "explain"}, {"?", "help"}}
	}
	var parts []string
	if f := m.filter.Value(); f != "" && m.focus == focusTree {
		parts = append(parts, warnStyle.Render("filter: "+f)+mutedStyle.Render(" (esc clears)"))
	}
	if st := m.findStatus(); st != "" && m.focus == focusPreview {
		parts = append(parts, warnStyle.Render(st))
	}
	for _, k := range keys {
		parts = append(parts, keyStyle.Render(k[0])+" "+mutedStyle.Render(k[1]))
	}
	return ansi.Truncate(strings.Join(parts, "  "), m.width, "…")
}

// ---------------------------------------------------------------- text

const emptyMarkdown = "# Nothing selected\n\nNo context was discovered for this project."

const honesty = "\n\n---\n_AYCA shows what is configured on disk. It cannot verify what an agent actually loaded at runtime._"

var categoryHelp = map[discovery.Category]string{
	discovery.CategoryInstruction: "Instruction files such as `CLAUDE.md` and `AGENTS.md` give agents standing guidance: architecture notes, conventions, commands. Agents typically load them at session start (nested ones on demand). Files pulled in by `@path` imports in CLAUDE.md appear beneath the file that imports them.",
	discovery.CategoryMemory:      "Notes Claude Code keeps for this project (auto memory). `MEMORY.md` is loaded into every session; the topic files it points to are read when needed.",
	discovery.CategoryAgent:       "Subagent definitions. Their names and descriptions are in context so the main agent can delegate; each subagent's own prompt is used only when it runs.",
	discovery.CategoryCommand:     "Slash commands and custom prompts. Their text is sent to the agent only when you invoke one, though names and descriptions may be listed up front.",
	discovery.CategoryOutputStyle: "Output styles change Claude Code's system prompt. Only the active style (set by `outputStyle` in settings) applies.",
	discovery.CategoryHook:        "Hooks are commands that run on agent events. SessionStart and UserPromptSubmit hooks can add their output straight into the context; others can block actions or send feedback. AYCA lists them and the scripts they run. It never runs them.",
	discovery.CategoryRuntime:     "Context that exists in a session but has no file AYCA can read: built-in system prompts, the session environment, account-level connectors, launch flags and hook output. These entries explain what's there and where to look.",
	discovery.CategoryRule:        "Rules are scoped or conditional instructions: Cursor `.cursor/rules/*.mdc`, Claude Code `.claude/rules/*.md`, and Codex execution-policy `.rules` files.",
	discovery.CategorySkill:       "Skills are folders with a `SKILL.md`. Agents usually see only each skill's name and description up front and load the full content when the skill is used.",
	discovery.CategoryMCP:         "MCP servers give agents extra tools. Each entry is read from a configuration file; values that look like secrets are masked.",
}

func (m Model) groupMarkdown(n *tree.Node) string {
	var b strings.Builder
	switch n.Kind {
	case tree.KindCategory:
		cat := discovery.Category(strings.TrimPrefix(n.ID, "cat:"))
		fmt.Fprintf(&b, "# %s\n\n%s\n", n.Label, categoryHelp[cat])
		if !n.HasChildren() {
			b.WriteString("\nNothing of this kind was found for this project.\n")
		}
	case tree.KindScope:
		fmt.Fprintf(&b, "# %s\n\n", n.Label)
		switch n.Scope {
		case discovery.ScopeProject:
			fmt.Fprintf(&b, "Inside the project root `%s`, including nested subdirectories and project-specific entries kept in user config.\n", m.env.Tilde(m.env.ProjectRoot))
		case discovery.ScopeAncestor:
			b.WriteString("Directories above the project root. Some agents (e.g. Claude Code) read instruction files all the way up to `/`.\n")
		case discovery.ScopeGlobal:
			b.WriteString("User-wide configuration that applies to every project.\n")
		case discovery.ScopeSystem:
			b.WriteString("Machine-wide context: administrator-managed policy files that apply to every user and can't be overridden, plus context built into the agents themselves.\n")
		}
		b.WriteString("\n")
		for _, c := range n.Children {
			fmt.Fprintf(&b, "- `%s`", c.Label)
			if c.Item != nil && len(c.Item.Agents) > 0 {
				fmt.Fprintf(&b, " — %s", strings.Join(c.Item.Agents, ", "))
			}
			b.WriteString("\n")
		}
	}
	b.WriteString(honesty)
	return b.String()
}

// budgetSummary is the header's per-agent percentage, e.g.
// "claude 3.1% · codex 0.8%". Agents with nothing counted are left out.
func (m Model) budgetSummary() string {
	var parts []string
	for _, a := range m.budget.Agents {
		if a.Tokens == 0 {
			continue
		}
		parts = append(parts, a.Agent+" "+budgetStyle(a.Percent()).Render(fmt.Sprintf("%s%%", pct(a.Percent()))))
	}
	return strings.Join(parts, mutedStyle.Render(" · "))
}

func pct(p float64) string {
	if p > 0 && p < 0.1 {
		return "<0.1"
	}
	return fmt.Sprintf("%.1f", p)
}

func (m Model) budgetMarkdown() string {
	var b strings.Builder
	b.WriteString("# Startup context\n\nHow much of each agent's context window the context on disk fills before you type anything. Press `c` to return.\n")
	for _, a := range m.budget.Agents {
		fmt.Fprintf(&b, "\n## %s — ~%s tokens · %s%% of %s\n\n", a.Agent, budget.Human(a.Tokens), pct(a.Percent()), budget.Human(a.Window))
		if len(a.Entries) == 0 {
			b.WriteString("_Nothing on disk counts at startup._\n")
		} else {
			b.WriteString("| Category | Items | Tokens | Window |\n| --- | ---: | ---: | ---: |\n")
			for _, st := range a.ByCategory() {
				fmt.Fprintf(&b, "| %s | %d | %s | %s%% |\n", tree.CategoryLabel(st.Category), st.Count, budget.Human(st.Tokens),
					pct(float64(st.Tokens)*100/float64(max(a.Window, 1))))
			}
			b.WriteString("\n| Tokens | Item | Counted as |\n| ---: | --- | --- |\n")
			for _, e := range a.Entries {
				fmt.Fprintf(&b, "| %s | `%s` | %s |\n", budget.Human(e.Tokens), strings.ReplaceAll(e.Name, "|", "\\|"), e.How)
			}
		}
		for _, n := range a.Notes {
			fmt.Fprintf(&b, "\n%s\n", n)
		}
		if len(a.NotCounted) > 0 {
			b.WriteString("\n**Not counted:** " + strings.Join(a.NotCounted, "; ") + ".\n")
		}
	}
	b.WriteString(`
---
_Estimates: tokens ≈ bytes ÷ 4, and only what's on disk. Nested files, shadowed files and disabled plugins are left out because they don't load at session start. Skills, subagents and commands count only their name and description. The window sizes are typical defaults and depend on the model: set yours with ` + "`--window`" + `. For the real numbers, run ` + "`/context`" + ` inside Claude Code._
`)
	return b.String()
}

func (m Model) warningsMarkdown() string {
	var b strings.Builder
	b.WriteString("# Discovery warnings\n\nThese locations could not be read. Press `w` to return.\n\n")
	if len(m.base.Warnings) == 0 {
		b.WriteString("_None._\n")
	}
	for _, w := range m.base.Warnings {
		fmt.Fprintf(&b, "- `%s` — %s\n", m.env.Tilde(w.Path), w.Err)
	}
	return b.String()
}

func truncateLeft(s string, w int) string {
	if w <= 1 || ansi.StringWidth(s) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && ansi.StringWidth(string(r))+1 > w {
		r = r[1:]
	}
	return "…" + string(r)
}

func humanSize(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KiB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1024*1024))
	}
}

func humanCount(n int) string {
	if n < 1000 {
		return fmt.Sprint(n)
	}
	return fmt.Sprintf("%.1fk", float64(n)/1000)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
