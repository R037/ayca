package effective

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/R037/ayca/internal/budget"
	"github.com/R037/ayca/internal/discovery"
	"github.com/R037/ayca/internal/testfixture"
)

const (
	claude = discovery.AgentClaude
	codex  = discovery.AgentCodex
	cursor = discovery.AgentCursor
)

// chainWhere finds the first chain (depth first) whose last item matches pred.
func chainWhere(items []discovery.Item, pred func(chain []*discovery.Item) bool) []*discovery.Item {
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

func byPath(res discovery.Result, path string) []*discovery.Item {
	return chainWhere(res.Items, func(c []*discovery.Item) bool { return c[len(c)-1].Path == path })
}

func byName(res discovery.Result, name string) []*discovery.Item {
	return chainWhere(res.Items, func(c []*discovery.Item) bool { return c[len(c)-1].Name == name })
}

func mustChain(t *testing.T, c []*discovery.Item, what string) []*discovery.Item {
	t.Helper()
	if c == nil {
		t.Fatalf("fixture item not found: %s", what)
	}
	return c
}

func discoverAt(fx *testfixture.Fixture, dir string) discovery.Result {
	return discovery.Discover(fx.Env(dir), nil)
}

func TestClassifyFixture(t *testing.T) {
	fx := testfixture.New(t)
	res := discoverAt(fx, fx.Project)
	ctx := ContextOf(res)
	p, h := fx.P, fx.H

	type tc struct {
		name   string
		agent  string
		chain  func() []*discovery.Item
		state  State
		conf   Confidence // "" = documented
		reason []string   // substrings
	}
	path := func(pth string) func() []*discovery.Item { return func() []*discovery.Item { return byPath(res, pth) } }
	name := func(n string) func() []*discovery.Item { return func() []*discovery.Item { return byName(res, n) } }
	// child of the item at top path, selected by child path
	child := func(top, ch string) func() []*discovery.Item {
		return func() []*discovery.Item {
			return chainWhere(res.Items, func(c []*discovery.Item) bool {
				return len(c) > 1 && c[0].Path == top && c[len(c)-1].Path == ch
			})
		}
	}
	imp := func(top, ch string) func() []*discovery.Item { return child(top, ch) }
	hook := func(prefix string) func() []*discovery.Item {
		return func() []*discovery.Item {
			return chainWhere(res.Items, func(c []*discovery.Item) bool {
				l := c[len(c)-1]
				return l.Category == discovery.CategoryHook && len(c) == 1 && strings.HasPrefix(l.Name, prefix)
			})
		}
	}
	sk := h(".claude/skills/frontend-design")
	cases := []tc{
		// ---- Claude Code
		{"managed", claude, path(filepath.Join(fx.ManagedDir, "CLAUDE.md")), Startup, "", []string{"Managed policy"}},
		{"user", claude, path(h(".claude/CLAUDE.md")), Startup, "", nil},
		{"project", claude, path(p("CLAUDE.md")), Startup, "", []string{"project root"}},
		{"local", claude, path(p("CLAUDE.local.md")), Startup, "", nil},
		{"ancestor", claude, path(filepath.Join(fx.Workspace, "CLAUDE.md")), Startup, "", nil},
		{"import", claude, imp(p("CLAUDE.md"), p("docs/architecture.md")), Startup, "", []string{"imported into CLAUDE.md", "5 levels"}},
		{"import in cycle", claude, imp(h(".claude/CLAUDE.md"), h(".claude/shared/style.md")), Startup, "", []string{"imported into"}},
		{"missing import", claude, imp(p("CLAUDE.md"), p("docs/missing.md")), NotLoaded, "", []string{"file not found"}},
		{"nested", claude, path(p("services/api/CLAUDE.md")), OnDemand, "", []string{"Claude reads files in services/api/"}},
		{"rule style", claude, path(p(".claude/rules/style.md")), Startup, "", nil},
		{"rule backend", claude, path(p(".claude/rules/backend.md")), Conditional, "", []string{"services/**"}},
		{"rules dir", claude, path(p(".claude/rules")), Startup, "", []string{"1 always loaded", "1 conditional"}},
		{"user rule", claude, path(h(".claude/rules/security.md")), Startup, "", nil},
		{"skill", claude, path(p(".claude/skills/deploy")), Listed, "", []string{"name and description listed", "SKILL.md loads when the skill is invoked"}},
		{"skill SKILL.md", claude, child(p(".claude/skills/deploy"), p(".claude/skills/deploy/SKILL.md")), Invocation, "", []string{"full text loads when the skill is invoked"}},
		{"skill script", claude, child(p(".claude/skills/deploy"), p(".claude/skills/deploy/scripts/deploy.sh")), OnDemand, "", []string{"run, not read", "never runs"}},
		{"skill reference", claude, child(sk, sk+"/references/patterns.md"), OnDemand, "", []string{"supporting file"}},
		{"skill folder", claude, child(sk, sk+"/references"), OnDemand, "", nil},
		{"pdf claude", claude, name("pdf"), Listed, "", nil},
		{"disabled plugin skill", claude, name("demo-skill"), Disabled, "", []string{"plugin demo"}},
		{"disabled plugin skill file", claude, func() []*discovery.Item {
			return chainWhere(res.Items, func(c []*discovery.Item) bool {
				return len(c) == 2 && c[0].Name == "demo-skill" && c[1].Name == "SKILL.md"
			})
		}, Disabled, "", []string{"plugin demo"}},
		{"disabled plugin hook", claude, hook("PostToolUse"), Disabled, "", []string{"plugin demo"}},
		{"subagent", claude, path(h(".claude/agents/reviewer.md")), Listed, "", []string{"subagent's prompt"}},
		{"command with description", claude, path(h(".claude/commands/ship.md")), Listed, "", nil},
		{"hook SessionStart", claude, hook("SessionStart"), Event, "", []string{"added to the agent's context"}},
		{"hook PreToolUse", claude, hook("PreToolUse"), Event, "", nil},
		{"hook script", claude, func() []*discovery.Item {
			return chainWhere(res.Items, func(c []*discovery.Item) bool { return len(c) == 2 && c[1].Name == "session-start.sh" })
		}, Event, "", []string{"script itself is not sent"}},
		{"mcp project", claude, name("filesystem"), Unknown, ConfRuntimeDependent, []string{"need your approval", "tool search", "never starts servers"}},
		{"mcp user", claude, name("linear"), Unknown, ConfRuntimeDependent, []string{"tool search"}},
		{"builtin prompt", claude, name("Claude Code system prompt"), Startup, "", []string{"not inspectable"}},
		{"session env", claude, name("Session environment"), Startup, "", nil},
		{"connectors", claude, name("claude.ai connectors"), Unknown, ConfRuntimeDependent, nil},
		{"launch flags", claude, name("Launch flags & hook output"), Unknown, ConfRuntimeDependent, nil},
		// ---- Codex
		{"codex user", codex, path(h(".codex/AGENTS.md")), Startup, "", nil},
		{"codex override", codex, path(p("AGENTS.override.md")), Startup, "", []string{"32 KiB"}},
		{"codex shadowed", codex, path(p("AGENTS.md")), Shadowed, "", []string{"AGENTS.override.md"}},
		{"codex ancestor", codex, path(filepath.Join(fx.Workspace, "AGENTS.md")), NotLoaded, "", []string{"project root"}},
		{"codex nested", codex, path(p("services/api/AGENTS.md")), OnDemand, ConfVersionDependent, []string{"does not load it at startup"}},
		{"codex exec rules", codex, path(h(".codex/rules/default.rules")), Enforced, "", []string{"not sent to the model"}},
		{"codex prompt", codex, path(h(".codex/prompts/review.md")), Invocation, "", []string{"/prompts:review"}},
		{"codex project skill", codex, path(p(".agents/skills/lint")), Listed, "", nil},
		{"codex mcp", codex, name("github"), Unknown, ConfRuntimeDependent, nil},
		{"codex builtin", codex, name("Codex system prompt"), Startup, "", nil},
		{"codex ignores claude file", codex, path(p("CLAUDE.md")), NotLoaded, "", []string{"does not read"}},
		// ---- Cursor
		{"cursor AGENTS.md", cursor, path(p("AGENTS.md")), Startup, "", nil},
		{"cursor nested AGENTS.md", cursor, path(p("services/api/AGENTS.md")), OnDemand, "", []string{"services/api/"}},
		{"cursor always", cursor, path(p(".cursor/rules/always.mdc")), Startup, "", nil},
		{"cursor globs", cursor, path(p(".cursor/rules/typescript.mdc")), Conditional, "", []string{"**/*.ts"}},
		{"cursor agent requested", cursor, path(p(".cursor/rules/agent-picks.mdc")), Listed, "", []string{"Agent Requested"}},
		{"cursor nested rule", cursor, path(p("web/.cursor/rules/react.mdc")), Conditional, "", []string{"**/*.tsx"}},
		{"cursor separate skill", cursor, path(h(".cursor/skills/pdf")), Listed, "", nil},
		{"cursor mcp", cursor, name("search"), Unknown, ConfRuntimeDependent, nil},
		{"cursor app settings", cursor, name("Cursor system prompt & User Rules"), Unknown, ConfRuntimeDependent, nil},
		{"claude pdf for cursor", cursor, name("pdf"), NotLoaded, "", nil}, // first "pdf" is the Claude/Codex one
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ch := mustChain(t, c.chain(), c.name)
			l := ClassifyPath(ch, c.agent, ctx)
			if l.State != c.state {
				t.Fatalf("state = %s (%s), want %s", l.State, l.Reason, c.state)
			}
			want := c.conf
			if want == "" {
				want = ConfDocumented
			}
			if l.Confidence != want {
				t.Errorf("confidence = %s, want %s", l.Confidence, want)
			}
			if l.Agent != c.agent {
				t.Errorf("agent = %s", l.Agent)
			}
			for _, s := range c.reason {
				if !strings.Contains(strings.ToLower(l.Reason), strings.ToLower(s)) {
					t.Errorf("reason %q lacks %q", l.Reason, s)
				}
			}
			if l.Reason == "" || strings.HasSuffix(l.Reason, ".") {
				t.Errorf("reason should be one sentence without trailing period: %q", l.Reason)
			}
		})
	}
}

