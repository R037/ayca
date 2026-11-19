package explain

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/R037/ayca/internal/discovery"
	"github.com/R037/ayca/internal/effective"
	"github.com/R037/ayca/internal/testfixture"
)

func chainWhere(items []discovery.Item, pred func(c []*discovery.Item) bool) []*discovery.Item {
	var walk func(prefix []*discovery.Item, items []discovery.Item) []*discovery.Item
	walk = func(prefix []*discovery.Item, items []discovery.Item) []*discovery.Item {
		for i := range items {
			c := append(append([]*discovery.Item{}, prefix...), &items[i])
			if pred(c) {
				return c
			}
			if r := walk(c, items[i].Children); r != nil {
				return r
			}
		}
		return nil
	}
	return walk(nil, items)
}

func byPath(t *testing.T, res discovery.Result, path string) []*discovery.Item {
	t.Helper()
	c := chainWhere(res.Items, func(c []*discovery.Item) bool { return c[len(c)-1].Path == path })
	if c == nil {
		t.Fatalf("no item at %s", path)
	}
	return c
}

func byName(t *testing.T, res discovery.Result, name string) []*discovery.Item {
	t.Helper()
	c := chainWhere(res.Items, func(c []*discovery.Item) bool { return c[len(c)-1].Name == name })
	if c == nil {
		t.Fatalf("no item named %s", name)
	}
	return c
}

func mustContain(t *testing.T, what, s string, subs ...string) {
	t.Helper()
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			t.Errorf("%s lacks %q:\n%s", what, sub, s)
		}
	}
}

func fieldValues(e Explanation, label string) []string {
	var out []string
	for _, f := range e.Where {
		if f.Label == label {
			out = append(out, f.Value)
		}
	}
	return out
}

