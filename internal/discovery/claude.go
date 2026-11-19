package discovery

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

func discoverClaude(c *collector) {
	e := c.env
	settings := readClaudeSettings(c)

	// Managed policy: deployed by administrators, applies to every user and
	// project on the machine, and cannot be excluded.
	const policy = "managed policy — applies machine-wide and cannot be overridden"
	c.file(filepath.Join(e.ManagedDir, "CLAUDE.md"), CategoryInstruction, ScopeSystem, policy, AgentClaude)
	c.hooksFile(filepath.Join(e.ManagedDir, "managed-settings.json"), ScopeSystem, policy, "", AgentClaude)
	c.mcpJSONFile(filepath.Join(e.ManagedDir, "managed-mcp.json"), ScopeSystem, policy, AgentClaude)

	// Global first so that, if the project *is* the home directory, the
	// user-level files keep their global scope.
	c.file(filepath.Join(e.ClaudeDir, "CLAUDE.md"), CategoryInstruction, ScopeGlobal, "user memory", AgentClaude)
	c.ruleDir(filepath.Join(e.ClaudeDir, "rules"), mdExts, ScopeGlobal, "user rules", AgentClaude)

	// Claude Code reads CLAUDE.md files from the working directory up to the
	// filesystem root; ancestors are listed nearest-first.
	dirs := append(slices.Clone(e.ProjectDirs()), e.AncestorDirs()...)
	for _, d := range dirs {
		scope := e.scopeOf(d)
		c.file(filepath.Join(d, "CLAUDE.md"), CategoryInstruction, scope, "", AgentClaude)
		c.file(filepath.Join(d, ".claude", "CLAUDE.md"), CategoryInstruction, scope, "", AgentClaude)
		c.file(filepath.Join(d, "CLAUDE.local.md"), CategoryInstruction, scope, "local (usually git-ignored)", AgentClaude)
	}

	c.autoMemory()

	styleNote := ""
	c.outputStyle = settings.outputStyle
	if settings.outputStyle != "" {
		styleNote = "active output style: " + settings.outputStyle
	}
	for _, d := range e.ProjectDirs() {
		cd := filepath.Join(d, ".claude")
		c.ruleDir(filepath.Join(cd, "rules"), mdExts, ScopeProject, "", AgentClaude)
		c.skills(filepath.Join(cd, "skills"), ScopeProject, "", AgentClaude)
		c.fileDir(filepath.Join(cd, "agents"), "", CategoryAgent, mdExts, ScopeProject, "", AgentClaude)
		c.fileDir(filepath.Join(cd, "commands"), "", CategoryCommand, mdExts, ScopeProject, "", AgentClaude)
		c.fileDir(filepath.Join(cd, "output-styles"), "", CategoryOutputStyle, mdExts, ScopeProject, styleNote, AgentClaude)
	}
	c.skills(filepath.Join(e.ClaudeDir, "skills"), ScopeGlobal, "", AgentClaude)
	c.fileDir(filepath.Join(e.ClaudeDir, "agents"), "", CategoryAgent, mdExts, ScopeGlobal, "", AgentClaude)
	c.fileDir(filepath.Join(e.ClaudeDir, "commands"), "", CategoryCommand, mdExts, ScopeGlobal, "", AgentClaude)
	c.fileDir(filepath.Join(e.ClaudeDir, "output-styles"), "", CategoryOutputStyle, mdExts, ScopeGlobal, styleNote, AgentClaude)

	// Hooks from settings files (user, project, local).
	c.hooksFile(filepath.Join(e.ClaudeDir, "settings.json"), ScopeGlobal, "", "", AgentClaude)
	c.hooksFile(filepath.Join(e.ProjectRoot, ".claude", "settings.json"), ScopeProject, "", "", AgentClaude)
	c.hooksFile(filepath.Join(e.ProjectRoot, ".claude", "settings.local.json"), ScopeProject, "local settings", "", AgentClaude)

	claudePlugins(c, settings)

	// MCP: project (.mcp.json, shared), user and local scopes (~/.claude.json).
	c.mcpJSONFile(filepath.Join(e.ProjectRoot, ".mcp.json"), ScopeProject, "project scope (.mcp.json)", AgentClaude)
	doc, _ := c.readJSON(e.ClaudeJSON)
	if servers, ok := doc["mcpServers"].(map[string]any); ok {
		c.mcpServers(e.ClaudeJSON, servers, ScopeGlobal, "user scope", AgentClaude)
	}
	if projects, ok := doc["projects"].(map[string]any); ok {
		for _, key := range uniqueDirs(e.ProjectRoot, e.Cwd) {
			p, _ := projects[key].(map[string]any)
			if servers, ok := p["mcpServers"].(map[string]any); ok {
				c.mcpServers(e.ClaudeJSON, servers, ScopeProject,
					"local scope — private to you, stored in "+e.Tilde(e.ClaudeJSON), AgentClaude)
			}
		}
	}

	claudeRuntime(c, settings, doc)
}

