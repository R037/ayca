package tui

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/R037/ayca/internal/discovery"
	"github.com/R037/ayca/internal/effective"
	"github.com/R037/ayca/internal/explain"
	"github.com/R037/ayca/internal/tree"
)

// viewID is one perspective on the same discovered context.
type viewID int

const (
	viewHierarchy viewID = iota
	viewEffective
	viewFilesystem
	viewInventory
)

var viewNames = map[viewID]string{
	viewHierarchy:  "Hierarchy",
	viewEffective:  "Effective",
	viewFilesystem: "Filesystem",
	viewInventory:  "Inventory",
}

// ParseView maps a name such as "effective" to a view.
func parseView(name string) (viewID, bool) {
	for id, n := range viewNames {
		if strings.EqualFold(n, strings.TrimSpace(name)) {
			return id, true
		}
	}
	return 0, false
}

// ViewNames lists the accepted --view values in display order.
func ViewNames() []string {
	return []string{"hierarchy", "effective", "filesystem", "inventory"}
}

// perAgent reports whether the view's tree depends on the selected agent.
func (v viewID) perAgent() bool { return v == viewHierarchy || v == viewEffective }

// usesAt reports whether the view is built from the working-directory result.
func (v viewID) usesAt() bool { return v == viewHierarchy || v == viewEffective }

// viewMem remembers where the user was in one view.
type viewMem struct {
	state          *tree.State
	cursor, offset int
	selID          string
}

func (m Model) memKey() string {
	if m.view.perAgent() {
		return fmt.Sprintf("%d:%s", m.view, m.agent)
	}
	return fmt.Sprint(int(m.view))
}

func (m *Model) saveView() {
	id := ""
	if n := m.selected(); n != nil {
		id = n.ID
	}
	m.mem[m.memKey()] = viewMem{state: m.state, cursor: m.cursor, offset: m.offset, selID: id}
}

// switchTo changes view and/or agent, remembering the old position and
// restoring the one last used in the new view.
func (m *Model) switchTo(v viewID, agent string) {
	m.saveView()
	m.view, m.agent = v, agent
	m.adhoc = nil
	mem, ok := m.mem[m.memKey()]
	if !ok {
		mem = viewMem{state: tree.NewState()}
	}
	m.state, m.cursor, m.offset = mem.state, mem.cursor, mem.offset
	m.rebuild()
	m.refreshRows()
	if ok && mem.selID != "" {
		if i := tree.IndexOf(m.rows, mem.selID); i >= 0 {
			m.cursor = i
		}
	}
	m.previewID = ""
	m.clampCursor()
}

// viewResult is the discovery result the current view is built from.
func (m Model) viewResult() discovery.Result {
	if m.view.usesAt() {
		return m.at
	}
	return m.base
}

// rebuild builds the node tree of the current view.
func (m *Model) rebuild() {
	m.chains = nil
	switch m.view {
	case viewHierarchy:
		m.roots = tree.BuildHierarchy(m.at.Items, m.agent, m.at.Env)
	case viewEffective:
		m.roots, m.chains = m.buildEffective()
	case viewFilesystem:
		m.roots = tree.BuildFilesystem(m.base.Items, m.base.Env)
	default:
		m.roots = tree.Build(m.base.Items)
	}
}

// wdLabel is the working directory relative to the project root ("." for
// the root itself).
func (m Model) wdLabel() string {
	root := m.base.Env.ProjectRoot
	if rel, err := filepath.Rel(root, m.wd); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return m.base.Env.Tilde(m.wd)
}

// wdHeader is the working directory label for the header line.
func (m Model) wdHeader() string {
	if l := m.wdLabel(); l == "." {
		return ". (project root)"
	}
	return m.wdLabel()
}

// wdPhrase words the working directory for prose.
func (m Model) wdPhrase() string {
	if l := m.wdLabel(); l != "." {
		return "`" + strings.TrimSuffix(l, "/") + "/`"
	}
	return "the project root"
}

func stateBadge(s effective.State) string {
	switch s {
	case effective.Startup:
		return "startup"
	case effective.Listed:
		return "listed"
	case effective.OnDemand:
		return "on demand"
	case effective.Conditional:
		return "conditional"
	case effective.Invocation:
		return "invoked"
	case effective.Event:
		return "event"
	case effective.Enforced:
		return "enforced"
	case effective.Disabled:
		return "disabled"
	case effective.Shadowed:
		return "shadowed"
	case effective.NotLoaded:
		return "not loaded"
	}
	return "unknown"
}