func TestExplainFixture(t *testing.T) {
	fx := testfixture.New(t)
	res := discovery.Discover(fx.Env(fx.Project), nil)
	ctx := effective.ContextOf(res)
	p, h := fx.P, fx.H
	ex := func(c []*discovery.Item, agent string) Explanation { return Explain(c, agent, ctx) }

	t.Run("skill", func(t *testing.T) {
		e := ex(byPath(t, res, h(".claude/skills/frontend-design")), "claude")
		if e.Kind != "Skill" || e.Title != "frontend-design" {
			t.Errorf("kind/title = %q / %q", e.Kind, e.Title)
		}
		mustContain(t, "what", e.What, "Build distinctive, production-grade frontend interfaces")
		mustContain(t, "discovery", e.Discovery, "~/.claude/skills")
		mustContain(t, "related", strings.Join(e.Related, "\n"), "supporting files", "references/patterns.md", "references/missing.md")
		if e.Loads[0].Agent != "claude" || e.Loads[0].State != effective.Listed {
			t.Errorf("loads = %+v", e.Loads)
		}
	})
	t.Run("skill script", func(t *testing.T) {
		sk := h(".claude/skills/frontend-design")
		c := chainWhere(res.Items, func(c []*discovery.Item) bool { return c[len(c)-1].Path == sk+"/scripts/validate.sh" })
		e := ex(c, "claude")
		if e.Kind != "Skill script" {
			t.Errorf("kind = %q", e.Kind)
		}
		mustContain(t, "what", e.What, "never does")
		mustContain(t, "part of", fieldValues(e, "Part of")[0], "frontend-design")
	})
	t.Run("skill supporting file", func(t *testing.T) {
		sk := h(".claude/skills/frontend-design")
		c := chainWhere(res.Items, func(c []*discovery.Item) bool { return c[len(c)-1].Path == sk+"/references/patterns.md" })
		e := ex(c, "claude")
		if e.Kind != "Skill supporting file" {
			t.Errorf("kind = %q", e.Kind)
		}
	})
	t.Run("pdf symlink", func(t *testing.T) {
		e := ex(byName(t, res, "pdf"), "claude")
		real := fieldValues(e, "Real path")
		if len(real) != 1 || real[0] != h(".agents/skills/pdf") {
			t.Errorf("real path = %v", real)
		}
		mustContain(t, "related", strings.Join(e.Related, "\n"), "symlink", "Codex")
		if len(e.Loads) < 2 || e.Loads[1].Agent != "codex" {
			t.Errorf("loads = %+v", e.Loads)
		}
		// Cursor does not read the shared pdf skill.
		c := ex(byName(t, res, "pdf"), "cursor")
		mustContain(t, "discovery", c.Discovery, "Cursor does not read this location", "Claude Code")
		if c.Loads[0].State != effective.NotLoaded {
			t.Errorf("cursor load = %+v", c.Loads[0])
		}
	})
	t.Run("disabled plugin", func(t *testing.T) {
		e := ex(byName(t, res, "demo-skill"), "claude")
		mustContain(t, "related", strings.Join(e.Related, "\n"), "plugin demo", "disabled")
		mustContain(t, "provider", strings.Join(fieldValues(e, "Provider"), ""), "plugin demo")
		if e.Loads[0].State != effective.Disabled {
			t.Errorf("load = %+v", e.Loads[0])
		}
	})
	t.Run("missing import", func(t *testing.T) {
		c := chainWhere(res.Items, func(c []*discovery.Item) bool { return c[len(c)-1].Missing })
		e := ex(c, "claude")
		if e.Kind != "@import" {
			t.Errorf("kind = %q", e.Kind)
		}
		if e.Loads[0].State != effective.NotLoaded {
			t.Errorf("load = %+v", e.Loads[0])
		}
		mustContain(t, "discovery", e.Discovery, "@path")
		parent := ex(byPath(t, res, p("CLAUDE.md")), "claude")
		mustContain(t, "related", strings.Join(parent.Related, "\n"), "Missing", "@docs/missing.md", "@docs/architecture.md")
	})
	t.Run("import", func(t *testing.T) {
		c := chainWhere(res.Items, func(c []*discovery.Item) bool { return len(c) == 2 && c[1].Path == p("docs/architecture.md") })
		e := ex(c, "claude")
		mustContain(t, "what", e.What, "CLAUDE.md")
		mustContain(t, "load", e.Loads[0].Reason, "imported into CLAUDE.md")
	})
	t.Run("shadowed", func(t *testing.T) {
		e := ex(byPath(t, res, p("AGENTS.md")), "codex")
		mustContain(t, "related", strings.Join(e.Related, "\n"), "Shadowed by AGENTS.override.md")
		if e.Loads[0].State != effective.Shadowed {
			t.Errorf("load = %+v", e.Loads[0])
		}
		// Cursor reads the same file without any shadowing.
		cu := ex(byPath(t, res, p("AGENTS.md")), "cursor")
		if cu.Loads[0].State != effective.Startup {
			t.Errorf("cursor load = %+v", cu.Loads[0])
		}
		ov := ex(byPath(t, res, p("AGENTS.override.md")), "codex")
		mustContain(t, "related", strings.Join(ov.Related, "\n"), "Shadows AGENTS.md")
	})
	t.Run("conditional rule", func(t *testing.T) {
		e := ex(byPath(t, res, p(".claude/rules/backend.md")), "claude")
		if e.Kind != "Rule (conditional)" {
			t.Errorf("kind = %q", e.Kind)
		}
		mustContain(t, "load", e.Loads[0].Reason, "services/**")
	})
	t.Run("hook", func(t *testing.T) {
		c := chainWhere(res.Items, func(c []*discovery.Item) bool {
			return len(c) == 1 && c[0].Category == discovery.CategoryHook && strings.HasPrefix(c[0].Name, "SessionStart")
		})
		e := ex(c, "claude")
		if e.Kind != "Hook" {
			t.Errorf("kind = %q", e.Kind)
		}
		mustContain(t, "what", e.What, "SessionStart", "added to the agent's context")
		mustContain(t, "caveats", strings.Join(e.Caveats, "\n"), "never runs hooks")
	})
	t.Run("mcp", func(t *testing.T) {
		e := ex(byName(t, res, "filesystem"), "claude")
		if e.Kind != "MCP server" {
			t.Errorf("kind = %q", e.Kind)
		}
		mustContain(t, "discovery", e.Discovery, ".mcp.json")
		mustContain(t, "caveats", strings.Join(e.Caveats, "\n"), effective.RulesBasis)
	})
	t.Run("runtime", func(t *testing.T) {
		e := ex(byName(t, res, "Claude Code system prompt"), "claude")
		if e.Kind != "Runtime context" {
			t.Errorf("kind = %q", e.Kind)
		}
	})
	t.Run("nested codex has version caveat", func(t *testing.T) {
		e := ex(byPath(t, res, p("services/api/AGENTS.md")), "codex")
		if e.Kind != "Instruction file (nested)" {
			t.Errorf("kind = %q", e.Kind)
		}
		mustContain(t, "caveats", strings.Join(e.Caveats, "\n"), effective.RulesBasis, "version-dependent")
	})
	t.Run("subagent description", func(t *testing.T) {
		e := ex(byPath(t, res, h(".claude/agents/reviewer.md")), "claude")
		mustContain(t, "what", e.What, "Reviews diffs for correctness and style")
	})
	t.Run("codex prompt", func(t *testing.T) {
		e := ex(byPath(t, res, h(".codex/prompts/review.md")), "codex")
		if e.Kind != "Custom prompt" {
			t.Errorf("kind = %q", e.Kind)
		}
		mustContain(t, "what", e.What, "/prompts:review", "Review the current branch")
	})
	t.Run("exec policy", func(t *testing.T) {
		e := ex(byPath(t, res, h(".codex/rules/default.rules")), "codex")
		if e.Kind != "Execution policy rules" || e.Loads[0].State != effective.Enforced {
			t.Errorf("%q %+v", e.Kind, e.Loads[0])
		}
	})
}

