package discovery

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func children(it Item) []string {
	var out []string
	for _, c := range it.Children {
		out = append(out, c.Name)
	}
	return out
}

func TestManagedPolicy(t *testing.T) {
	f := newFixture(t)
	etc := filepath.Join(f.root, "etc", "claude-code")
	md := f.write(filepath.Join(etc, "CLAUDE.md"), "# Company policy")
	f.write(filepath.Join(etc, "managed-settings.json"), `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"/opt/audit.sh"}]}]}}`)
	f.write(filepath.Join(etc, "managed-mcp.json"), `{"mcpServers":{"corp-wiki":{"url":"https://wiki.example.com/mcp"}}}`)

	res := f.discover(f.proj)
	if it := find(t, res, md); it.Scope != ScopeSystem || !strings.Contains(it.Note, "managed policy") {
		t.Errorf("managed CLAUDE.md: %+v", it)
	}
	findMCP(t, res, "corp-wiki", ScopeSystem)
	var hook *Item
	for i := range res.Items {
		if res.Items[i].Category == CategoryHook {
			hook = &res.Items[i]
		}
	}
	if hook == nil || hook.Scope != ScopeSystem || hook.Name != "SessionStart · /opt/audit.sh" {
		t.Fatalf("managed hook: %+v", hook)
	}
}

func TestAutoMemory(t *testing.T) {
	f := newFixture(t)
	key := claudeProjectKey(f.proj)
	if strings.ContainsAny(key, "/._") {
		t.Fatalf("key not sanitized: %s", key)
	}
	mem := filepath.Join(f.home, ".claude", "projects", key, "memory")
	f.write(filepath.Join(mem, "MEMORY.md"), "- [Name](name.md)")
	f.write(filepath.Join(mem, "name.md"), "the name")
	f.write(filepath.Join(f.home, ".claude", "projects", "-other-project", "memory", "MEMORY.md"), "other")

	res := f.discover(f.proj)
	it := find(t, res, mem)
	if it.Category != CategoryMemory || it.Scope != ScopeProject || !it.IsDir {
		t.Fatalf("memory: %+v", it)
	}
	if got := children(it); !reflect.DeepEqual(got, []string{"MEMORY.md", "name.md"}) {
		t.Errorf("memory children = %v", got)
	}
	n := 0
	for _, i := range res.Items {
		if i.Category == CategoryMemory {
			n++
		}
	}
	if n != 1 {
		t.Errorf("found %d memory dirs, want only this project's", n)
	}
}

func TestHooksAndScripts(t *testing.T) {
	f := newFixture(t)
	// User hook with a script path under ~.
	script := f.write(filepath.Join(f.home, ".claude", "hooks", "state.sh"), "echo hi")
	f.write(filepath.Join(f.home, ".claude", "settings.json"), `{"hooks":{
		"SessionStart":[{"matcher":"startup","hooks":[{"type":"command","command":"bash '`+script+`' session","timeout":10}]}],
		"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"curl -H 'Authorization: Bearer abcdefghijklmnop' https://x"}]}]}}`)
	// Plugin hook using ${CLAUDE_PLUGIN_ROOT} and a sibling script argument.
	root := filepath.Join(f.home, ".claude", "plugins", "cache", "m", "sp", "1.0")
	f.write(filepath.Join(root, "hooks", "run-hook.cmd"), "runner")
	f.write(filepath.Join(root, "hooks", "session-start"), "cat skill.md")
	f.write(filepath.Join(root, "hooks", "hooks.json"), `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"\"${CLAUDE_PLUGIN_ROOT}/hooks/run-hook.cmd\" session-start"}]}]}}`)
	f.write(filepath.Join(f.home, ".claude", "plugins", "installed_plugins.json"), `{"plugins":{"sp@m":[{"scope":"user","installPath":"`+root+`"}]}}`)
	// Codex (same layout) and Cursor (flat layout, relative script).
	f.write(filepath.Join(f.home, ".codex", "hooks.json"), `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"true"}]}]}}`)
	f.write(filepath.Join(f.proj, ".cursor", "hooks", "fmt.sh"), "prettier")
	f.write(filepath.Join(f.proj, ".cursor", "hooks.json"), `{"version":1,"hooks":{"afterFileEdit":[{"command":"./hooks/fmt.sh"}]}}`)

	res := f.discover(f.proj)
	hooks := map[string]Item{}
	for _, it := range res.Items {
		if it.Category == CategoryHook {
			hooks[it.Name] = it
		}
	}
	user, ok := hooks["SessionStart · bash '~/.claude/hooks/state.sh' session"]
	if !ok {
		t.Fatalf("user hook missing; have %v", keys(hooks))
	}
	if !strings.Contains(user.Note, "added to the agent's context") || user.Config["matcher"] != "startup" ||
		!reflect.DeepEqual(children(user), []string{"state.sh"}) {
		t.Errorf("user hook: %+v", user)
	}
	for name, h := range hooks {
		if strings.HasPrefix(name, "PreToolUse") {
			if strings.Contains(name, "abcdefghijklmnop") || strings.Contains(h.Config["command"].(string), "abcdefghijklmnop") {
				t.Errorf("token leaked in hook: %q / %v", name, h.Config["command"])
			}
		}
	}
	plugin, ok := hooks[`SessionStart · "<plugin>/hooks/run-hook.cmd" session-start`]
	if !ok {
		t.Fatalf("plugin hook missing; have %v", keys(hooks))
	}
	if got := children(plugin); !reflect.DeepEqual(got, []string{"run-hook.cmd", "session-start"}) {
		t.Errorf("plugin hook scripts = %v", got)
	}
	if !strings.Contains(plugin.Note, "plugin sp") {
		t.Errorf("plugin note = %q", plugin.Note)
	}
	if h := hooks["SessionStart · true"]; len(h.Agents) != 1 || h.Agents[0] != AgentCodex {
		t.Errorf("codex hook: %+v", h)
	}
	cur := hooks["afterFileEdit · ./hooks/fmt.sh"]
	if cur.Scope != ScopeProject || !reflect.DeepEqual(children(cur), []string{"fmt.sh"}) {
		t.Errorf("cursor hook: %+v", cur)
	}
}

