package discovery

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// fixture is a fake filesystem layout: a home directory and a workspace
// containing a git project.
type fixture struct {
	t    *testing.T
	root string
	home string
	ws   string // workspace (ancestor of the project)
	proj string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	f := &fixture{t: t, root: root,
		home: filepath.Join(root, "home"),
		ws:   filepath.Join(root, "ws"),
		proj: filepath.Join(root, "ws", "proj")}
	f.mkdir(f.home)
	f.mkdir(filepath.Join(f.proj, ".git"))
	return f
}

func (f *fixture) mkdir(p string) {
	f.t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) write(p, content string) string {
	f.t.Helper()
	f.mkdir(filepath.Dir(p))
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
	return p
}

func (f *fixture) symlink(target, link string) {
	f.t.Helper()
	f.mkdir(filepath.Dir(link))
	if err := os.Symlink(target, link); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) env(cwd string) Env {
	return NewEnvWith(cwd, f.home, func(string) string { return "" })
}

func (f *fixture) discover(cwd string, agents ...string) Result {
	return Discover(f.env(cwd), agents)
}

func find(t *testing.T, res Result, path string) Item {
	t.Helper()
	for _, it := range res.Items {
		if it.Path == path {
			return it
		}
		for _, a := range it.Aliases {
			if a == path {
				return it
			}
		}
	}
	var got []string
	for _, it := range res.Items {
		got = append(got, it.Path)
	}
	t.Fatalf("no item for %s; have:\n  %s", path, strings.Join(got, "\n  "))
	return Item{}
}

func findMCP(t *testing.T, res Result, name string, scope Scope) Item {
	t.Helper()
	for _, it := range res.Items {
		if it.Category == CategoryMCP && it.Name == name && it.Scope == scope {
			return it
		}
	}
	t.Fatalf("no MCP server %q with scope %s", name, scope)
	return Item{}
}

func absent(t *testing.T, res Result, path string) {
	t.Helper()
	for _, it := range res.Items {
		if it.Path == path {
			t.Fatalf("unexpected item %s", path)
		}
	}
}

func TestEnvProjectRootAndDirs(t *testing.T) {
	f := newFixture(t)
	sub := filepath.Join(f.proj, "a", "b")
	f.mkdir(sub)
	e := f.env(sub)
	if e.ProjectRoot != f.proj {
		t.Fatalf("ProjectRoot = %s, want %s", e.ProjectRoot, f.proj)
	}
	want := []string{f.proj, filepath.Join(f.proj, "a"), sub}
	if got := e.ProjectDirs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ProjectDirs = %v, want %v", got, want)
	}
	anc := e.AncestorDirs()
	if anc[0] != f.ws || anc[len(anc)-1] != "/" {
		t.Fatalf("AncestorDirs = %v", anc)
	}

	// Without a git root, the working directory is the project root.
	plain := filepath.Join(f.root, "plain")
	f.mkdir(plain)
	if e := f.env(plain); e.ProjectRoot != plain {
		t.Fatalf("ProjectRoot without git = %s", e.ProjectRoot)
	}
}

