package discovery

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// Adapter discovers the context locations of one coding agent.
type Adapter struct {
	Name     string
	discover func(c *collector)
}

// Adapters lists all supported agents. Adding an agent means adding an entry
// here and a discover function.
var Adapters = []Adapter{
	{AgentClaude, discoverClaude},
	{AgentCodex, discoverCodex},
	{AgentCursor, discoverCursor},
}

// Discover runs the adapters for the requested agents (all when empty) and
// returns de-duplicated items. It never modifies the filesystem.
func Discover(env Env, agents []string) Result {
	c := newCollector(env)
	enabled := func(a string) bool { return len(agents) == 0 || slices.Contains(agents, a) }
	for _, a := range Adapters {
		if enabled(a.Name) {
			a.discover(c)
		}
	}
	discoverNested(c, enabled)
	return Result{Env: env, Items: c.items, Warnings: c.warnings}
}

// ---------------------------------------------------------------- Claude Code

func discoverClaude(c *collector) {
	e := c.env
	// Global first so that, if the project *is* the home directory, the
	// user-level files keep their global scope.
	c.file(filepath.Join(e.ClaudeDir, "CLAUDE.md"), CategoryInstruction, ScopeGlobal, "user memory", AgentClaude)
	c.ruleDir(filepath.Join(e.ClaudeDir, "rules"), []string{".md"}, ScopeGlobal, "user rules", AgentClaude)

	// Claude Code reads CLAUDE.md files from the working directory up to the
	// filesystem root; ancestors are listed nearest-first.
	dirs := append(slices.Clone(e.ProjectDirs()), e.AncestorDirs()...)
	for _, d := range dirs {
		scope := e.scopeOf(d)
		c.file(filepath.Join(d, "CLAUDE.md"), CategoryInstruction, scope, "", AgentClaude)
		c.file(filepath.Join(d, ".claude", "CLAUDE.md"), CategoryInstruction, scope, "", AgentClaude)
		c.file(filepath.Join(d, "CLAUDE.local.md"), CategoryInstruction, scope, "local (usually git-ignored)", AgentClaude)
	}
	for _, d := range e.ProjectDirs() {
		c.ruleDir(filepath.Join(d, ".claude", "rules"), []string{".md"}, ScopeProject, "", AgentClaude)
		c.skills(filepath.Join(d, ".claude", "skills"), ScopeProject, "", AgentClaude)
	}

	c.skills(filepath.Join(e.ClaudeDir, "skills"), ScopeGlobal, "", AgentClaude)
	claudePlugins(c)

	// MCP: project (.mcp.json, shared), user and local scopes (~/.claude.json).
	c.mcpJSONFile(filepath.Join(e.ProjectRoot, ".mcp.json"), ScopeProject, "project scope (.mcp.json)", AgentClaude)
	if doc, ok := c.readJSON(e.ClaudeJSON); ok {
		if servers, ok := doc["mcpServers"].(map[string]any); ok {
			c.mcpServers(e.ClaudeJSON, servers, ScopeGlobal, "user scope", AgentClaude)
		}
		if projects, ok := doc["projects"].(map[string]any); ok {
			for _, key := range []string{e.ProjectRoot, e.Cwd} {
				p, _ := projects[key].(map[string]any)
				if servers, ok := p["mcpServers"].(map[string]any); ok {
					c.mcpServers(e.ClaudeJSON, servers, ScopeProject,
						"local scope — private to you, stored in "+e.Tilde(e.ClaudeJSON), AgentClaude)
				}
			}
		}
	}
}

// claudePlugins adds skills and MCP servers shipped by installed plugins.
func claudePlugins(c *collector) {
	e := c.env
	manifest := filepath.Join(e.ClaudeDir, "plugins", "installed_plugins.json")
	doc, ok := c.readJSON(manifest)
	if !ok {
		return
	}
	enabled := claudeEnabledPlugins(c)
	plugins, _ := doc["plugins"].(map[string]any)
	for _, id := range sortedKeys(plugins) {
		installs, _ := plugins[id].([]any)
		for _, raw := range installs {
			inst, _ := raw.(map[string]any)
			path, _ := inst["installPath"].(string)
			if path == "" {
				continue
			}
			scope := ScopeGlobal
			if s, _ := inst["scope"].(string); s == "project" || s == "local" {
				pp, _ := inst["projectPath"].(string)
				if pp != e.ProjectRoot && pp != e.Cwd {
					continue // installed for a different project
				}
				scope = ScopeProject
			}
			name := strings.SplitN(id, "@", 2)[0]
			note := "plugin " + name
			if on, known := enabled[id]; known && !on {
				note += " (disabled in settings)"
			}
			c.skills(filepath.Join(path, "skills"), scope, note, AgentClaude)
			c.mcpJSONFile(filepath.Join(path, ".mcp.json"), scope, note, AgentClaude)
		}
	}
}

// claudeEnabledPlugins reads enabledPlugins from user and project settings.
func claudeEnabledPlugins(c *collector) map[string]bool {
	out := map[string]bool{}
	files := []string{filepath.Join(c.env.ClaudeDir, "settings.json")}
	for _, d := range []string{c.env.ProjectRoot} {
		files = append(files, filepath.Join(d, ".claude", "settings.json"), filepath.Join(d, ".claude", "settings.local.json"))
	}
	for _, f := range files {
		doc, ok := c.readJSON(f)
		if !ok {
			continue
		}
		if m, ok := doc["enabledPlugins"].(map[string]any); ok {
			for k, v := range m {
				if b, ok := v.(bool); ok {
					out[k] = b
				}
			}
		}
	}
	return out
}