func keys(m map[string]Item) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestImports(t *testing.T) {
	f := newFixture(t)
	f.write(filepath.Join(f.proj, "CLAUDE.md"), "See @README.md and @docs/style.md.\n"+
		"Mail me@example.com, ping @alice.\n"+
		"Missing: @docs/gone.md\n"+
		"`@ignored/inline.md`\n```\n@ignored/fenced.md\n```\n"+
		"Personal: @~/.claude/me.md\n")
	f.write(filepath.Join(f.proj, "README.md"), "readme, back to @CLAUDE.md")
	f.write(filepath.Join(f.proj, "docs", "style.md"), "style, see @./tabs.md")
	f.write(filepath.Join(f.proj, "docs", "tabs.md"), "tabs")
	f.write(filepath.Join(f.home, ".claude", "me.md"), "me")

	res := f.discover(f.proj)
	root := find(t, res, filepath.Join(f.proj, "CLAUDE.md"))
	if got := children(root); !reflect.DeepEqual(got, []string{"@README.md", "@docs/style.md", "@docs/gone.md", "@~/.claude/me.md"}) {
		t.Fatalf("imports = %v", got)
	}
	if len(root.Children[0].Children) != 0 {
		t.Errorf("import cycle back to CLAUDE.md should be cut: %+v", root.Children[0].Children)
	}
	style := root.Children[1]
	if got := children(style); !reflect.DeepEqual(got, []string{"@./tabs.md"}) || style.Children[0].Path != filepath.Join(f.proj, "docs", "tabs.md") {
		t.Errorf("nested import: %+v", style.Children)
	}
	if gone := root.Children[2]; !gone.Missing || !strings.Contains(gone.Note, "not found") {
		t.Errorf("missing import: %+v", gone)
	}
	// AGENTS.md is not a Claude file: imports aren't expanded there.
	f.write(filepath.Join(f.proj, "AGENTS.md"), "@README.md")
	res = f.discover(f.proj)
	if a := find(t, res, filepath.Join(f.proj, "AGENTS.md")); len(a.Children) != 0 {
		t.Errorf("AGENTS.md imports expanded: %v", children(a))
	}
}

