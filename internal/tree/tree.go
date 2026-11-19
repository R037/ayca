// Package tree turns discovered items into a navigable, filterable tree.
// It holds no terminal or rendering concerns beyond the line prefixes.
package tree

import (
	"fmt"
	"slices"
	"strings"

	"ayca/internal/discovery"
)

// Kind of tree node.
type Kind int

const (
	KindCategory Kind = iota
	KindScope
	KindItem
)

// Node is one line in the tree.
type Node struct {
	ID       string
	Label    string
	Kind     Kind
	Scope    discovery.Scope
	Item     *discovery.Item // nil for group nodes
	Children []*Node
}

// HasChildren reports whether the node can be expanded.
func (n *Node) HasChildren() bool { return len(n.Children) > 0 }

var categoryLabels = map[discovery.Category]string{
	discovery.CategoryInstruction: "Instructions",
	discovery.CategoryMemory:      "Memory",
	discovery.CategoryRule:        "Rules",
	discovery.CategorySkill:       "Skills",
	discovery.CategoryAgent:       "Subagents",
	discovery.CategoryCommand:     "Commands",
	discovery.CategoryOutputStyle: "Output styles",
	discovery.CategoryHook:        "Hooks",
	discovery.CategoryMCP:         "MCP Servers",
	discovery.CategoryRuntime:     "Runtime (not on disk)",
}

var scopeLabels = map[discovery.Scope]string{
	discovery.ScopeProject:  "Project",
	discovery.ScopeAncestor: "Ancestor directories",
	discovery.ScopeGlobal:   "Global",
	discovery.ScopeSystem:   "System",
}

// CategoryLabel returns the display label for a category.
func CategoryLabel(c discovery.Category) string { return categoryLabels[c] }

// ScopeLabel returns the display label for a scope.
func ScopeLabel(s discovery.Scope) string { return scopeLabels[s] }

// Build groups items as Category → Scope → Item, keeping discovery order
// inside each group. Empty core categories are kept so users can see that
// they were checked and nothing was found; other empty categories are hidden.
func Build(items []discovery.Item) []*Node {
	var roots []*Node
	for _, cat := range discovery.Categories {
		catNode := &Node{ID: "cat:" + string(cat), Kind: KindCategory}
		count := 0
		for _, sc := range discovery.Scopes {
			scNode := &Node{ID: catNode.ID + "/" + string(sc), Label: scopeLabels[sc], Kind: KindScope, Scope: sc}
			for i := range items {
				it := &items[i]
				if it.Category == cat && it.Scope == sc {
					scNode.Children = append(scNode.Children, itemNode(it))
					count++
				}
			}
			if len(scNode.Children) > 0 {
				scNode.Label = fmt.Sprintf("%s (%d)", scNode.Label, len(scNode.Children))
				catNode.Children = append(catNode.Children, scNode)
			}
		}
		if count == 0 && !slices.Contains(discovery.CoreCategories, cat) {
			continue
		}
		catNode.Label = fmt.Sprintf("%s (%d)", categoryLabels[cat], count)
		roots = append(roots, catNode)
	}
	return roots
}

func itemNode(it *discovery.Item) *Node {
	n := &Node{ID: it.ID, Label: it.Name, Kind: KindItem, Scope: it.Scope, Item: it}
	for i := range it.Children {
		n.Children = append(n.Children, itemNode(&it.Children[i]))
	}
	return n
}

// Row is a visible line: a node plus its drawing context.
type Row struct {
	Node     *Node
	Depth    int
	Prefix   string // tree-drawing characters before the node
	Expanded bool
	Match    bool // node itself matched the filter
}

// State tracks which nodes are expanded. Groups default to expanded; items
// with children (skill folders, rule folders) default to collapsed.
type State struct {
	overrides map[string]bool
}

// NewState returns an empty expansion state.
func NewState() *State { return &State{overrides: map[string]bool{}} }

// Expanded reports whether n is expanded.
func (s *State) Expanded(n *Node) bool {
	if v, ok := s.overrides[n.ID]; ok {
		return v
	}
	return n.Kind != KindItem
}

// Set expands or collapses n.
func (s *State) Set(n *Node, v bool) { s.overrides[n.ID] = v }

// Toggle flips n's expansion.
func (s *State) Toggle(n *Node) { s.Set(n, !s.Expanded(n)) }

// Visible flattens the tree into rows. With a non-empty filter, only nodes
// that match (by label or path, case-insensitively) and their ancestors are
// shown, and those ancestors are forced open; everything below a matching
// item stays visible subject to normal expansion.
func Visible(roots []*Node, st *State, filter string) []Row {
	filter = strings.ToLower(strings.TrimSpace(filter))
	var rows []Row
	var walk func(nodes []*Node, depth int, guides, filter string)
	walk = func(nodes []*Node, depth int, guides, filter string) {
		var shown []*Node
		for _, n := range nodes {
			if filter == "" || matches(n, filter) || anyDescendantMatches(n, filter) {
				shown = append(shown, n)
			}
		}
		for i, n := range shown {
			last := i == len(shown)-1
			prefix, childGuides := "", guides
			if depth >= 2 { // categories and scopes are drawn without branch lines
				branch := "├─ "
				if last {
					branch = "└─ "
				}
				prefix = guides + branch
				if last {
					childGuides = guides + "   "
				} else {
					childGuides = guides + "│  "
				}
			}
			selfMatch := filter != "" && matches(n, filter)
			expanded := st.Expanded(n)
			if filter != "" && !selfMatch && anyDescendantMatches(n, filter) {
				expanded = true
			}
			rows = append(rows, Row{Node: n, Depth: depth, Prefix: prefix, Expanded: expanded && n.HasChildren(), Match: selfMatch})
			if expanded && n.HasChildren() {
				// Below a matching item everything is shown (e.g. a skill's files).
				childFilter := filter
				if selfMatch {
					childFilter = ""
				}
				walk(n.Children, depth+1, childGuides, childFilter)
			}
		}
	}
	walk(roots, 0, "", filter)
	return rows
}

func matches(n *Node, f string) bool {
	if n.Kind != KindItem {
		return false
	}
	if strings.Contains(strings.ToLower(n.Label), f) {
		return true
	}
	return n.Item != nil && strings.Contains(strings.ToLower(n.Item.Path), f)
}

func anyDescendantMatches(n *Node, f string) bool {
	for _, c := range n.Children {
		if matches(c, f) || anyDescendantMatches(c, f) {
			return true
		}
	}
	return false
}

// Parent returns the index of the row's parent in rows, or -1.
func Parent(rows []Row, i int) int {
	if i < 0 || i >= len(rows) {
		return -1
	}
	d := rows[i].Depth
	for j := i - 1; j >= 0; j-- {
		if rows[j].Depth < d {
			return j
		}
	}
	return -1
}

// IndexOf returns the index of the row with the given node ID, or -1.
func IndexOf(rows []Row, id string) int {
	for i, r := range rows {
		if r.Node.ID == id {
			return i
		}
	}
	return -1
}
