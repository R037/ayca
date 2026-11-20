package discovery_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/R037/ayca/internal/discovery"
	"github.com/R037/ayca/internal/testfixture"
)

func find(t *testing.T, res discovery.Result, path string) *discovery.Item {
	t.Helper()
	for i := range res.Items {
		it := &res.Items[i]
		if it.Path == path || strings.TrimSuffix(it.Path, "/") == path {
			return it
		}
	}
	t.Fatalf("no item at %s", path)
	return nil
}

func hasFinding(it *discovery.Item, agent, path string, m discovery.Mechanism) bool {
	for _, f := range it.Found {
		if f.Agent == agent && f.Path == path && f.Mechanism == m {
			return true
		}
	}
	return false
}

func run(t *testing.T) (*testfixture.Fixture, discovery.Result) {
	t.Helper()
	f := testfixture.New(t)
	done := make(chan discovery.Result, 1)
	go func() { done <- discovery.Discover(f.Env(f.Project), nil) }()
	select {
	case res := <-done:
		return f, res
	case <-time.After(20 * time.Second):
		t.Fatal("discovery did not terminate (symlink or import cycle?)")
	}
	return nil, discovery.Result{}
}

func TestFixtureFindings(t *testing.T) {
	f, res := run(t)

	t.Run("shared skill", func(t *testing.T) {
		it := find(t, res, f.H(".claude/skills/pdf"))
		if !hasFinding(it, "claude", f.H(".claude/skills/pdf"), discovery.MechUserSkills) {
			t.Errorf("claude finding missing: %+v", it.Found)
		}
		if !hasFinding(it, "codex", f.H(".agents/skills/pdf"), discovery.MechSharedSkills) {
			t.Errorf("codex finding missing: %+v", it.Found)
		}
		if it.HasMechanism("cursor", discovery.MechUserSkills) {
			t.Error("cursor must not find the shared skill")
		}
	})
	t.Run("cursor pdf is separate", func(t *testing.T) {
		it := find(t, res, f.H(".cursor/skills/pdf"))
		if len(it.Agents) != 1 || it.Agents[0] != "cursor" {
			t.Errorf("agents = %v", it.Agents)
		}
		if !it.HasMechanism("cursor", discovery.MechUserSkills) {
			t.Error("want user-skills")
		}
	})
	t.Run("plugin skill", func(t *testing.T) {
		var it *discovery.Item
		for i := range res.Items {
			if res.Items[i].Name == "demo-skill" {
				it = &res.Items[i]
			}
		}
		if it == nil {
			t.Fatal("plugin skill not found")
		}
		if it.Provider() != "plugin:demo" || !it.DisabledFor("claude") || !it.HasMechanism("claude", discovery.MechPluginSkills) {
			t.Errorf("found = %+v", it.Found)
		}
	})
	t.Run("agents override", func(t *testing.T) {
		it := find(t, res, f.P("AGENTS.md"))
		if got := it.ShadowedFor("codex"); got != f.P("AGENTS.override.md") {
			t.Errorf("ShadowedFor(codex) = %q", got)
		}
		if it.ShadowedFor("cursor") != "" {
			t.Error("cursor still reads AGENTS.md")
		}
		if !it.HasMechanism("cursor", discovery.MechDirInstructions) || !it.HasMechanism("codex", discovery.MechDirInstructions) {
			t.Errorf("found = %+v", it.Found)
		}
		ov := find(t, res, f.P("AGENTS.override.md"))
		if ov.ShadowedFor("codex") != "" {
			t.Error("override is not shadowed")
		}
	})
	t.Run("nested", func(t *testing.T) {
		it := find(t, res, f.P("services/api/CLAUDE.md"))
		if !it.HasMechanism("claude", discovery.MechNestedInstructions) {
			t.Errorf("found = %+v", it.Found)
		}
		ag := find(t, res, f.P("services/api/AGENTS.md"))
		if !ag.HasMechanism("codex", discovery.MechNestedInstructions) || !ag.HasMechanism("cursor", discovery.MechNestedInstructions) {
			t.Errorf("found = %+v", ag.Found)
		}
		r := find(t, res, f.P("web/.cursor/rules"))
		if !r.HasMechanism("cursor", discovery.MechNestedRules) {
			t.Errorf("found = %+v", r.Found)
		}
	})
	t.Run("rules, local, managed, memory-less", func(t *testing.T) {
		if !find(t, res, f.P("CLAUDE.local.md")).HasMechanism("claude", discovery.MechLocalInstructions) {
			t.Error("local-instructions")
		}
		if !find(t, res, f.ManagedDir+"/CLAUDE.md").HasMechanism("claude", discovery.MechManagedInstructions) {
			t.Error("managed-instructions")
		}
		if !find(t, res, f.P(".cursor/rules")).HasMechanism("cursor", discovery.MechProjectRules) {
			t.Error("project-rules")
		}
		if !find(t, res, f.H(".codex/rules")).HasMechanism("codex", discovery.MechExecPolicy) {
			t.Error("exec-policy")
		}
		if !find(t, res, f.H(".codex/prompts")).HasMechanism("codex", discovery.MechUserCommands) {
			t.Error("user-commands")
		}
		if !find(t, res, f.H(".claude/CLAUDE.md")).HasMechanism("claude", discovery.MechUserInstructions) {
			t.Error("user-instructions")
		}
		if !find(t, res, f.H(".codex/AGENTS.md")).HasMechanism("codex", discovery.MechUserInstructions) {
			t.Error("codex user-instructions")
		}
	})
	t.Run("ancestor", func(t *testing.T) {
		it := find(t, res, f.Workspace+"/CLAUDE.md")
		if it.Scope != discovery.ScopeAncestor || !it.HasMechanism("claude", discovery.MechDirInstructions) {
			t.Errorf("%+v", it)
		}
	})
	t.Run("imports", func(t *testing.T) {
		root := find(t, res, f.P("CLAUDE.md"))
		if len(root.Children) == 0 {
			t.Fatal("no import children")
		}
		missing := 0
		for _, c := range root.Children {
			if !c.HasMechanism("claude", discovery.MechImport) {
				t.Errorf("child %s lacks import finding: %+v", c.Name, c.Found)
			}
			if c.Missing {
				missing++
			}
		}
		if missing != 1 {
			t.Errorf("want 1 missing import, got %d", missing)
		}
		home := find(t, res, f.H(".claude/CLAUDE.md"))
		if len(home.Children) != 1 || len(home.Children[0].Children) != 0 {
			t.Errorf("import cycle not cut: %+v", home.Children)
		}
	})
	t.Run("hooks and mcp", func(t *testing.T) {
		for _, it := range res.Items {
			switch {
			case it.Category == discovery.CategoryHook && it.Path == f.H(".claude/settings.json"):
				if !it.HasMechanism("claude", discovery.MechUserHooks) {
					t.Errorf("hook: %+v", it.Found)
				}
			case it.Category == discovery.CategoryHook && it.Path == f.P(".claude/settings.json"):
				if !it.HasMechanism("claude", discovery.MechProjectHooks) {
					t.Errorf("hook: %+v", it.Found)
				}
			case it.Category == discovery.CategoryMCP && it.Name == "linear":
				if !it.HasMechanism("claude", discovery.MechUserMCP) {
					t.Errorf("linear: %+v", it.Found)
				}
			case it.Category == discovery.CategoryMCP && it.Name == "db":
				if !it.HasMechanism("claude", discovery.MechLocalMCP) {
					t.Errorf("db: %+v", it.Found)
				}
			case it.Category == discovery.CategoryMCP && it.Name == "filesystem":
				if !it.HasMechanism("claude", discovery.MechProjectMCP) {
					t.Errorf("filesystem: %+v", it.Found)
				}
			}
		}
	})
	t.Run("runtime", func(t *testing.T) {
		want := map[string]discovery.Mechanism{
			"Claude Code system prompt":         discovery.MechBuiltinPrompt,
			"Session environment":               discovery.MechSessionEnv,
			"claude.ai connectors":              discovery.MechConnectors,
			"Launch flags & hook output":        discovery.MechLaunchFlags,
			"Codex system prompt":               discovery.MechBuiltinPrompt,
			"Cursor system prompt & User Rules": discovery.MechAppSettings,
		}
		for _, it := range res.Items {
			if m, ok := want[it.Name]; ok {
				if len(it.Found) == 0 || it.Found[0].Mechanism != m {
					t.Errorf("%s: %+v", it.Name, it.Found)
				}
				delete(want, it.Name)
			}
		}
		if len(want) != 0 {
			t.Errorf("runtime items missing: %v", want)
		}
	})
}