func TestClassifyPdfAcrossAgents(t *testing.T) {
	fx := testfixture.New(t)
	res := discoverAt(fx, fx.Project)
	ctx := ContextOf(res)
	shared := mustChain(t, byPath(res, fx.H(".claude/skills/pdf")), "shared pdf")
	if got := Classify(shared[0], claude, ctx).State; got != Listed {
		t.Errorf("claude: %s", got)
	}
	if got := Classify(shared[0], codex, ctx).State; got != Listed {
		t.Errorf("codex: %s", got)
	}
	if l := Classify(shared[0], cursor, ctx); l.State != NotLoaded || !strings.Contains(l.Reason, "does not read") {
		t.Errorf("cursor on shared pdf: %+v", l)
	}
	own := mustChain(t, byPath(res, fx.H(".cursor/skills/pdf")), "cursor pdf")
	if own[0] == shared[0] {
		t.Fatal("cursor pdf must be a separate item")
	}
	if got := Classify(own[0], claude, ctx).State; got != NotLoaded {
		t.Errorf("claude on cursor pdf: %s", got)
	}
}

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestClassifyExtras(t *testing.T) {
	fx := testfixture.New(t)
	write(t, fx.P(".claude/settings.local.json"), `{"outputStyle":"Terse"}`)
	write(t, fx.P(".claude/output-styles/terse.md"), "---\nname: Terse\n---\nBe brief.\n")
	write(t, fx.P(".claude/output-styles/verbose.md"), "Be verbose.\n")
	write(t, fx.P(".claude/skills/manual/SKILL.md"), "---\nname: manual\ndescription: x\ndisable-model-invocation: true\n---\nbody\n")
	write(t, fx.P(".claude/commands/bare.md"), "No description here.\n")
	write(t, fx.P(".cursor/rules/manual.mdc"), "Just text.\n")
	write(t, fx.P(".cursorrules"), "old rules\n")
	memKey := regexp.MustCompile(`[^A-Za-z0-9]`).ReplaceAllString(fx.Project, "-")
	mem := fx.H(".claude/projects/" + memKey + "/memory")
	write(t, filepath.Join(mem, "MEMORY.md"), "- [topic](topic.md)\n")
	write(t, filepath.Join(mem, "topic.md"), "details\n")

	res := discoverAt(fx, fx.Project)
	ctx := ContextOf(res)
	if ctx.OutputStyle != "Terse" {
		t.Fatalf("output style = %q", ctx.OutputStyle)
	}
	check := func(agent string, ch []*discovery.Item, st State, sub string) {
		t.Helper()
		ch = mustChain(t, ch, sub)
		l := ClassifyPath(ch, agent, ctx)
		if l.State != st || !strings.Contains(strings.ToLower(l.Reason), strings.ToLower(sub)) {
			t.Errorf("%s: %s -> %s (%s), want %s containing %q", agent, ch[len(ch)-1].Name, l.State, l.Reason, st, sub)
		}
	}
	check(claude, byPath(res, fx.P(".claude/output-styles/terse.md")), Startup, "active output style")
	check(claude, byPath(res, fx.P(".claude/output-styles/verbose.md")), NotLoaded, "not the active output style")
	check(claude, byPath(res, fx.P(".claude/output-styles")), Startup, "1 active")
	check(claude, byPath(res, fx.P(".claude/skills/manual")), Invocation, "/manual")
	check(claude, byPath(res, fx.P(".claude/commands/bare.md")), Invocation, "/bare")
	check(claude, byPath(res, fx.P(".claude/commands")), Invocation, "1 manual")
	check(cursor, byPath(res, fx.P(".cursor/rules/manual.mdc")), Invocation, "@-mention")
	check(cursor, byPath(res, fx.P(".cursorrules")), Startup, "Legacy")
	if l := Classify(mustChain(t, byPath(res, fx.P(".cursorrules")), "")[0], cursor, ctx); l.Confidence != ConfVersionDependent {
		t.Errorf("cursorrules confidence = %s", l.Confidence)
	}
	check(claude, byPath(res, mem), Startup, "first 200 lines")
	check(claude, byPath(res, filepath.Join(mem, "MEMORY.md")), Startup, "MEMORY.md")
	check(claude, byPath(res, filepath.Join(mem, "topic.md")), OnDemand, "topic file")
}