func TestInstructionScopesAndAgents(t *testing.T) {
	f := newFixture(t)
	globalClaude := f.write(filepath.Join(f.home, ".claude", "CLAUDE.md"), "# global")
	globalCodex := f.write(filepath.Join(f.home, ".codex", "AGENTS.md"), "# codex global")
	ancestor := f.write(filepath.Join(f.ws, "CLAUDE.md"), "# workspace")
	ancestorAgents := f.write(filepath.Join(f.ws, "AGENTS.md"), "# workspace agents")
	projClaude := f.write(filepath.Join(f.proj, "CLAUDE.md"), "# project")
	projDotClaude := f.write(filepath.Join(f.proj, ".claude", "CLAUDE.md"), "# project .claude")
	projLocal := f.write(filepath.Join(f.proj, "CLAUDE.local.md"), "# local")
	projAgents := f.write(filepath.Join(f.proj, "AGENTS.md"), "# agents")
	nested := f.write(filepath.Join(f.proj, "web", "CLAUDE.md"), "# web")
	nestedAgents := f.write(filepath.Join(f.proj, "web", "AGENTS.md"), "# web agents")
	f.write(filepath.Join(f.proj, "node_modules", "pkg", "CLAUDE.md"), "# ignored")

	res := f.discover(f.proj)
	cases := []struct {
		path   string
		scope  Scope
		agents []string
	}{
		{globalClaude, ScopeGlobal, []string{"claude"}},
		{globalCodex, ScopeGlobal, []string{"codex"}},
		{ancestor, ScopeAncestor, []string{"claude"}},
		{ancestorAgents, ScopeAncestor, []string{"codex"}},
		{projClaude, ScopeProject, []string{"claude"}},
		{projDotClaude, ScopeProject, []string{"claude"}},
		{projLocal, ScopeProject, []string{"claude"}},
		{projAgents, ScopeProject, []string{"codex", "cursor"}},
		{nested, ScopeProject, []string{"claude"}},
		{nestedAgents, ScopeProject, []string{"codex", "cursor"}},
	}
	for _, c := range cases {
		it := find(t, res, c.path)
		if it.Scope != c.scope || !reflect.DeepEqual(it.Agents, c.agents) || it.Category != CategoryInstruction {
			t.Errorf("%s: got scope=%s agents=%v cat=%s, want scope=%s agents=%v",
				c.path, it.Scope, it.Agents, it.Category, c.scope, c.agents)
		}
	}
	if n := find(t, res, nested).Note; !strings.Contains(n, "nested") {
		t.Errorf("nested note = %q", n)
	}
	if n := find(t, res, ancestorAgents).Note; !strings.Contains(n, "likely not loaded") {
		t.Errorf("ancestor AGENTS.md note = %q", n)
	}
	absent(t, res, filepath.Join(f.proj, "node_modules", "pkg", "CLAUDE.md"))
	if len(res.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", res.Warnings)
	}
}

func TestCwdBelowProjectRoot(t *testing.T) {
	f := newFixture(t)
	root := f.write(filepath.Join(f.proj, "CLAUDE.md"), "root")
	sub := f.write(filepath.Join(f.proj, "svc", "CLAUDE.md"), "svc")
	deeper := f.write(filepath.Join(f.proj, "svc", "api", "CLAUDE.md"), "api")

	res := f.discover(filepath.Join(f.proj, "svc"))
	if it := find(t, res, root); it.Scope != ScopeProject || it.Note != "" {
		t.Errorf("root: %+v", it)
	}
	// svc/CLAUDE.md is on the path to cwd: loaded at start, not "nested".
	if it := find(t, res, sub); it.Scope != ScopeProject || it.Note != "" {
		t.Errorf("svc: %+v", it)
	}
	if it := find(t, res, deeper); !strings.Contains(it.Note, "nested") {
		t.Errorf("api: %+v", it)
	}
	if got := find(t, res, root).Name; got != "CLAUDE.md" {
		t.Errorf("Name = %q, want project-relative", got)
	}
}

func TestSymlinkedSkillsAreMergedAcrossAgents(t *testing.T) {
	f := newFixture(t)
	shared := filepath.Join(f.root, "shared", "deploy")
	f.write(filepath.Join(shared, "SKILL.md"), "---\nname: deploy\n---\n# Deploy")
	f.write(filepath.Join(shared, "scripts", "run.sh"), "echo hi")
	claudeLink := filepath.Join(f.home, ".claude", "skills", "deploy")
	codexLink := filepath.Join(f.home, ".codex", "skills", "deploy")
	agentsLink := filepath.Join(f.home, ".agents", "skills", "deploy")
	f.symlink(shared, claudeLink)
	f.symlink(shared, codexLink)
	f.symlink(shared, agentsLink)

	res := f.discover(f.proj)
	var skills []Item
	for _, it := range res.Items {
		if it.Category == CategorySkill {
			skills = append(skills, it)
		}
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1 merged: %+v", len(skills), skills)
	}
	s := skills[0]
	if s.Path != claudeLink || !reflect.DeepEqual(s.Agents, []string{"claude", "codex"}) || len(s.Aliases) != 2 {
		t.Fatalf("merged skill = path %s agents %v aliases %v", s.Path, s.Agents, s.Aliases)
	}
	if s.Target != shared || s.Scope != ScopeGlobal {
		t.Fatalf("target=%s scope=%s", s.Target, s.Scope)
	}
	// Children: scripts/ dir (with run.sh) first, then SKILL.md.
	if len(s.Children) != 2 || s.Children[0].Name != "scripts/" || s.Children[1].Name != "SKILL.md" {
		t.Fatalf("children = %+v", s.Children)
	}
}