// buildEffective projects the context onto the working directory and
// returns the tree plus the item chain behind every entry node.
func (m Model) buildEffective() ([]*tree.Node, map[string][]*discovery.Item) {
	full := m.at
	if m.fullReady {
		full = m.full
	}
	p := effective.Project(m.at, full, m.agent)
	env := m.at.Env
	agentName := explain.AgentName(m.agent)
	where := fmt.Sprintf("\n\nShown for **%s** starting in %s.", agentName, m.wdPhrase())
	chains := map[string][]*discovery.Item{}
	prefix := "e:" + m.agent + ":"

	entry := func(pre string, e effective.Entry) *tree.Node {
		last, top := e.Item(), e.Chain[0]
		var n *tree.Node
		if len(e.Chain) == 1 && top.Category != discovery.CategoryInstruction {
			n = tree.ItemNode(pre, last)
		} else {
			label := last.Name
			if len(e.Chain) > 1 {
				if last.Path != "" {
					label = env.DisplayPath(last.Path)
				}
				if top.Category == discovery.CategoryInstruction {
					label = "@" + label
				}
			}
			n = &tree.Node{ID: pre + last.ID, Label: label, Kind: tree.KindItem, Scope: last.Scope, Item: last}
		}
		chains[n.ID] = e.Chain
		return n
	}

	var roots []*tree.Node
	for _, g := range p.Groups {
		pre := prefix + string(g.Key) + ":"
		var kids []*tree.Node
		for _, e := range g.Entries {
			n := entry(pre, e)
			n.Badge = stateBadge(e.Load.State)
			kids = append(kids, n)
		}
		roots = append(roots, tree.NewGroup(prefix+"grp:"+string(g.Key),
			fmt.Sprintf("%s (%d)", g.Label, len(kids)), g.Help+where, kids...))
	}
	if len(p.Excluded) > 0 {
		pre := prefix + "excluded:"
		var kids []*tree.Node
		for _, e := range p.Excluded {
			n := entry(pre, e)
			n.Badge = "excluded"
			kids = append(kids, n)
		}
		roots = append(roots, tree.NewGroup(prefix+"grp:excluded",
			fmt.Sprintf("Excluded from this working directory (%d)", len(kids)),
			fmt.Sprintf("Found in the project, but not on the path from the project root to `%s`, so %s does not read these from there. Press `d` to choose a different working directory.", m.wdLabel(), agentName), kids...))
	}
	return roots, chains
}

// legendMarkdown explains the loading states used by the Effective view.
func legendMarkdown() string {
	var b strings.Builder
	b.WriteString("\n\n**How things load**\n\n")
	for _, s := range []effective.State{effective.Startup, effective.Listed, effective.Conditional, effective.OnDemand,
		effective.Invocation, effective.Event, effective.Enforced, effective.Shadowed, effective.Disabled, effective.Unknown} {
		fmt.Fprintf(&b, "- **%s** — %s\n", s.Label(), s.Describe())
	}
	return b.String()
}

func projectionFooter() string {
	return "\n\n---\n_This projection applies documented loading rules (" + effective.RulesBasis +
		"). It is not an observation of a running session._"
}

// chainFor returns the item chain (ancestors first) behind a node.
func (m Model) chainFor(n *tree.Node) []*discovery.Item {
	if n == nil {
		return nil
	}
	if c, ok := m.chains[n.ID]; ok {
		return c
	}
	var chain []*discovery.Item
	for _, a := range tree.Ancestors(m.roots, n.ID) {
		if a.Item == nil {
			continue
		}
		if len(chain) > 0 && isChildItem(chain[len(chain)-1], a.Item) {
			chain = append(chain, a.Item)
		} else {
			chain = []*discovery.Item{a.Item} // a new top-level item
		}
	}
	return chain
}

func isChildItem(parent, child *discovery.Item) bool {
	for i := range parent.Children {
		if &parent.Children[i] == child {
			return true
		}
	}
	return false
}

var countSuffix = regexp.MustCompile(`\s*\(\d+\)$`)

// crumbs are the labels from the root down to the node, without counts.
func (m Model) crumbs(n *tree.Node) []string {
	var out []string
	if n == nil {
		return out
	}
	for _, a := range tree.Ancestors(m.roots, n.ID) {
		out = append(out, strings.TrimSuffix(countSuffix.ReplaceAllString(a.Label, ""), "/"))
	}
	return out
}

// ---------------------------------------------------------------- agents

func (m Model) enabledAgents() []string {
	if len(m.agents) > 0 {
		return m.agents
	}
	return discovery.AllAgents
}

// defaultAgent is Claude Code when enabled and present, else the first
// enabled agent with anything on disk.
func defaultAgent(res discovery.Result, enabled []string) string {
	has := func(a string) bool {
		for i := range res.Items {
			it := &res.Items[i]
			if it.Category != discovery.CategoryRuntime && slices.Contains(it.Agents, a) {
				return true
			}
		}
		return false
	}
	if slices.Contains(enabled, discovery.AgentClaude) && has(discovery.AgentClaude) {
		return discovery.AgentClaude
	}
	for _, a := range enabled {
		if has(a) {
			return a
		}
	}
	if len(enabled) > 0 {
		return enabled[0]
	}
	return discovery.AgentClaude
}

func (m *Model) cycleAgent() {
	en := m.enabledAgents()
	if len(en) < 2 {
		return
	}
	i := slices.Index(en, m.agent)
	next := en[(i+1)%len(en)]
	if m.view.perAgent() {
		m.switchTo(m.view, next)
		return
	}
	m.agent = next
	m.previewID = ""
	m.explCache = map[string]explain.Explanation{}
	m.syncPreview()
}
