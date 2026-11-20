package tui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/R037/ayca/internal/effective"
	"github.com/R037/ayca/internal/explain"
	"github.com/R037/ayca/internal/tree"
)

const (
	stripContent = 4
	stripHeight  = stripContent + 2 // with its border
	minStripRows = 26               // terminal height needed to show the strip
)

func (m Model) stripVisible() bool { return m.height >= minStripRows }

// explanation returns the cached explanation of the node for the selected agent.
func (m *Model) explanation(n *tree.Node) (explain.Explanation, bool) {
	chain := m.chainFor(n)
	if len(chain) == 0 {
		return explain.Explanation{}, false
	}
	key := fmt.Sprintf("%d|%s|%s", m.view, m.agent, n.ID)
	if e, ok := m.explCache[key]; ok {
		return e, true
	}
	e := explain.Explain(chain, m.agent, effective.ContextOf(m.viewResult()))
	m.explCache[key] = e
	return e, true
}

var (
	mdMarks  = regexp.MustCompile("[`*_]")
	sentence = regexp.MustCompile(`^(.+?[.!?])(\s|$)`)
)

// firstSentence returns the first sentence of s with Markdown marks removed.
func firstSentence(s string) string {
	s = strings.Join(strings.Fields(mdMarks.ReplaceAllString(s, "")), " ")
	if m := sentence.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return s
}

// stripLines are the four lines of the explain strip for the selection.
func (m *Model) stripLines(width int) []string {
	line := func(label, value string) string {
		return ansi.Truncate(labelStyle.Render(label)+value, width, "…")
	}
	blank := make([]string, stripContent)
	n := m.selected()
	if m.adhoc != nil {
		a := m.adhoc
		return []string{
			line("What", "Linked document · "+a.name),
			line("From", truncateLeft(m.base.Env.Tilde(a.content.SourcePath), width-9)),
			line("Loads", "Opened by following a link; it may not be part of any agent's context"),
			line("Related", mutedStyle.Render("AYCA never fetches URLs and only opens files in the skill and config folders")),
		}
	}
	if n == nil {
		return blank
	}
	if n.Item == nil && len(m.chainFor(n)) == 0 {
		what := n.Label
		if h := firstSentence(n.Help); h != "" {
			what += " — " + h
		}
		out := append([]string{line("What", what)}, blank[1:]...)
		return out
	}
	e, ok := m.explanation(n)
	if !ok {
		return blank
	}
	chain := m.chainFor(n)
	top, last := chain[0], chain[len(chain)-1]

	what := fmt.Sprintf("%s · %s · %s", e.Kind, top.Scope, firstSentence(e.What))

	var path, real string
	extra := 0
	for _, f := range e.Where {
		switch f.Label {
		case "Path", "Defined in":
			path = m.base.Env.Tilde(f.Value)
		case "Real path":
			real = m.base.Env.Tilde(f.Value)
		case "Also at":
			extra++
		}
	}
	tail := ""
	if real != "" {
		tail += " → " + real
	}
	if extra > 0 {
		tail += fmt.Sprintf(" · +%d other location%s", extra, plural(extra))
	}
	from := truncateLeft(path, max(width-9-ansi.StringWidth(tail), 12)) + tail

	loads := ""
	if len(e.Loads) > 0 {
		l := e.Loads[0]
		loads = fmt.Sprintf("%s: %s — %s", explain.AgentName(l.Agent), l.State.Label(), l.Reason)
		if t := m.startupTokens(last.ID); t > 0 {
			loads += fmt.Sprintf(" · ~%s tokens at startup", humanCount(t))
		}
	}

	var rel []string
	for _, r := range e.Related {
		rel = append(rel, strings.TrimSuffix(r, "."))
	}
	related := strings.Join(rel, " · ")
	if related == "" {
		related = mutedStyle.Render("none")
	}
	return []string{line("What", what), line("From", from), line("Loads", loads), line("Related", related)}
}

// startupTokens is the estimated startup cost of an item for the selected agent.
func (m *Model) startupTokens(id string) int {
	for _, a := range m.budget.Agents {
		if a.Agent != m.agent {
			continue
		}
		for _, en := range a.Entries {
			if en.ItemID == id {
				return en.Tokens
			}
		}
	}
	return 0
}

func (m Model) stripView() string {
	inner := m.width - 2
	lines := m.stripLines(inner)
	return paneStyle(false).Width(inner).Height(stripContent).Render(strings.Join(lines, "\n"))
}

// explainMarkdown is the full explanation panel for the selection.
func (m *Model) explainMarkdown() string {
	n := m.selected()
	if n == nil {
		return "# Nothing selected\n\nSelect an item to see how it is discovered and loaded."
	}
	if e, ok := m.explanation(n); ok {
		return e.Markdown() + "\n_Press `e` to return._\n"
	}
	return "# " + n.Label + "\n\nThis is a group. Select an item inside it to see how it is discovered and when it loads.\n\n" + n.Help + "\n\n_Press `e` to return._\n"
}

func panelTitle(p panel) string {
	switch p {
	case panelWarnings:
		return "Discovery warnings"
	case panelBudget:
		return "Startup context"
	case panelExplain:
		return "Explanation"
	case panelHelp:
		return "Keyboard help"
	}
	return ""
}

const helpMarkdown = `# Keyboard help

AYCA is read-only and offline. It never runs or fetches anything it finds.

## Anywhere

- ` + "`1` `2` `3` `4`" + ` — Hierarchy, Effective, Filesystem, Inventory view
- ` + "`a`" + ` — next agent (Hierarchy and Effective follow it; explanations use it)
- ` + "`d`" + ` — choose the working directory
- ` + "`e`" + ` — explain the selection in full
- ` + "`c`" + ` / ` + "`w`" + ` — startup context budget / discovery warnings
- ` + "`v`" + ` — raw or rendered document
- ` + "`r`" + ` — rescan
- ` + "`tab`" + ` — switch between tree and preview
- ` + "`?`" + ` — this help
- ` + "`q`" + ` / ` + "`ctrl+c`" + ` — quit

## Tree

- ` + "`↑` `↓` `j` `k`" + ` — move
- ` + "`←` `→` `h` `l`" + ` — collapse or expand; ` + "`←`" + ` on a collapsed row goes to its parent
- ` + "`space`" + ` — toggle a folder or group
- ` + "`enter`" + ` — read the item (focus the preview), or toggle a group
- ` + "`pgup` `pgdn` `ctrl+u` `ctrl+d`" + ` — page
- ` + "`g` `G` `home` `end`" + ` — first / last row
- ` + "`/`" + ` — filter by name or path (` + "`enter`" + ` keeps it, ` + "`esc`" + ` clears it)

## Preview

- ` + "`↑` `↓` `pgup` `pgdn` `g` `G`" + ` — scroll
- ` + "`]` `[`" + ` — next / previous link in the document
- ` + "`enter`" + ` — open the selected link
- ` + "`esc` `backspace`" + ` — go back after following links; ` + "`esc`" + ` with nothing to go back to returns to the tree
- ` + "`/`" + ` — find in the document; ` + "`n` `N`" + ` next / previous match, ` + "`esc`" + ` clears
- ` + "`←` `h`" + ` — back to the tree

## Working directory picker

- ` + "`↑` `↓`" + ` choose, ` + "`enter`" + ` select, ` + "`esc`" + ` cancel

Press ` + "`?`" + ` to close this panel.
`

// isHeader reports whether a node is a group-like header row.
func isHeader(n *tree.Node) bool {
	return n.Kind == tree.KindCategory || n.Kind == tree.KindScope || n.Kind == tree.KindGroup
}