func TestSkillsNestedInGroupFolders(t *testing.T) {
	f := newFixture(t)
	f.write(filepath.Join(f.home, ".claude", "skills", "synced", "bucket-1", "pdf", "SKILL.md"), "# pdf")
	f.write(filepath.Join(f.proj, ".claude", "skills", "local-skill", "SKILL.md"), "# local")
	f.write(filepath.Join(f.proj, ".claude", "skills", "not-a-skill", "README.md"), "nope")
	res := f.discover(f.proj)
	if it := find(t, res, filepath.Join(f.home, ".claude", "skills", "synced", "bucket-1", "pdf")); it.Name != "pdf" || it.Scope != ScopeGlobal {
		t.Errorf("synced skill: %+v", it)
	}
	if it := find(t, res, filepath.Join(f.proj, ".claude", "skills", "local-skill")); it.Scope != ScopeProject {
		t.Errorf("project skill: %+v", it)
	}
	absent(t, res, filepath.Join(f.proj, ".claude", "skills", "not-a-skill"))
}

func TestClaudePluginSkillsAndMCP(t *testing.T) {
	f := newFixture(t)
	install := filepath.Join(f.home, ".claude", "plugins", "cache", "mkt", "helper", "1.0.0")
	f.write(filepath.Join(install, "skills", "review", "SKILL.md"), "# review")
	f.write(filepath.Join(install, ".mcp.json"), `{"mcpServers":{"helper-db":{"command":"db","env":{"DB_PASSWORD":"hunter2"}}}}`)
	other := filepath.Join(f.home, ".claude", "plugins", "cache", "mkt", "other", "1.0.0")
	f.write(filepath.Join(other, "skills", "elsewhere", "SKILL.md"), "# other project")
	f.write(filepath.Join(f.home, ".claude", "plugins", "installed_plugins.json"), `{"version":2,"plugins":{
		"helper@mkt":[{"scope":"user","installPath":"`+install+`"}],
		"other@mkt":[{"scope":"project","projectPath":"/somewhere/else","installPath":"`+other+`"}]}}`)
	f.write(filepath.Join(f.home, ".claude", "settings.json"), `{"enabledPlugins":{"helper@mkt":false}}`)

	res := f.discover(f.proj)
	sk := find(t, res, filepath.Join(install, "skills", "review"))
	if sk.Scope != ScopeGlobal || sk.Note != "plugin helper (disabled in settings)" {
		t.Errorf("plugin skill: scope=%s note=%q", sk.Scope, sk.Note)
	}
	if m := findMCP(t, res, "helper-db", ScopeGlobal); m.Config["env"].(map[string]any)["DB_PASSWORD"] != Masked {
		t.Errorf("plugin MCP env not masked: %v", m.Config)
	}
	absent(t, res, filepath.Join(other, "skills", "elsewhere"))
}

func TestCodexOverrideShadowsAgentsMD(t *testing.T) {
	f := newFixture(t)
	over := f.write(filepath.Join(f.proj, "AGENTS.override.md"), "override")
	base := f.write(filepath.Join(f.proj, "AGENTS.md"), "base")
	res := f.discover(f.proj, AgentCodex)
	if n := find(t, res, over).Note; !strings.Contains(n, "overrides") {
		t.Errorf("override note = %q", n)
	}
	if n := find(t, res, base).Note; !strings.Contains(n, "shadowed") {
		t.Errorf("base note = %q", n)
	}
}