// ---------------------------------------------------------------- settings

type claudeSettings struct {
	enabledPlugins map[string]bool
	outputStyle    string
}

// readClaudeSettings merges user, project and local settings (later wins).
func readClaudeSettings(c *collector) claudeSettings {
	s := claudeSettings{enabledPlugins: map[string]bool{}}
	e := c.env
	files := []string{
		filepath.Join(e.ClaudeDir, "settings.json"),
		filepath.Join(e.ProjectRoot, ".claude", "settings.json"),
		filepath.Join(e.ProjectRoot, ".claude", "settings.local.json"),
		filepath.Join(e.ManagedDir, "managed-settings.json"),
	}
	for _, f := range files {
		doc, ok := c.readJSON(f)
		if !ok {
			continue
		}
		if m, ok := doc["enabledPlugins"].(map[string]any); ok {
			for k, v := range m {
				if b, ok := v.(bool); ok {
					s.enabledPlugins[k] = b
				}
			}
		}
		if v, ok := doc["outputStyle"].(string); ok && v != "" {
			s.outputStyle = v
		}
	}
	return s
}

// ---------------------------------------------------------------- memory

var nonAlnum = regexp.MustCompile(`[^A-Za-z0-9]`)

// claudeProjectKey is how Claude Code names per-project folders under
// ~/.claude/projects: the absolute path with every non-alphanumeric
// character replaced by "-".
func claudeProjectKey(dir string) string { return nonAlnum.ReplaceAllString(dir, "-") }

// autoMemory adds Claude Code's auto memory for this project.
func (c *collector) autoMemory() {
	e := c.env
	for _, d := range uniqueDirs(e.ProjectRoot, e.Cwd) {
		dir := filepath.Join(e.ClaudeDir, "projects", claudeProjectKey(d), "memory")
		fi, ok := c.stat(dir)
		if !ok || !fi.IsDir() {
			continue
		}
		id := realPath(dir)
		children := pruneEmptyDirs(c.listDir(dir, id, CategoryMemory, ScopeProject, []string{AgentClaude}, func(n string) bool {
			return strings.HasSuffix(strings.ToLower(n), ".md")
		}, 0))
		if len(children) == 0 {
			continue
		}
		c.add(Item{
			Name:     "auto memory",
			Path:     dir,
			Category: CategoryMemory,
			Scope:    ScopeProject,
			Agents:   []string{AgentClaude},
			IsDir:    true,
			Note:     "MEMORY.md is loaded into every session for this project; other files are read on demand",
			Children: children,
		})
	}
}

// ---------------------------------------------------------------- plugins

// claudePlugins adds what installed plugins contribute: skills, subagents,
// commands, output styles, hooks and MCP servers.
func claudePlugins(c *collector, settings claudeSettings) {
	e := c.env
	doc, ok := c.readJSON(filepath.Join(e.ClaudeDir, "plugins", "installed_plugins.json"))
	if !ok {
		return
	}
	plugins, _ := doc["plugins"].(map[string]any)
	for _, id := range sortedKeys(plugins) {
		installs, _ := plugins[id].([]any)
		for _, raw := range installs {
			inst, _ := raw.(map[string]any)
			root, _ := inst["installPath"].(string)
			if root == "" {
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
			disabled := false
			if on, known := settings.enabledPlugins[id]; known && !on {
				note += " (disabled in settings)"
				disabled = true
			}
			add := func() {
				c.skills(filepath.Join(root, "skills"), scope, note, AgentClaude)
				c.fileDir(filepath.Join(root, "agents"), name+": agents/", CategoryAgent, mdExts, scope, note, AgentClaude)
				c.fileDir(filepath.Join(root, "commands"), name+": commands/", CategoryCommand, mdExts, scope, note, AgentClaude)
				c.fileDir(filepath.Join(root, "output-styles"), name+": output-styles/", CategoryOutputStyle, mdExts, scope, note, AgentClaude)
				c.hooksFile(filepath.Join(root, "hooks", "hooks.json"), scope, note, root, AgentClaude)
				if manifest, ok := c.readJSON(filepath.Join(root, ".claude-plugin", "plugin.json")); ok {
					if hooks, ok := manifest["hooks"].(map[string]any); ok {
						c.hooks(filepath.Join(root, ".claude-plugin", "plugin.json"), hooks, scope, note, root, AgentClaude)
					}
				}
				c.mcpJSONFile(filepath.Join(root, ".mcp.json"), scope, note, AgentClaude)
			}
			if disabled {
				c.deferring(add)
			} else {
				add()
			}
		}
	}
}

// fileExists reports whether p is a regular file (following symlinks).
func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}
