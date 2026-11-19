// Package tree turns discovered items into a navigable, filterable tree.
// It holds no terminal or rendering concerns beyond the line prefixes.
package tree

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/R037/ayca/internal/discovery"
)

// Kind of tree node.
type Kind int

const (
	KindCategory Kind = iota
	KindScope
	KindItem
	// KindGroup is a generic header at any depth; it never gets branch lines.
	KindGroup
	// KindDir is a filesystem directory or config-file container that is
	// not itself an item; it is drawn like an item, with branch lines.
	KindDir
)

// Node is one line in the tree.
type Node struct {
	ID       string
	Label    string
	Kind     Kind
	Scope    discovery.Scope
	Item     *discovery.Item // nil for group nodes
	Children []*Node
	// Help is Markdown shown in the preview when a group or dir node is selected.
	Help string
	// Badge is a short suffix shown after the label.
	Badge string
	// Path is the absolute path a KindDir node stands for.
	Path string
	// Link marks a symlink/alias appearance of Item rather than its physical location.
	Link bool
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

func itemNode(it *discovery.Item) *Node { return ItemNode("", it) }

// ItemNode builds the node of an item and its children. Every node ID is
// prefix + the item's ID, so one item can appear in several views.
func ItemNode(prefix string, it *discovery.Item) *Node {
	n := &Node{ID: prefix + it.ID, Label: it.Name, Kind: KindItem, Scope: it.Scope, Item: it}
	for i := range it.Children {
		n.Children = append(n.Children, ItemNode(prefix, &it.Children[i]))
	}
	return n
}

// NewGroup builds a generic group node.
func NewGroup(id, label, help string, children ...*Node) *Node {
	return &Node{ID: id, Label: label, Kind: KindGroup, Help: help, Children: children}
}

// isGroupKind reports whether the kind is drawn as a header without branches.
func isGroupKind(k Kind) bool { return k == KindCategory || k == KindScope || k == KindGroup }

// Ancestors returns the path from a root to the node with the given ID
// (inclusive), or nil when absent.
func Ancestors(roots []*Node, id string) []*Node {
	for _, r := range roots {
		if r.ID == id {
			return []*Node{r}
		}
		if p := Ancestors(r.Children, id); p != nil {
			return append([]*Node{r}, p...)
		}
	}
	return nil
}

// ExpandTo expands every ancestor of the node so that it becomes visible.
func (s *State) ExpandTo(roots []*Node, id string) {
	p := Ancestors(roots, id)
	for i := 0; i+1 < len(p); i++ {
		s.Set(p[i], true)
	}
}

// FindPath returns the ID of the first node whose item path, target or alias
// equals absPath (also compared after resolving symlinks), preferring
// non-link nodes. It returns "" when nothing matches.
func FindPath(roots []*Node, absPath string) string {
	if absPath == "" {
		return ""
	}
	want := filepath.Clean(absPath)
	wantReal := evalOr(want)
	cache := map[string]string{}
	eq := func(p string) bool {
		if p == "" {
			return false
		}
		p = filepath.Clean(p)
		if p == want {
			return true
		}
		r, ok := cache[p]
		if !ok {
			r = evalOr(p)
			cache[p] = r
		}
		return r == wantReal
	}
	itemMatch := func(it *discovery.Item) bool {
		if eq(it.Path) || eq(it.Target) {
			return true
		}
		for _, a := range it.Aliases {
			if eq(a) {
				return true
			}
		}
		return false
	}
	exactPath := func(it *discovery.Item) bool { return it.Path != "" && filepath.Clean(it.Path) == want }
	exactAlias := func(it *discovery.Item) bool {
		if it.Target != "" && filepath.Clean(it.Target) == want {
			return true
		}
		for _, a := range it.Aliases {
			if filepath.Clean(a) == want {
				return true
			}
		}
		return false
	}
	var walk func(nodes []*Node, pass int) string
	walk = func(nodes []*Node, pass int) string {
		for _, n := range nodes {
			switch {
			case pass == 0 && n.Item != nil && exactPath(n.Item):
				return n.ID
			case pass == 1 && n.Item != nil && exactAlias(n.Item):
				return n.ID
			case pass == 2 && n.Item != nil && !n.Link && itemMatch(n.Item):
				return n.ID
			case pass == 3 && n.Item != nil && n.Link && itemMatch(n.Item):
				return n.ID
			case pass == 4 && n.Kind == KindDir && eq(n.Path):
				return n.ID
			}
			if id := walk(n.Children, pass); id != "" {
				return id
			}
		}
		return ""
	}
	for pass := 0; pass < 5; pass++ {
		if id := walk(roots, pass); id != "" {
			return id
		}
	}
	return ""
}

func evalOr(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// Row is a visible line: a node plus its drawing context.
type Row struct {
	Node     *Node
	Depth    int
	Prefix   string // tree-drawing characters before the node
	Expanded bool
	Match    bool // node itself matched the filter
	// Indent is the number of group-kind (category, scope, group) ancestors.
	Indent int
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
	var walk func(nodes []*Node, depth, indent int, guides, filter string)
	walk = func(nodes []*Node, depth, indent int, guides, filter string) {
		var shown []*Node
		for _, n := range nodes {
			if filter == "" || matches(n, filter) || anyDescendantMatches(n, filter) {
				shown = append(shown, n)
			}
		}
		for i, n := range shown {
			last := i == len(shown)-1
			prefix, childGuides := "", guides
			childIndent := indent
			if isGroupKind(n.Kind) {
				childIndent++
			} else { // only item and dir nodes are drawn with branch lines
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
			rows = append(rows, Row{Node: n, Depth: depth, Prefix: prefix, Expanded: expanded && n.HasChildren(), Match: selfMatch, Indent: indent})
			if expanded && n.HasChildren() {
				// Below a matching item everything is shown (e.g. a skill's files).
				childFilter := filter
				if selfMatch {
					childFilter = ""
				}
				walk(n.Children, depth+1, childIndent, childGuides, childFilter)
			}
		}
	}
	walk(roots, 0, 0, "", filter)
	return rows
}

func matches(n *Node, f string) bool {
	if n.Kind != KindItem && n.Kind != KindDir {
		return false
	}
	if strings.Contains(strings.ToLower(n.Label), f) {
		return true
	}
	if n.Kind == KindDir {
		return strings.Contains(strings.ToLower(n.Path), f)
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
