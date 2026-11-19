package tree_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/R037/ayca/internal/discovery"
	"github.com/R037/ayca/internal/testfixture"
	"github.com/R037/ayca/internal/tree"
)

func world(t *testing.T) (*testfixture.Fixture, discovery.Env, []discovery.Item) {
	t.Helper()
	f := testfixture.New(t)
	env := f.Env(f.Project)
	res := discovery.Discover(env, nil)
	return f, env, res.Items
}

func walkAll(nodes []*tree.Node, fn func(n *tree.Node, anc []*tree.Node)) {
	var rec func(ns []*tree.Node, anc []*tree.Node)
	rec = func(ns []*tree.Node, anc []*tree.Node) {
		for _, n := range ns {
			fn(n, anc)
			rec(n.Children, append(append([]*tree.Node(nil), anc...), n))
		}
	}
	rec(nodes, nil)
}

func assertUnique(t *testing.T, name string, roots []*tree.Node) {
	t.Helper()
	seen := map[string]bool{}
	walkAll(roots, func(n *tree.Node, _ []*tree.Node) {
		if seen[n.ID] {
			t.Errorf("%s: duplicate node ID %q", name, n.ID)
		}
		seen[n.ID] = true
	})
}

func find(roots []*tree.Node, pred func(n *tree.Node) bool) (*tree.Node, []*tree.Node) {
	var got *tree.Node
	var ancs []*tree.Node
	walkAll(roots, func(n *tree.Node, anc []*tree.Node) {
		if got == nil && pred(n) {
			got, ancs = n, anc
		}
	})
	return got, ancs
}

func ancLabels(anc []*tree.Node) string {
	var s []string
	for _, a := range anc {
		s = append(s, a.Label)
	}
	return strings.Join(s, " > ")
}

func itemAt(path string) func(n *tree.Node) bool {
	return func(n *tree.Node) bool {
		return n.Item != nil && !n.Link && (n.Item.Path == path || n.Item.Target == path)
	}
}

func topLabels(roots []*tree.Node) []string {
	var out []string
	for _, r := range roots {
		out = append(out, r.Label)
	}
	return out
}