func TestAgentsCommandsOutputStyles(t *testing.T) {
	f := newFixture(t)
	f.write(filepath.Join(f.proj, ".claude", "agents", "reviewer.md"), "---\nname: reviewer\n---\nReview.")
	f.write(filepath.Join(f.home, ".claude", "agents", "planner.md"), "plan")
	f.write(filepath.Join(f.proj, ".claude", "commands", "ship.md"), "ship it")
	f.write(filepath.Join(f.proj, ".claude", "commands", "git", "pr.md"), "pr")
	f.write(filepath.Join(f.home, ".claude", "output-styles", "terse.md"), "be terse")
	f.write(filepath.Join(f.home, ".claude", "settings.json"), `{"outputStyle":"terse"}`)
	f.write(filepath.Join(f.home, ".codex", "prompts", "review.md"), "codex prompt")
	f.write(filepath.Join(f.proj, ".cursor", "commands", "lint.md"), "lint")
	root := filepath.Join(f.home, ".claude", "plugins", "cache", "m", "sp", "1.0")
	f.write(filepath.Join(root, "agents", "helper.md"), "helper")
	f.write(filepath.Join(f.home, ".claude", "plugins", "installed_plugins.json"), `{"plugins":{"sp@m":[{"installPath":"`+root+`"}]}}`)

	res := f.discover(f.proj)
	cases := []struct {
		path  string
		cat   Category
		scope Scope
		agent string
	}{
		{filepath.Join(f.proj, ".claude", "agents"), CategoryAgent, ScopeProject, AgentClaude},
		{filepath.Join(f.home, ".claude", "agents"), CategoryAgent, ScopeGlobal, AgentClaude},
		{filepath.Join(f.proj, ".claude", "commands"), CategoryCommand, ScopeProject, AgentClaude},
		{filepath.Join(f.home, ".claude", "output-styles"), CategoryOutputStyle, ScopeGlobal, AgentClaude},
		{filepath.Join(f.home, ".codex", "prompts"), CategoryCommand, ScopeGlobal, AgentCodex},
		{filepath.Join(f.proj, ".cursor", "commands"), CategoryCommand, ScopeProject, AgentCursor},
		{filepath.Join(root, "agents"), CategoryAgent, ScopeGlobal, AgentClaude},
	}
	for _, c := range cases {
		it := find(t, res, c.path)
		if it.Category != c.cat || it.Scope != c.scope || it.Agents[0] != c.agent {
			t.Errorf("%s: cat=%s scope=%s agents=%v", c.path, it.Category, it.Scope, it.Agents)
		}
	}
	if got := children(find(t, res, filepath.Join(f.proj, ".claude", "commands"))); !reflect.DeepEqual(got, []string{"git/", "ship.md"}) {
		t.Errorf("commands children = %v", got)
	}
	if n := find(t, res, filepath.Join(f.home, ".claude", "output-styles")).Note; n != "active output style: terse" {
		t.Errorf("output style note = %q", n)
	}
	if name := find(t, res, filepath.Join(root, "agents")).Name; name != "sp: agents/" {
		t.Errorf("plugin agents name = %q", name)
	}
}

func TestRuntimeInfo(t *testing.T) {
	f := newFixture(t)
	f.write(filepath.Join(f.home, ".claude.json"), `{"claudeAiMcpEverConnected":["claude.ai Gmail","claude.ai Notion"]}`)
	res := f.discover(f.proj)
	got := map[string]Item{}
	for _, it := range res.Items {
		if it.Category == CategoryRuntime {
			got[it.Name] = it
		}
	}
	for _, name := range []string{"Claude Code system prompt", "Session environment", "claude.ai connectors",
		"Launch flags & hook output", "Codex system prompt", "Cursor system prompt & User Rules"} {
		it, ok := got[name]
		if !ok || it.Info == "" || it.Scope != ScopeSystem || it.Path != "" {
			t.Errorf("runtime item %q: %+v", name, it)
		}
	}
	if c := got["claude.ai connectors"].Info; !strings.Contains(c, "- claude.ai Gmail") || !strings.Contains(c, "may since have been disconnected") {
		t.Errorf("connectors info:\n%s", c)
	}
	// Only the selected agents' runtime entries appear.
	res = f.discover(f.proj, AgentCodex)
	for _, it := range res.Items {
		if it.Category == CategoryRuntime && it.Name != "Codex system prompt" {
			t.Errorf("unexpected runtime item %q for codex-only", it.Name)
		}
	}
}

func TestDeferredPerAgent(t *testing.T) {
	f := newFixture(t)
	f.write(filepath.Join(f.proj, "AGENTS.md"), "shared")
	f.write(filepath.Join(f.proj, "AGENTS.override.md"), "codex only")
	f.write(filepath.Join(f.proj, "sub", "CLAUDE.md"), "nested")
	res := f.discover(f.proj)

	agents := find(t, res, filepath.Join(f.proj, "AGENTS.md"))
	if !agents.Deferred(AgentCodex) || agents.Deferred(AgentCursor) || agents.DeferredForAll() {
		t.Errorf("AGENTS.md: shadowed for codex only, got DeferredFor=%v", agents.DeferredFor)
	}
	if override := find(t, res, filepath.Join(f.proj, "AGENTS.override.md")); len(override.DeferredFor) != 0 {
		t.Errorf("override should load: %v", override.DeferredFor)
	}
	if nested := find(t, res, filepath.Join(f.proj, "sub", "CLAUDE.md")); !nested.DeferredForAll() {
		t.Errorf("nested CLAUDE.md should be deferred: %v", nested.DeferredFor)
	}
}
