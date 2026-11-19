package budget

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/R037/ayca/internal/discovery"
)

func write(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// setup builds a home and a git project and returns the discovery result.
func setup(t *testing.T, build func(home, proj string)) discovery.Result {
	t.Helper()
	root := t.TempDir()
	home, proj := filepath.Join(root, "home"), filepath.Join(root, "proj")
	if err := os.MkdirAll(filepath.Join(proj, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	build(home, proj)
	env := discovery.NewEnvWith(proj, home, func(string) string { return "" })
	env.ManagedDir = filepath.Join(root, "managed")
	return discovery.Discover(env, nil)
}

func agent(t *testing.T, rep Report, name string) Agent {
	t.Helper()
	for _, a := range rep.Agents {
		if a.Agent == name {
			return a
		}
	}
	t.Fatalf("no estimate for %s", name)
	return Agent{}
}

func entry(a Agent, name string) (Entry, bool) {
	for _, e := range a.Entries {
		if e.Name == name {
			return e, true
		}
	}
	return Entry{}, false
}

func TestEstimateClaude(t *testing.T) {
	res := setup(t, func(home, proj string) {
		write(t, filepath.Join(proj, "CLAUDE.md"), strings.Repeat("a", 400)+"\nSee @docs/extra.md\n")
		write(t, filepath.Join(proj, "docs", "extra.md"), strings.Repeat("b", 800))
		write(t, filepath.Join(proj, "sub", "CLAUDE.md"), strings.Repeat("c", 4000)) // nested: on demand
		write(t, filepath.Join(home, ".claude", "skills", "pdf", "SKILL.md"),
			"---\nname: pdf\ndescription: Work with PDF files\n---\n"+strings.Repeat("x", 10000))
		write(t, filepath.Join(proj, ".claude", "rules", "always.md"), strings.Repeat("r", 200))
		write(t, filepath.Join(proj, ".claude", "rules", "go.md"), "---\npaths:\n  - \"**/*.go\"\n---\n"+strings.Repeat("g", 2000))
		write(t, filepath.Join(proj, ".claude", "commands", "ship.md"), "---\ndescription: Ship it\n---\nbody")
		write(t, filepath.Join(proj, ".claude", "commands", "plain.md"), strings.Repeat("p", 2000))
		write(t, filepath.Join(home, ".claude", "settings.json"), `{"outputStyle":"terse"}`)
		write(t, filepath.Join(home, ".claude", "output-styles", "terse.md"), "---\nname: terse\n---\n"+strings.Repeat("t", 40))
		write(t, filepath.Join(home, ".claude", "output-styles", "chatty.md"), strings.Repeat("y", 4000))
		write(t, filepath.Join(proj, ".mcp.json"), `{"mcpServers":{"gh":{"command":"gh-mcp"}}}`)
	})
	rep := Estimate(res, nil, nil)
	a := agent(t, rep, discovery.AgentClaude)

	want := map[string]int{
		"CLAUDE.md":      tokens(400 + len("\nSee @docs/extra.md\n")),
		"@docs/extra.md": 200,
		"pdf":            tokens(len("pdf") + len("Work with PDF files") + 4),
	}
	if e, ok := entry(a, "CLAUDE.md"); !ok || e.Tokens != want["CLAUDE.md"] || e.How != HowFull {
		t.Errorf("CLAUDE.md entry = %+v", e)
	}
	if e, ok := entry(a, "@docs/extra.md"); !ok || e.Tokens != 200 {
		t.Errorf("@import entry = %+v, %v", e, ok)
	}
	if e, ok := entry(a, "pdf"); !ok || e.Tokens != want["pdf"] || e.How != HowDescription {
		t.Errorf("skill entry = %+v", e)
	}
	if e, ok := entry(a, "always.md"); !ok || e.Tokens != 50 {
		t.Errorf("unconditional rule = %+v, %v", e, ok)
	}
	if _, ok := entry(a, "go.md"); ok {
		t.Error("rule with paths: should not count at startup")
	}
	if _, ok := entry(a, "sub/CLAUDE.md"); ok {
		t.Error("nested CLAUDE.md should not count at startup")
	}
	if e, ok := entry(a, "ship.md"); !ok || e.How != HowDescription {
		t.Errorf("command with description = %+v, %v", e, ok)
	}
	if _, ok := entry(a, "plain.md"); ok {
		t.Error("command without a description should not count")
	}
	if e, ok := entry(a, "terse.md"); !ok || e.Tokens != 10 {
		t.Errorf("active output style = %+v, %v", e, ok)
	}
	if _, ok := entry(a, "chatty.md"); ok {
		t.Error("inactive output style should not count")
	}

	sum := 0
	for _, e := range a.Entries {
		sum += e.Tokens
	}
	if a.Tokens != sum || a.Window != 200_000 {
		t.Errorf("total %d (sum %d), window %d", a.Tokens, sum, a.Window)
	}
	joined := strings.Join(a.NotCounted, ";")
	if !strings.Contains(joined, "1 MCP server") || !strings.Contains(joined, "system prompt") {
		t.Errorf("not counted = %v", a.NotCounted)
	}
	if !strings.Contains(strings.Join(a.Notes, ";"), "1 conditional rule") {
		t.Errorf("notes = %v", a.Notes)
	}
	// Largest first.
	for i := 1; i < len(a.Entries); i++ {
		if a.Entries[i].Tokens > a.Entries[i-1].Tokens {
			t.Fatalf("entries not sorted: %+v", a.Entries)
		}
	}
	if _, ok := rep.ByID[a.Entries[0].ItemID]; !ok {
		t.Error("ByID missing the largest entry")
	}
}

func TestMemoryFirst200Lines(t *testing.T) {
	res := setup(t, func(home, p string) {
		key := strings.Map(func(r rune) rune {
			if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
				return r
			}
			return '-'
		}, p)
		write(t, filepath.Join(home, ".claude", "projects", key, "memory", "MEMORY.md"), strings.Repeat("123\n", 500))
	})
	a := agent(t, Estimate(res, []string{discovery.AgentClaude}, nil), discovery.AgentClaude)
	if e, ok := entry(a, "auto memory"); !ok || e.Tokens != 200 || e.How != HowMemory {
		t.Errorf("memory entry = %+v, %v (entries %+v)", e, ok, a.Entries)
	}
}

func TestEstimateCodexAndCursor(t *testing.T) {
	res := setup(t, func(home, proj string) {
		write(t, filepath.Join(proj, "AGENTS.md"), strings.Repeat("s", 4000)) // shadowed by override
		write(t, filepath.Join(proj, "AGENTS.override.md"), strings.Repeat("o", 40))
		write(t, filepath.Join(home, ".codex", "rules", "default.rules"), strings.Repeat("e", 400))
		write(t, filepath.Join(proj, ".cursor", "rules", "always.mdc"), "---\nalwaysApply: true\n---\n"+strings.Repeat("c", 80))
		write(t, filepath.Join(proj, ".cursor", "rules", "ts.mdc"), "---\nglobs: \"*.ts\"\nalwaysApply: false\n---\n"+strings.Repeat("t", 800))
		write(t, filepath.Join(proj, ".cursor", "rules", "db.mdc"), "---\ndescription: Database rules\n---\n"+strings.Repeat("d", 800))
	})
	rep := Estimate(res, []string{discovery.AgentCodex, discovery.AgentCursor}, map[string]int{discovery.AgentCodex: 1000})
	if len(rep.Agents) != 2 {
		t.Fatalf("agents = %d, want 2", len(rep.Agents))
	}

	codex := agent(t, rep, discovery.AgentCodex)
	if codex.Window != 1000 {
		t.Errorf("window override not applied: %d", codex.Window)
	}
	if e, ok := entry(codex, "AGENTS.override.md"); !ok || e.Tokens != 10 {
		t.Errorf("override = %+v, %v", e, ok)
	}
	if _, ok := entry(codex, "AGENTS.md"); ok {
		t.Error("shadowed AGENTS.md should not count")
	}
	if _, ok := entry(codex, "default.rules"); ok {
		t.Error("execution policy rules are not prompt context")
	}
	if p := codex.Percent(); p != 1 {
		t.Errorf("percent = %v, want 1", p)
	}

	cursor := agent(t, rep, discovery.AgentCursor)
	if e, ok := entry(cursor, "always.mdc"); !ok || e.Tokens != 20 {
		t.Errorf("alwaysApply rule = %+v, %v", e, ok)
	}
	if _, ok := entry(cursor, "ts.mdc"); ok {
		t.Error("glob rule should not count")
	}
	if e, ok := entry(cursor, "db.mdc"); !ok || e.Tokens >= 200 {
		t.Errorf("agent-requested rule should count its description only: %+v, %v", e, ok)
	}
	if cursor.Window != DefaultWindows[discovery.AgentCursor] {
		t.Errorf("cursor window = %d", cursor.Window)
	}
}

func TestCodexProjectCap(t *testing.T) {
	res := setup(t, func(home, proj string) {
		write(t, filepath.Join(proj, "AGENTS.md"), strings.Repeat("x", 64*1024))
	})
	a := agent(t, Estimate(res, []string{discovery.AgentCodex}, nil), discovery.AgentCodex)
	if a.Tokens != tokens(codexProjectCap) || len(a.Notes) == 0 {
		t.Errorf("tokens = %d, want capped at %d; notes %v", a.Tokens, tokens(codexProjectCap), a.Notes)
	}
}

func TestEmpty(t *testing.T) {
	res := setup(t, func(home, proj string) {})
	for _, a := range Estimate(res, nil, nil).Agents {
		if a.Tokens != 0 || len(a.Entries) != 0 {
			t.Errorf("%s: %+v", a.Agent, a)
		}
	}
}

func TestParseWindowAndHuman(t *testing.T) {
	for in, want := range map[string]int{"200000": 200000, "200k": 200000, "1m": 1000000, "1.5M": 1500000} {
		if got, err := ParseWindow(in); err != nil || got != want {
			t.Errorf("ParseWindow(%q) = %d, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "abc", "-5", "200x"} {
		if _, err := ParseWindow(bad); err == nil {
			t.Errorf("ParseWindow(%q) should fail", bad)
		}
	}
	for n, want := range map[int]string{950: "950", 1000: "1k", 12345: "12.3k", 1_200_000: "1.2M"} {
		if got := Human(n); got != want {
			t.Errorf("Human(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestFrontmatter(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.md")
	write(t, p, "---\nname: 'demo'\ndescription: >\n  folded line one\n  line two\nglobs:\n  - a\n  - b\n---\nbody\n")
	fm := frontmatter(p)
	if fm["name"] != "demo" || fm["description"] != "folded line one line two" || fm["globs"] != "a, b" {
		t.Errorf("frontmatter = %#v", fm)
	}
	if bodyTokens(p) != tokens(len("body\n")) {
		t.Errorf("bodyTokens = %d", bodyTokens(p))
	}
}

func TestByCategory(t *testing.T) {
	a := Agent{Entries: []Entry{
		{Category: discovery.CategorySkill, Tokens: 10},
		{Category: discovery.CategoryInstruction, Tokens: 5},
		{Category: discovery.CategorySkill, Tokens: 7},
	}}
	got := a.ByCategory()
	if len(got) != 2 || got[0].Category != discovery.CategoryInstruction || got[1].Tokens != 17 || got[1].Count != 2 {
		t.Errorf("ByCategory = %+v", got)
	}
}