func TestHierarchyGroupsAndPlacement(t *testing.T) {
	f, env, items := world(t)

	claude := tree.BuildHierarchy(items, discovery.AgentClaude, env)
	assertUnique(t, "claude", claude)
	var heads []string
	for _, r := range claude {
		heads = append(heads, strings.SplitN(r.Label, " (", 2)[0])
		if r.Kind != tree.KindGroup || r.Help == "" {
			t.Errorf("group %q: kind %v help empty=%v", r.Label, r.Kind, r.Help == "")
		}
	}
	want := []string{"Managed policy", "Global context", "Project context", "Parent directories", "Nested / conditional context", "Plugins", "Hooks", "MCP servers", "Runtime context"}
	if strings.Join(heads, "|") != strings.Join(want, "|") {
		t.Fatalf("claude groups = %v", topLabels(claude))
	}
	if !strings.Contains(claude[1].Label, "~/.claude") {
		t.Errorf("global label %q", claude[1].Label)
	}

	n, anc := find(claude, itemAt(f.P("services/api/CLAUDE.md")))
	if n == nil || !strings.HasPrefix(anc[0].Label, "Nested") {
		t.Fatalf("services/api/CLAUDE.md placement: %v", ancLabels(anc))
	}
	n, anc = find(claude, itemAt(f.H(".agents/skills/pdf")))
	if n == nil || !strings.HasPrefix(anc[0].Label, "Global") {
		t.Fatalf("pdf for claude: %v", ancLabels(anc))
	}
	if n, _ := find(claude, itemAt(f.H(".cursor/skills/pdf"))); n != nil {
		t.Error("cursor pdf must not appear for claude")
	}
	// Plugin items only under Plugins, marked disabled.
	walkAll(claude, func(n *tree.Node, anc []*tree.Node) {
		if n.Item != nil && n.Item.Provider() != "" && !strings.HasPrefix(anc[0].Label, "Plugins") {
			t.Errorf("plugin item %s outside Plugins: %s", n.Item.Name, ancLabels(anc))
		}
	})
	_, anc = find(claude, func(n *tree.Node) bool { return n.Item != nil && n.Item.Name == "demo-skill" })
	if anc == nil || !strings.Contains(anc[len(anc)-1].Label, "(disabled)") {
		t.Errorf("plugin skill ancestors: %s", ancLabels(anc))
	}
	hook, anc := find(claude, func(n *tree.Node) bool {
		return n.Item != nil && n.Item.Category == discovery.CategoryHook && n.Item.Provider() != ""
	})
	if hook == nil || !strings.HasPrefix(anc[0].Label, "Plugins") {
		t.Errorf("plugin hook: %v", ancLabels(anc))
	}
	// Skills are browsable folders.
	pdf, _ := find(claude, itemAt(f.H(".agents/skills/pdf")))
	if len(pdf.Children) == 0 {
		t.Error("pdf skill has no children")
	}
	if pdf.ID != "h:claude:"+pdf.Item.ID {
		t.Errorf("id %q", pdf.ID)
	}

	codex := tree.BuildHierarchy(items, discovery.AgentCodex, env)
	assertUnique(t, "codex", codex)
	if n, _ := find(codex, itemAt(f.P("AGENTS.md"))); n == nil {
		t.Error("codex missing project AGENTS.md")
	}
	if n, anc := find(codex, itemAt(f.H(".agents/skills/pdf"))); n == nil || !strings.HasPrefix(anc[0].Label, "Global") {
		t.Error("codex pdf not global")
	}
	if g, _ := find(codex, func(n *tree.Node) bool { return strings.Contains(n.Label, "not read by Codex") }); g == nil {
		t.Error("codex parent group label")
	}
	if !strings.Contains(codex[0].Label, "~/.codex, ~/.agents") {
		t.Errorf("codex global label %q", codex[1].Label)
	}

	cursor := tree.BuildHierarchy(items, discovery.AgentCursor, env)
	assertUnique(t, "cursor", cursor)
	if n, _ := find(cursor, itemAt(f.P("AGENTS.md"))); n == nil {
		t.Error("cursor missing project AGENTS.md")
	}
	if n, anc := find(cursor, itemAt(f.H(".cursor/skills/pdf"))); n == nil || !strings.HasPrefix(anc[0].Label, "Global") {
		t.Error("cursor pdf not under Global")
	}
	if n, _ := find(cursor, itemAt(f.H(".agents/skills/pdf"))); n != nil {
		t.Error("shared pdf must not appear for cursor")
	}
}

func TestFilesystemView(t *testing.T) {
	f, env, items := world(t)
	roots := tree.BuildFilesystem(items, env)
	assertUnique(t, "fs", roots)

	phys, anc := find(roots, itemAt(f.H(".agents/skills/pdf")))
	if phys == nil || !strings.HasPrefix(anc[0].Label, "Home") {
		t.Fatalf("physical pdf: %v", ancLabels(anc))
	}
	if !strings.Contains(phys.Badge, "⇆") || !strings.Contains(phys.Badge, "2") {
		t.Errorf("physical badge %q", phys.Badge)
	}
	if len(phys.Children) == 0 {
		t.Error("physical pdf has no children")
	}
	link, anc := find(roots, func(n *tree.Node) bool { return n.Link && n.ID == "fs:link:"+f.H(".claude/skills/pdf") })
	if link == nil || !strings.Contains(link.Badge, "→") || !strings.Contains(link.Badge, "~/.agents/skills/pdf") || link.Item != phys.Item || len(link.Children) != 0 {
		t.Fatalf("link node: %+v (%s)", link, ancLabels(anc))
	}
	cur, _ := find(roots, itemAt(f.H(".cursor/skills/pdf")))
	if cur == nil || cur == phys || cur.Badge != "" {
		t.Errorf("cursor pdf: %+v", cur)
	}
	// hooks and MCP under a config-file dir
	for _, tc := range []struct{ file, name string }{
		{f.P(".mcp.json"), "filesystem"}, {f.H(".claude/settings.json"), ""},
	} {
		d, _ := find(roots, func(n *tree.Node) bool { return n.Kind == tree.KindDir && n.Path == tc.file })
		if d == nil || d.Badge != "config file" || len(d.Children) == 0 {
			t.Errorf("config dir %s: %+v", tc.file, d)
		}
	}
	// compressed chain
	d, _ := find(roots, func(n *tree.Node) bool { return n.Kind == tree.KindDir && n.Label == "services/" })
	if d == nil {
		t.Error("no services/ dir")
	}
	if g := roots[0]; !strings.HasPrefix(g.Label, "Project") || g.Kind != tree.KindGroup {
		t.Errorf("first root %q", g.Label)
	}
	// help for known dir
	d, _ = find(roots, func(n *tree.Node) bool { return n.Kind == tree.KindDir && n.Path == f.H(".claude") })
	if d == nil || !strings.Contains(d.Help, "Claude Code user config") {
		t.Errorf("~/.claude help: %+v", d)
	}
}