func TestLabels(t *testing.T) {
	for _, s := range []State{Startup, Listed, OnDemand, Conditional, Invocation, Event, Enforced, Disabled, Shadowed, NotLoaded, Unknown} {
		if s.Label() == "" || s.Describe() == "" {
			t.Errorf("%s lacks label or description", s)
		}
	}
	if RulesBasis == "" || !strings.Contains(RulesBasis, "2.1.328") || !strings.Contains(RulesBasis, "0.167.1") {
		t.Errorf("RulesBasis = %q", RulesBasis)
	}
}

func TestProjection(t *testing.T) {
	fx := testfixture.New(t)
	root := discoverAt(fx, fx.Project)
	api := discoverAt(fx, fx.P("services/api"))

	where := func(p Projection, name string) GroupKey {
		for _, g := range p.Groups {
			for _, e := range g.Entries {
				if e.Item().Path == fx.P(name) {
					return g.Key
				}
			}
		}
		return ""
	}
	atRoot := Project(root, root, claude)
	atAPI := Project(api, root, claude)
	if len(atRoot.Groups) != 5 {
		t.Fatalf("groups = %d", len(atRoot.Groups))
	}
	for i, k := range []GroupKey{GroupApplicable, GroupDiscoverable, GroupConditional, GroupNotLoaded, GroupUnknown} {
		if atRoot.Groups[i].Key != k || atRoot.Groups[i].Label == "" || atRoot.Groups[i].Help == "" {
			t.Errorf("group %d = %+v", i, atRoot.Groups[i].Key)
		}
	}
	if got := where(atRoot, "services/api/CLAUDE.md"); got != GroupConditional {
		t.Errorf("at root api/CLAUDE.md in %q", got)
	}
	if got := where(atAPI, "services/api/CLAUDE.md"); got != GroupApplicable {
		t.Errorf("at api api/CLAUDE.md in %q", got)
	}
	if got := where(atRoot, "web/CLAUDE.md"); got != GroupConditional {
		t.Errorf("at root web/CLAUDE.md in %q", got)
	}
	if got := where(atAPI, "web/CLAUDE.md"); got != "" {
		t.Errorf("web/CLAUDE.md should not be projected at api, in %q", got)
	}
	if len(atRoot.Excluded) != 0 {
		t.Errorf("root excluded = %d", len(atRoot.Excluded))
	}
	var web *Entry
	for i, e := range atAPI.Excluded {
		if e.Item().Path == fx.P("web/CLAUDE.md") {
			web = &atAPI.Excluded[i]
		}
	}
	if web == nil {
		t.Fatal("web/CLAUDE.md not excluded at services/api")
	}
	if web.Load.State != NotLoaded || !strings.Contains(web.Load.Reason, "In web/, which is not on the path from the project root to services/api/") {
		t.Errorf("excluded load = %+v", web.Load)
	}
	// Imports are separate entries, rules expanded to files, skills stay one entry.
	if where(atRoot, "docs/architecture.md") != GroupApplicable {
		t.Error("import entry missing from applicable group")
	}
	if where(atRoot, ".claude/rules/backend.md") != GroupConditional || where(atRoot, ".claude/rules/style.md") != GroupApplicable {
		t.Error("rule files not expanded")
	}
	for _, g := range atRoot.Groups {
		for _, e := range g.Entries {
			if e.Item().Category == discovery.CategorySkill && len(e.Chain) != 1 {
				t.Errorf("skill expanded: %s", e.Item().Name)
			}
		}
	}

	// Cursor: the web rules are excluded at services/api, with a clean reason.
	cur := Project(api, root, cursor)
	found := false
	for _, e := range cur.Excluded {
		if e.Item().Path == fx.P("web/.cursor/rules/react.mdc") {
			found = true
			if !strings.Contains(e.Load.Reason, "In web/, which") {
				t.Errorf("reason = %q", e.Load.Reason)
			}
		}
	}
	if !found {
		t.Error("react.mdc not excluded")
	}
	// Codex: services/api/AGENTS.md becomes startup there.
	if got := where(Project(api, root, codex), "services/api/AGENTS.md"); got != GroupApplicable {
		t.Errorf("codex api AGENTS.md at api in %q", got)
	}
	if got := where(Project(root, root, codex), "services/api/AGENTS.md"); got != GroupConditional {
		t.Errorf("codex api AGENTS.md at root in %q", got)
	}
}