func TestRules(t *testing.T) {
	f := newFixture(t)
	f.write(filepath.Join(f.proj, ".cursor", "rules", "coding.mdc"), "---\nglobs: *.ts\n---\nUse TS")
	f.write(filepath.Join(f.proj, ".cursor", "rules", "frontend", "react.mdc"), "React rules")
	f.write(filepath.Join(f.proj, ".cursor", "rules", "notes.txt"), "ignored")
	f.mkdir(filepath.Join(f.proj, ".cursor", "rules", "empty"))
	legacy := f.write(filepath.Join(f.proj, ".cursorrules"), "legacy")
	f.write(filepath.Join(f.proj, ".claude", "rules", "testing.md"), "test rules")
	f.write(filepath.Join(f.home, ".codex", "rules", "default.rules"), `prefix_rule(pattern=["git"])`)
	f.write(filepath.Join(f.proj, "pkg", ".cursor", "rules", "pkg.mdc"), "nested rule")

	res := f.discover(f.proj)
	cursor := find(t, res, filepath.Join(f.proj, ".cursor", "rules"))
	if !cursor.IsDir || cursor.Category != CategoryRule || cursor.Name != ".cursor/rules/" {
		t.Fatalf("cursor rules: %+v", cursor)
	}
	var names []string
	for _, c := range cursor.Children {
		names = append(names, c.Name)
	}
	if !reflect.DeepEqual(names, []string{"frontend/", "coding.mdc"}) {
		t.Errorf("cursor rule children = %v (empty dirs and .txt should be dropped)", names)
	}
	if it := find(t, res, legacy); it.Category != CategoryRule {
		t.Errorf(".cursorrules: %+v", it)
	}
	find(t, res, filepath.Join(f.proj, ".claude", "rules"))
	if it := find(t, res, filepath.Join(f.home, ".codex", "rules")); it.Scope != ScopeGlobal {
		t.Errorf("codex rules: %+v", it)
	}
	if it := find(t, res, filepath.Join(f.proj, "pkg", ".cursor", "rules")); !strings.Contains(it.Note, "nested") {
		t.Errorf("nested cursor rules: %+v", it)
	}
}

func TestMCPDiscovery(t *testing.T) {
	f := newFixture(t)
	f.write(filepath.Join(f.proj, ".mcp.json"), `{"mcpServers":{
		"github":{"command":"gh-mcp","env":{"GITHUB_TOKEN":"ghp_abcdefghijklmnopqrstuvwxyz"}},
		"fs":{"command":"npx","args":["-y","@mcp/fs","."]}}}`)
	f.write(filepath.Join(f.home, ".claude.json"), `{
		"oauthAccount":{"emailAddress":"x@example.com"},
		"mcpServers":{"linear":{"type":"http","url":"https://mcp.linear.app/sse","headers":{"Authorization":"Bearer abc"}}},
		"projects":{"`+f.proj+`":{"mcpServers":{"local-db":{"command":"pg","args":["--password","s3cret"]}}},
		            "/other":{"mcpServers":{"not-mine":{"command":"x"}}}}}`)
	f.write(filepath.Join(f.home, ".codex", "config.toml"), `
model = "o3"
[mcp_servers.docs]
command = "docs-mcp"
env = { API_KEY = "sk-1234567890abcdef" }
`)
	f.write(filepath.Join(f.proj, ".cursor", "mcp.json"), `{"mcpServers":{"browser":{"url":"http://localhost:3000/mcp?token=abc&x=1"}}}`)

	res := f.discover(f.proj)
	gh := findMCP(t, res, "github", ScopeProject)
	if gh.Path != filepath.Join(f.proj, ".mcp.json") || !gh.Virtual || gh.Config["env"].(map[string]any)["GITHUB_TOKEN"] != Masked {
		t.Errorf("github: %+v", gh)
	}
	if args := findMCP(t, res, "fs", ScopeProject).Config["args"].([]any); args[1] != "@mcp/fs" {
		t.Errorf("fs args changed: %v", args)
	}
	lin := findMCP(t, res, "linear", ScopeGlobal)
	if lin.Config["headers"].(map[string]any)["Authorization"] != Masked || lin.Config["url"] != "https://mcp.linear.app/sse" {
		t.Errorf("linear: %v", lin.Config)
	}
	local := findMCP(t, res, "local-db", ScopeProject)
	if !strings.Contains(local.Note, "local scope") || local.Config["args"].([]any)[1] != Masked {
		t.Errorf("local-db: %+v", local)
	}
	for _, it := range res.Items {
		if it.Name == "not-mine" {
			t.Error("MCP server from another project leaked in")
		}
	}
	docs := findMCP(t, res, "docs", ScopeGlobal)
	if docs.Agents[0] != AgentCodex || docs.Config["env"].(map[string]any)["API_KEY"] != Masked {
		t.Errorf("codex docs: %+v", docs)
	}
	br := findMCP(t, res, "browser", ScopeProject)
	if u := br.Config["url"].(string); strings.Contains(u, "abc") || !strings.Contains(u, "x=1") {
		t.Errorf("cursor url not masked: %s", u)
	}
}

