package tree

import (
	"strings"
	"testing"

	"ayca/internal/discovery"
)

func sampleItems() []discovery.Item {
	return []discovery.Item{
		{ID: "a", Name: "CLAUDE.md", Path: "/p/CLAUDE.md", Category: discovery.CategoryInstruction, Scope: discovery.ScopeProject},
		{ID: "g", Name: "~/.claude/CLAUDE.md", Path: "/h/.claude/CLAUDE.md", Category: discovery.CategoryInstruction, Scope: discovery.ScopeGlobal},
		{ID: "s", Name: "pdf", Path: "/h/.claude/skills/pdf", IsDir: true, Category: discovery.CategorySkill, Scope: discovery.ScopeGlobal,
			Children: []discovery.Item{
				{ID: "s/SKILL.md", Name: "SKILL.md", Path: "/h/.claude/skills/pdf/SKILL.md", Category: discovery.CategorySkill, Scope: discovery.ScopeGlobal},
				{ID: "s/ref.md", Name: "ref.md", Path: "/h/.claude/skills/pdf/ref.md", Category: discovery.CategorySkill, Scope: discovery.ScopeGlobal},
			}},
	}
}

func labels(rows []Row) []string {
	var out []string
	for _, r := range rows {
		out = append(out, strings.Repeat(" ", r.Depth)+r.Prefix+r.Node.Label)
	}
	return out
}

func TestBuildAndDefaultExpansion(t *testing.T) {
	roots := Build(sampleItems())
	if len(roots) != 4 {
		t.Fatalf("want 4 categories, got %d", len(roots))
	}
	got := strings.Join(labels(Visible(roots, NewState(), "")), "\n")
	want := strings.Join([]string{
		"Instructions (2)",
		" Project (1)",
		"  └─ CLAUDE.md",
		" Global (1)",
		"  └─ ~/.claude/CLAUDE.md",
		"Rules (0)",
		"Skills (1)",
		" Global (1)",
		"  └─ pdf", // item folders start collapsed
		"MCP Servers (0)",
	}, "\n")
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestExpandCollapse(t *testing.T) {
	roots := Build(sampleItems())
	st := NewState()
	rows := Visible(roots, st, "")
	pdf := rows[IndexOf(rows, "s")]
	st.Toggle(pdf.Node)
	rows = Visible(roots, st, "")
	i := IndexOf(rows, "s/SKILL.md")
	if i < 0 || rows[i].Prefix != "   ├─ " {
		t.Fatalf("children not shown correctly: %q", labels(rows))
	}
	if p := Parent(rows, i); rows[p].Node.ID != "s" {
		t.Fatalf("parent of SKILL.md = %s", rows[p].Node.ID)
	}
	// Collapsing a category hides everything below it.
	st.Set(roots[0], false)
	rows = Visible(roots, st, "")
	if IndexOf(rows, "a") >= 0 || !strings.HasPrefix(rows[1].Node.Label, "Rules") {
		t.Fatalf("collapse failed: %q", labels(rows))
	}
}

func TestFilter(t *testing.T) {
	roots := Build(sampleItems())
	st := NewState()
	// Matching a child forces its collapsed parent open.
	rows := Visible(roots, st, "REF")
	got := labels(rows)
	want := []string{"Skills (1)", " Global (1)", "  └─ pdf", "      └─ ref.md"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q want %q", got, want)
	}
	if !rows[3].Match || rows[2].Match {
		t.Fatal("match flags wrong")
	}
	// Paths match too.
	if rows := Visible(roots, st, "/h/.claude/CLAUDE"); IndexOf(rows, "g") < 0 || IndexOf(rows, "a") >= 0 {
		t.Fatalf("path filter: %q", labels(rows))
	}
	// Matching an item keeps all its children reachable when expanded.
	st.Set(roots[2].Children[0].Children[0], true)
	if rows := Visible(roots, st, "pdf"); IndexOf(rows, "s/ref.md") < 0 {
		t.Fatalf("children of match hidden: %q", labels(rows))
	}
	if rows := Visible(roots, st, "zzz"); len(rows) != 0 {
		t.Fatalf("no-match filter returned %d rows", len(rows))
	}
}
