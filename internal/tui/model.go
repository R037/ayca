// Package tui is the Bubble Tea interface: a context tree on the left and a
// file preview on the right.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"ayca/internal/budget"
	"ayca/internal/discovery"
	"ayca/internal/preview"
	"ayca/internal/tree"
)

type focus int

const (
	focusTree focus = iota
	focusPreview
)

type scanDoneMsg discovery.Result

// panel is a full-pane view that replaces the selected item's preview.
type panel string

const (
	panelNone     panel = ""
	panelWarnings panel = "warnings"
	panelBudget   panel = "budget"
)

// Model is the root Bubble Tea model.
type Model struct {
	env     discovery.Env
	agents  []string
	windows map[string]int
	budget  budget.Report

	res   discovery.Result
	roots []*tree.Node
	state *tree.State
	rows  []tree.Row

	cursor, offset int
	focus          focus

	filter    textinput.Model
	filtering bool

	raw      bool
	panel    panel
	scanning bool

	vp        viewport.Model
	renderer  *preview.Renderer
	contents  map[string]preview.Content
	rendered  map[string]string
	previewID string

	width, height int
}

// New creates the model from an initial discovery result. windows overrides
// the assumed context window size per agent (nil uses the defaults).
func New(res discovery.Result, agents []string, windows map[string]int, renderer *preview.Renderer) Model {
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.Placeholder = "filter by name or path"
	m := Model{
		env:      res.Env,
		agents:   agents,
		windows:  windows,
		state:    tree.NewState(),
		filter:   ti,
		vp:       viewport.New(0, 0),
		renderer: renderer,
	}
	m.setResult(res)
	return m
}

func (m *Model) setResult(res discovery.Result) {
	var keepID string
	if n := m.selected(); n != nil {
		keepID = n.ID
	}
	m.res = res
	m.budget = budget.Estimate(res, m.agents, m.windows)
	m.roots = tree.Build(res.Items)
	m.contents = map[string]preview.Content{}
	m.rendered = map[string]string{}
	m.previewID = ""
	m.refreshRows()
	if i := tree.IndexOf(m.rows, keepID); i >= 0 {
		m.cursor = i
	}
	m.clampCursor()
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

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

func (m Model) rescan() tea.Cmd {
	env, agents := m.env, m.agents
	return func() tea.Msg { return scanDoneMsg(discovery.Discover(env, agents)) }
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
		m.setResult(discovery.Result(msg))
		m.layout()
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.filtering {
			return m.updateFilter(msg)
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
		case "/":
			m.filtering = true
			m.focus = focusTree
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
			if r.Node.Kind == tree.KindItem {
				m.focus = focusPreview
			} else if r.Node.HasChildren() {
				m.state.Set(r.Node, !r.Expanded)
				m.refreshRows()
			}
		}
	case "esc":
		if m.filter.Value() != "" {
			m.filter.SetValue("")
			m.refreshRows()
		}
	}
	m.clampCursor()
	return m, nil
}

func (m Model) updatePreview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "left", "h":
		m.focus = focusTree
		return m, nil
	case "g", "home":
		m.vp.GotoTop()
		return m, nil
	case "G", "end":
		m.vp.GotoBottom()
		return m, nil
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
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

func (m Model) treeWidth() int {
	return max(28, min(m.width*2/5, 64))
}

func (m Model) bodyHeight() int { return max(m.height-headerLines-footerLines, 3) }
func (m Model) treeHeight() int { return m.bodyHeight() - 2 }

func (m Model) previewInnerWidth() int { return max(m.width-m.treeWidth()-2, 10) }