func TestFixtureEveryItemHasFindings(t *testing.T) {
	_, res := run(t)
	known := map[discovery.Mechanism]bool{}
	for _, m := range discovery.AllMechanisms {
		known[m] = true
	}
	for _, it := range res.Items {
		if len(it.Found) == 0 {
			t.Errorf("%s (%s) has no findings", it.Name, it.Category)
		}
		for _, fd := range it.Found {
			if !known[fd.Mechanism] {
				t.Errorf("%s: unknown mechanism %q", it.Name, fd.Mechanism)
			}
			if fd.Mechanism == "" {
				t.Errorf("%s: empty mechanism", it.Name)
			}
		}
	}
}

func TestFixtureSecretsMasked(t *testing.T) {
	_, res := run(t)
	n := 0
	for _, it := range res.Items {
		if it.Category != discovery.CategoryMCP {
			continue
		}
		n++
		b, _ := json.Marshal(it.Config)
		for _, secret := range []string{"sk-ant-api03-FAKE", "hunter2", "ghp_FAKE", "sk-live-FAKE", "sk-search-FAKE"} {
			if strings.Contains(string(b), secret) {
				t.Errorf("%s leaks %q: %s", it.Name, secret, b)
			}
		}
	}
	if n < 5 {
		t.Errorf("expected at least 5 MCP servers, got %d", n)
	}
}

func TestFixtureWithCwdNested(t *testing.T) {
	f := testfixture.New(t)
	env := f.Env(f.Project).WithCwd(f.P("services/api"))
	if env.ProjectRoot != f.Project || env.ManagedDir != f.ManagedDir {
		t.Fatalf("%+v", env)
	}
	res := discovery.Discover(env, nil)
	if len(res.Items) == 0 {
		t.Fatal("no items")
	}
}