func TestEmptyEnvironment(t *testing.T) {
	f := newFixture(t)
	res := f.discover(f.proj)
	if len(res.Items) != 0 || len(res.Warnings) != 0 {
		t.Fatalf("expected nothing, got %d items, warnings %v", len(res.Items), res.Warnings)
	}
	// Home directory that doesn't exist at all.
	e := NewEnvWith(f.proj, filepath.Join(f.root, "nope"), func(string) string { return "" })
	if res := Discover(e, nil); len(res.Items) != 0 || len(res.Warnings) != 0 {
		t.Fatalf("missing home: %+v", res)
	}
}

func TestMalformedAndUnreadableConfigAreWarnings(t *testing.T) {
	f := newFixture(t)
	f.write(filepath.Join(f.proj, ".mcp.json"), `{not json`)
	f.write(filepath.Join(f.home, ".codex", "config.toml"), `[[[`)
	f.write(filepath.Join(f.proj, "CLAUDE.md"), "fine")
	locked := filepath.Join(f.home, ".claude", "skills")
	f.write(filepath.Join(locked, "x", "SKILL.md"), "x")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })

	res := f.discover(f.proj)
	find(t, res, filepath.Join(f.proj, "CLAUDE.md"))
	want := 3
	if os.Geteuid() == 0 {
		want = 2 // root can read the locked dir
	}
	if len(res.Warnings) != want {
		t.Fatalf("warnings = %+v, want %d", res.Warnings, want)
	}
}

func TestAgentFilter(t *testing.T) {
	f := newFixture(t)
	f.write(filepath.Join(f.proj, "CLAUDE.md"), "c")
	agents := f.write(filepath.Join(f.proj, "AGENTS.md"), "a")
	f.write(filepath.Join(f.proj, "web", "CLAUDE.md"), "nested c")

	res := f.discover(f.proj, AgentCursor)
	if len(res.Items) != 1 || res.Items[0].Path != agents || !reflect.DeepEqual(res.Items[0].Agents, []string{"cursor"}) {
		t.Fatalf("cursor-only: %+v", res.Items)
	}
}

func TestHomeAsProjectKeepsGlobalScope(t *testing.T) {
	f := newFixture(t)
	g := f.write(filepath.Join(f.home, ".claude", "CLAUDE.md"), "g")
	f.write(filepath.Join(f.home, ".claude", "skills", "s", "SKILL.md"), "s")
	res := f.discover(f.home)
	if it := find(t, res, g); it.Scope != ScopeGlobal {
		t.Errorf("CLAUDE.md scope = %s", it.Scope)
	}
	if it := find(t, res, filepath.Join(f.home, ".claude", "skills", "s")); it.Scope != ScopeGlobal {
		t.Errorf("skill scope = %s", it.Scope)
	}
}

// TestDiscoveryIsReadOnly snapshots every file before and after discovery.
func TestDiscoveryIsReadOnly(t *testing.T) {
	f := newFixture(t)
	f.write(filepath.Join(f.proj, "CLAUDE.md"), "c")
	f.write(filepath.Join(f.proj, ".mcp.json"), `{"mcpServers":{"a":{"command":"a"}}}`)
	f.write(filepath.Join(f.home, ".claude.json"), `{"mcpServers":{"b":{"command":"b"}}}`)
	f.write(filepath.Join(f.home, ".claude", "skills", "s", "SKILL.md"), "s")
	f.write(filepath.Join(f.proj, ".cursor", "rules", "r.mdc"), "r")

	before := snapshot(t, f.root)
	time.Sleep(10 * time.Millisecond)
	Discover(f.env(f.proj), nil)
	if after := snapshot(t, f.root); !reflect.DeepEqual(before, after) {
		t.Fatalf("filesystem changed during discovery")
	}
}

func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		fi, _ := d.Info()
		data := ""
		if d.Type().IsRegular() {
			b, _ := os.ReadFile(p)
			data = string(b)
		}
		out[p] = fi.ModTime().String() + "|" + fi.Mode().String() + "|" + data
		return nil
	})
	return out
}