func (m *Model) layout() {
	m.vp.Width = m.previewInnerWidth()
	m.vp.Height = max(m.bodyHeight()-2-len(m.metaLines()), 1)
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

func (m *Model) syncPreview() {
	if m.width == 0 {
		return
	}
	id := "none"
	if m.panel != panelNone {
		id = "panel:" + string(m.panel)
	} else if n := m.selected(); n != nil {
		id = n.ID
	}
	if id == m.previewID {
		return
	}
	m.previewID = id
	m.vp.Height = max(m.bodyHeight()-2-len(m.metaLines()), 1)
	key := fmt.Sprintf("%s|%d|%v", id, m.vp.Width, m.raw)
	out, ok := m.rendered[key]
	if !ok {
		out = m.renderPreview()
		m.rendered[key] = out
	}
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
	switch m.panel {
	case panelWarnings:
		return m.renderer.Render(preview.Content{Text: m.warningsMarkdown(), Format: preview.FormatMarkdown}, w, false)
	case panelBudget:
		return m.renderer.Render(preview.Content{Text: m.budgetMarkdown(), Format: preview.FormatMarkdown}, w, false)
	}
	n := m.selected()
	if n == nil {
		return m.renderer.Render(preview.Content{Text: emptyMarkdown, Format: preview.FormatMarkdown}, w, false)
	}
	if n.Item == nil {
		return m.renderer.Render(preview.Content{Text: m.groupMarkdown(n), Format: preview.FormatMarkdown}, w, false)
	}
	return m.renderer.Render(m.content(n.Item), w, m.raw)
}

// metaLines describe the selected item: path, size, category, scope, agents.
func (m *Model) metaLines() []string {
	n := m.selected()
	if m.panel != panelNone || n == nil || n.Item == nil {
		return nil
	}
	it := n.Item
	c := m.content(it)
	w := m.previewInnerWidth()
	field := func(label, value string) string {
		return labelStyle.Render(label) + value
	}
	// Paths keep their tail (the file name) when they don't fit.
	pathField := func(label, p string) string {
		return field(label, truncateLeft(m.env.Tilde(p), w-9))
	}
	var lines []string
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

	var details []string
	if c.Size >= 0 {
		details = append(details, humanSize(c.Size))
	}
	if t := c.Tokens(); t > 0 {
		details = append(details, fmt.Sprintf("~%s tokens (est.)", humanCount(t)))
	}
	if it.Info == "" && !it.Missing {
		mode := "rendered"
		if m.raw {
			mode = "raw"
		}
		details = append(details, mode+" view")
		lines = append(lines, field("File", strings.Join(details, " · ")))
	}

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
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, w, "…")
	}
	lines = append(lines, mutedStyle.Render(strings.Repeat("─", w)))
	return lines
}

// ---------------------------------------------------------------- view

// View implements tea.Model.
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	return lipgloss.JoinVertical(lipgloss.Left, m.headerView(), m.bodyView(), m.footerView())
}

func (m Model) headerView() string {
	title := titleStyle.Render("AYCA — All Your Context Are Belong To Us") + "  " + taglineStyle.Render("Know what your agents know.")
	info := mutedStyle.Render("Project: ") + m.env.Tilde(m.env.ProjectRoot)
	if m.env.Cwd != m.env.ProjectRoot {
		info += mutedStyle.Render("  cwd: ") + m.env.Tilde(m.env.Cwd)
	}
	info += mutedStyle.Render(fmt.Sprintf("  ·  %d items", len(m.res.Items)))
	if s := m.budgetSummary(); s != "" {
		info += mutedStyle.Render("  ·  startup context on disk: ") + s + mutedStyle.Render(" (c)")
	}
	if len(m.agents) > 0 {
		info += mutedStyle.Render("  ·  agents: " + strings.Join(m.agents, ","))
	}
	if n := len(m.res.Warnings); n > 0 {
		info += "  " + warnStyle.Render(fmt.Sprintf("⚠ %d warning%s (w)", n, plural(n)))
	}
	if m.scanning {
		info += mutedStyle.Render("  ·  rescanning…")
	}
	return ansi.Truncate(title, m.width, "…") + "\n" + ansi.Truncate(info, m.width, "…")
}