// ---------------------------------------------------------------- Codex

func discoverCodex(c *collector) {
	e := c.env
	// AGENTS.override.md takes precedence over AGENTS.md in the same directory.
	agentsFiles := func(dir string, scope Scope, note string) {
		override := filepath.Join(dir, "AGENTS.override.md")
		hasOverride := c.file(override, CategoryInstruction, scope, joinNote(note, "overrides AGENTS.md"), AgentCodex)
		n := note
		if hasOverride {
			n = joinNote(note, "shadowed by AGENTS.override.md")
		}
		c.file(filepath.Join(dir, "AGENTS.md"), CategoryInstruction, scope, n, AgentCodex)
	}
	agentsFiles(e.CodexHome, ScopeGlobal, "")
	for _, d := range e.ProjectDirs() {
		agentsFiles(d, ScopeProject, "")
	}
	for _, d := range e.AncestorDirs() {
		agentsFiles(d, ScopeAncestor, "above the project root — Codex reads AGENTS.md from the project root down, so this is likely not loaded")
	}

	c.ruleDir(filepath.Join(e.CodexHome, "rules"), []string{".rules"}, ScopeGlobal, "execution policy rules", AgentCodex)

	for _, d := range e.ProjectDirs() {
		c.skills(filepath.Join(d, ".agents", "skills"), ScopeProject, "", AgentCodex)
		c.skills(filepath.Join(d, ".codex", "skills"), ScopeProject, "", AgentCodex)
	}
	c.skills(filepath.Join(e.AgentsDir, "skills"), ScopeGlobal, "", AgentCodex)
	c.skills(filepath.Join(e.CodexHome, "skills"), ScopeGlobal, "", AgentCodex)

	c.mcpTOMLFile(filepath.Join(e.CodexHome, "config.toml"), ScopeGlobal, "", AgentCodex)
	c.mcpTOMLFile(filepath.Join(e.ProjectRoot, ".codex", "config.toml"), ScopeProject, "project config — only applied for trusted projects", AgentCodex)
}

// ---------------------------------------------------------------- Cursor

func discoverCursor(c *collector) {
	e := c.env
	c.file(filepath.Join(e.ProjectRoot, "AGENTS.md"), CategoryInstruction, ScopeProject, "", AgentCursor)
	c.file(filepath.Join(e.ProjectRoot, ".cursorrules"), CategoryRule, ScopeProject, "legacy rules file", AgentCursor)
	for _, d := range e.ProjectDirs() {
		c.ruleDir(filepath.Join(d, ".cursor", "rules"), []string{".mdc", ".md"}, ScopeProject, "", AgentCursor)
		c.skills(filepath.Join(d, ".cursor", "skills"), ScopeProject, "", AgentCursor)
	}
	c.skills(filepath.Join(e.CursorDir, "skills"), ScopeGlobal, "", AgentCursor)

	c.mcpJSONFile(filepath.Join(e.ProjectRoot, ".cursor", "mcp.json"), ScopeProject, "", AgentCursor)
	c.mcpJSONFile(filepath.Join(e.CursorDir, "mcp.json"), ScopeGlobal, "", AgentCursor)
}

// ---------------------------------------------------------------- nested

const maxNestedDirs = 4000

// skipDirs are never descended into when looking for nested context.
var skipDirs = map[string]bool{
	"node_modules": true, "vendor": true, "dist": true, "build": true, "target": true,
	"venv": true, "__pycache__": true, "Pods": true, "DerivedData": true,
}

// discoverNested walks below the working directory for context files that
// agents load on demand when they work inside a subdirectory.
func discoverNested(c *collector, enabled func(string) bool) {
	e := c.env
	visited, stopped := 0, false
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if depth > 6 || stopped {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			c.warn(dir, err)
			return
		}
		for _, ent := range entries {
			if !ent.IsDir() { // symlinked dirs are deliberately not followed
				continue
			}
			name := ent.Name()
			if strings.HasPrefix(name, ".") || skipDirs[name] {
				continue
			}
			if visited >= maxNestedDirs {
				c.warnings = append(c.warnings, Warning{Path: e.Cwd, Err: "nested scan stopped after 4000 directories"})
				stopped = true
				return
			}
			visited++
			sub := filepath.Join(dir, name)
			rel, _ := filepath.Rel(e.ProjectRoot, sub)
			note := "nested — loaded on demand when the agent works in " + rel + "/"
			if enabled(AgentClaude) {
				c.file(filepath.Join(sub, "CLAUDE.md"), CategoryInstruction, ScopeProject, note, AgentClaude)
				c.file(filepath.Join(sub, "CLAUDE.local.md"), CategoryInstruction, ScopeProject, note, AgentClaude)
			}
			var agentsReaders []string
			if enabled(AgentCodex) {
				agentsReaders = append(agentsReaders, AgentCodex)
			}
			if enabled(AgentCursor) {
				agentsReaders = append(agentsReaders, AgentCursor)
			}
			if len(agentsReaders) > 0 {
				c.file(filepath.Join(sub, "AGENTS.md"), CategoryInstruction, ScopeProject, note, agentsReaders...)
			}
			if enabled(AgentCursor) {
				c.ruleDir(filepath.Join(sub, ".cursor", "rules"), []string{".mdc", ".md"}, ScopeProject, note, AgentCursor)
			}
			walk(sub, depth+1)
		}
	}
	// Scanning everything below $HOME or / is slow and mostly noise.
	if e.Cwd == e.Home || filepath.Dir(e.Cwd) == e.Cwd {
		return
	}
	walk(e.Cwd, 0)
}

func joinNote(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}

func sortStrings(s []string) { sort.Strings(s) }