func TestMarkdownSections(t *testing.T) {
	fx := testfixture.New(t)
	res := discovery.Discover(fx.Env(fx.Project), nil)
	ctx := effective.ContextOf(res)
	for _, agent := range discovery.AllAgents {
		for i := range res.Items {
			md := Explain([]*discovery.Item{&res.Items[i]}, agent, ctx).Markdown()
			for _, h := range []string{"## What is this?", "## Where does it come from?",
				"## How does " + AgentName(agent) + " discover it?", "## When does it load?",
				"## How does it relate to other context?", "## Limits"} {
				if !strings.Contains(md, h) {
					t.Fatalf("%s / %s lacks heading %q", agent, res.Items[i].Name, h)
				}
			}
		}
	}
}

func TestDescriptionCapped(t *testing.T) {
	fx := testfixture.New(t)
	long := strings.Repeat("word ", 100)
	if err := os.MkdirAll(fx.P(".claude/agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fx.P(".claude/agents/long.md"), []byte("---\nname: long\ndescription: "+long+"\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := discovery.Discover(fx.Env(fx.Project), nil)
	e := Explain(byPath(t, res, fx.P(".claude/agents/long.md")), "claude", effective.ContextOf(res))
	if len([]rune(e.What)) > 200+len("A subagent: a specialised helper with its own prompt and tools that the main agent can delegate work to. Its description reads: \"\"")+5 {
		t.Errorf("description not capped (%d runes)", len([]rune(e.What)))
	}
	mustContain(t, "what", e.What, "…")
}

var mechRef = regexp.MustCompile(`Mech[A-Za-z]+`)

// producedMechanisms reads the discovery adapters' source to list the
// mechanisms each agent's adapter can produce.
func producedMechanisms(t *testing.T) map[string]map[discovery.Mechanism]bool {
	t.Helper()
	byName := map[string]discovery.Mechanism{}
	for _, m := range discovery.AllMechanisms {
		byName["mech"+strings.ReplaceAll(string(m), "-", "")] = m
	}
	read := func(name string) []string {
		b, err := os.ReadFile(filepath.Join("..", "discovery", name))
		if err != nil {
			t.Fatal(err)
		}
		return mechRef.FindAllString(string(b), -1)
	}
	out := map[string]map[discovery.Mechanism]bool{}
	add := func(agent string, names []string) {
		if out[agent] == nil {
			out[agent] = map[discovery.Mechanism]bool{}
		}
		for _, n := range names {
			if m, ok := byName[strings.ToLower(n)]; ok {
				out[agent][m] = true
			} else if n != "Mechanism" {
				t.Fatalf("unknown mechanism constant %s", n)
			}
		}
	}
	add(discovery.AgentClaude, append(read("claude.go"), read("runtime.go")...))
	add(discovery.AgentCodex, read("codex.go"))
	add(discovery.AgentCursor, read("cursor.go"))
	// adapters.go (nested scan) and imports.go
	add(discovery.AgentClaude, []string{"MechNestedInstructions", "MechImport"})
	add(discovery.AgentCodex, []string{"MechNestedInstructions"})
	add(discovery.AgentCursor, []string{"MechNestedInstructions", "MechNestedRules"})
	return out
}

func TestKnowledgeTableCompleteness(t *testing.T) {
	for agent, mechs := range producedMechanisms(t) {
		if len(mechs) == 0 {
			t.Fatalf("no mechanisms found for %s", agent)
		}
		for m := range mechs {
			if strings.TrimSpace(Discovery[agent][m]) == "" {
				t.Errorf("no discovery text for (%s, %s)", agent, m)
			}
		}
	}
	// Everything in the table is a known mechanism, and every finding in the fixture is covered.
	known := map[discovery.Mechanism]bool{}
	for _, m := range discovery.AllMechanisms {
		known[m] = true
	}
	for agent, tbl := range Discovery {
		for m, txt := range tbl {
			if !known[m] || txt == "" {
				t.Errorf("bad table entry (%s, %s)", agent, m)
			}
		}
	}
	fx := testfixture.New(t)
	for _, dir := range []string{fx.Project, fx.P("services/api")} {
		res := discovery.Discover(fx.Env(dir), nil)
		var walk func(items []discovery.Item)
		walk = func(items []discovery.Item) {
			for i := range items {
				for _, f := range items[i].Found {
					if Discovery[f.Agent][f.Mechanism] == "" {
						t.Errorf("fixture finding (%s, %s) has no discovery text", f.Agent, f.Mechanism)
					}
				}
				walk(items[i].Children)
			}
		}
		walk(res.Items)
	}
}