func (m Model) bodyView() string {
	tw := m.treeWidth()
	th := m.treeHeight()
	inner := tw - 2

	var lines []string
	end := min(m.offset+th, len(m.rows))
	for i := m.offset; i < end; i++ {
		lines = append(lines, m.rowView(m.rows[i], i == m.cursor, inner))
	}
	if len(m.rows) == 0 {
		lines = append(lines, mutedStyle.Render("No matches."))
	}
	treeBox := paneStyle(m.focus == focusTree).Width(inner).Height(th).
		Render(strings.Join(lines, "\n"))

	pw := m.previewInnerWidth()
	meta := m.metaLines()
	content := strings.Join(append(meta, m.vp.View()), "\n")
	previewBox := paneStyle(m.focus == focusPreview).Width(pw).Height(th).
		Render(content)

	return lipgloss.JoinHorizontal(lipgloss.Top, treeBox, previewBox)
}

func (m Model) rowView(r tree.Row, selected bool, width int) string {
	n := r.Node
	indent := strings.Repeat("  ", min(r.Depth, 2))
	var plain, styled string
	switch n.Kind {
	case tree.KindCategory, tree.KindScope:
		arrow := "▶ "
		if r.Expanded {
			arrow = "▼ "
		}
		if !n.HasChildren() {
			arrow = "  "
		}
		plain = indent + arrow + n.Label
		if n.Kind == tree.KindCategory {
			styled = indent + arrow + categoryStyle.Render(n.Label)
			if !n.HasChildren() {
				styled = indent + arrow + mutedStyle.Render(n.Label+" — none found")
				plain += " — none found"
			}
		} else {
			styled = indent + arrow + scopeStyle(n.Scope).Render(n.Label)
		}
	default:
		it := n.Item
		icon := "• "
		if n.HasChildren() {
			icon = "▸ "
			if r.Expanded {
				icon = "▾ "
			}
		}
		badge := ""
		if r.Depth == 2 && len(it.Agents) > 0 {
			badge = " " + strings.Join(it.Agents, "·")
		}
		if len(it.Aliases) > 0 {
			badge += fmt.Sprintf(" ⇆%d", len(it.Aliases))
		}
		label := n.Label
		switch {
		case it.Missing:
			icon = "✗ "
			badge = " missing"
		case it.Info != "":
			icon = "◇ "
		}
		plain = indent + r.Prefix + icon + label + badge
		lbl := label
		if r.Match {
			lbl = matchStyle.Render(label)
		}
		if it.Missing {
			lbl = warnStyle.Render(label)
		}
		styled = indent + mutedStyle.Render(r.Prefix) + scopeStyle(it.Scope).Render(icon) + lbl + mutedStyle.Render(badge)
	}
	if selected {
		return selectedStyle.Width(width).Render(ansi.Truncate(plain, width, "…"))
	}
	return ansi.Truncate(styled, width, "…")
}

func (m Model) footerView() string {
	if m.filtering {
		return m.filter.View()
	}
	keys := [][2]string{{"↑↓/jk", "navigate"}, {"←→/hl", "collapse/expand"}, {"enter", "view"}, {"tab", "switch pane"},
		{"/", "search"}, {"v", "raw/rendered"}, {"c", "context budget"}, {"r", "rescan"}, {"q", "quit"}}
	if m.focus == focusPreview {
		keys = [][2]string{{"↑↓/jk", "scroll"}, {"pgup/pgdn", "page"}, {"g/G", "top/bottom"}, {"esc/tab", "back to tree"},
			{"v", "raw/rendered"}, {"q", "quit"}}
	}
	var parts []string
	if f := m.filter.Value(); f != "" {
		parts = append(parts, warnStyle.Render("filter: "+f)+mutedStyle.Render(" (esc clears)"))
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
	if len(m.res.Warnings) == 0 {
		b.WriteString("_None._\n")
	}
	for _, w := range m.res.Warnings {
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