func TestCandidateDirs(t *testing.T) {
	fx := testfixture.New(t)
	root := discoverAt(fx, fx.Project)
	got := CandidateDirs(root, root)
	want := []string{fx.Project, fx.P("services/api"), fx.P("services/payments"), fx.P("web")}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("CandidateDirs = %v, want %v", got, want)
	}
	// Launching below the root adds the cwd and every directory between.
	api := discoverAt(fx, fx.P("services/api"))
	got = CandidateDirs(api, root)
	for _, w := range want {
		found := false
		for _, g := range got {
			found = found || g == w
		}
		if !found {
			t.Errorf("missing %s in %v", w, got)
		}
	}
	if got[0] != fx.Project {
		t.Errorf("root not first: %v", got)
	}
	deep := fx.P("services/api/internal/http")
	write(t, filepath.Join(deep, "x.go"), "package x\n")
	got = CandidateDirs(discoverAt(fx, deep), root)
	for _, w := range []string{fx.P("services/api/internal"), deep} {
		ok := false
		for _, g := range got {
			ok = ok || g == w
		}
		if !ok {
			t.Errorf("missing %s in %v", w, got)
		}
	}
}

// walk calls fn for every item with its chain.
func walk(items []discovery.Item, prefix []*discovery.Item, fn func(chain []*discovery.Item)) {
	for i := range items {
		c := append(append([]*discovery.Item{}, prefix...), &items[i])
		fn(c)
		walk(items[i].Children, c, fn)
	}
}