func TestBuildUniqueIDs(t *testing.T) {
	_, _, items := world(t)
	assertUnique(t, "inventory", tree.Build(items))
}

func TestIndentAndPrefixes(t *testing.T) {
	_, env, items := world(t)
	for _, r := range tree.Visible(tree.Build(items), tree.NewState(), "") {
		switch r.Node.Kind {
		case tree.KindCategory:
			if r.Indent != 0 || r.Prefix != "" {
				t.Fatalf("category row %+v", r)
			}
		case tree.KindScope:
			if r.Indent != 1 || r.Prefix != "" {
				t.Fatalf("scope row %+v", r)
			}
		default:
			if r.Indent != 2 || r.Prefix == "" {
				t.Fatalf("item row %q %+v", r.Node.Label, r)
			}
		}
	}
	for _, r := range tree.Visible(tree.BuildHierarchy(items, discovery.AgentClaude, env), tree.NewState(), "") {
		if r.Node.Kind == tree.KindGroup && r.Prefix != "" {
			t.Fatalf("group with prefix: %q", r.Node.Label)
		}
		if r.Node.Kind == tree.KindItem && r.Prefix == "" {
			t.Fatalf("item without prefix: %q", r.Node.Label)
		}
	}
}

func TestAncestorsFindPathExpandTo(t *testing.T) {
	f, env, items := world(t)
	roots := tree.BuildFilesystem(items, env)
	id := "fs:" + func() string { n, _ := find(roots, itemAt(f.H(".agents/skills/pdf"))); return n.Item.ID }()
	anc := tree.Ancestors(roots, id)
	if len(anc) < 2 || anc[len(anc)-1].ID != id || anc[0].Kind != tree.KindGroup {
		t.Fatalf("ancestors: %v", ancLabels(anc))
	}
	if tree.Ancestors(roots, "nope") != nil {
		t.Error("expected nil")
	}
	if got := tree.FindPath(roots, f.H(".agents/skills/pdf")); got != id {
		t.Errorf("FindPath physical = %q want %q", got, id)
	}
	// Files inside a symlinked directory are found from either side.
	for name, r := range map[string][]*tree.Node{"filesystem": roots, "hierarchy": tree.BuildHierarchy(items, "claude", env), "inventory": tree.Build(items)} {
		a := tree.FindPath(r, f.H(".claude/skills/pdf/SKILL.md"))
		b := tree.FindPath(r, f.H(".agents/skills/pdf/SKILL.md"))
		if a == "" || a != b {
			t.Errorf("%s: SKILL.md via symlink = %q, via real path = %q", name, a, b)
		}
	}
	if got := tree.FindPath(roots, f.H(".claude/skills/pdf")); got != id {
		t.Errorf("FindPath via symlink = %q want %q (non-link preferred)", got, id)
	}
	if tree.FindPath(roots, filepath.Join(f.Root, "nothing")) != "" {
		t.Error("FindPath should miss")
	}

	st := tree.NewState()
	for _, r := range roots {
		st.Set(r, false)
	}
	st.ExpandTo(roots, id)
	if tree.IndexOf(tree.Visible(roots, st, ""), id) < 0 {
		t.Error("node not visible after ExpandTo")
	}
	_ = os.Stdout
}