// TestConsistentWithBudget checks that the budget and these rules agree on
// what is in the startup context.
func TestConsistentWithBudget(t *testing.T) {
	fx := testfixture.New(t)
	for _, dir := range []string{fx.Project, fx.P("services/api")} {
		res := discoverAt(fx, dir)
		ctx := ContextOf(res)
		rep := budget.Estimate(res, nil, nil)
		byID := map[string][]*discovery.Item{}
		walk(res.Items, nil, func(c []*discovery.Item) { byID[c[len(c)-1].ID] = c })
		for _, ag := range rep.Agents {
			counted := map[string]bool{}
			for _, e := range ag.Entries {
				counted[e.ItemID] = true
				c, ok := byID[e.ItemID]
				if !ok {
					t.Errorf("%s @%s: budget entry %s has no item", ag.Agent, filepath.Base(dir), e.ItemID)
					continue
				}
				if st := ClassifyPath(c, ag.Agent, ctx).State; st != Startup && st != Listed {
					t.Errorf("%s @%s: budget counts %s but it classifies as %s", ag.Agent, filepath.Base(dir), e.Name, st)
				}
			}
			if ag.Agent != claude {
				continue
			}
			// Every Startup Claude instruction/import/rule file must be counted.
			// Excluded: runtime items (not sizeable), directories, memory files
			// (the budget counts MEMORY.md under its directory item).
			walk(res.Items, nil, func(c []*discovery.Item) {
				l := c[len(c)-1]
				if l.IsDir || l.Virtual || l.Missing || l.Category == discovery.CategoryRuntime || l.Category == discovery.CategoryMemory {
					return
				}
				if !hasAgent(l, claude) && !hasAgent(c[0], claude) {
					return
				}
				if ClassifyPath(c, claude, ctx).State == Startup && !counted[l.ID] {
					t.Errorf("claude @%s: %s is Startup but missing from budget", filepath.Base(dir), l.Name)
				}
			})
		}
	}
}

func hasAgent(it *discovery.Item, a string) bool {
	for _, x := range it.Agents {
		if x == a {
			return true
		}
	}
	return false
}
