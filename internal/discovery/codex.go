package discovery

import "path/filepath"

func discoverCodex(c *collector) {
	e := c.env
	// AGENTS.override.md takes precedence over AGENTS.md in the same directory.
	agentsFiles := func(dir string, scope Scope, note string) {
		override := filepath.Join(dir, "AGENTS.override.md")
		hasOverride := c.file(override, CategoryInstruction, scope, joinNote(note, "overrides AGENTS.md"), AgentCodex)
		if hasOverride {
			c.deferring(func() {
				c.file(filepath.Join(dir, "AGENTS.md"), CategoryInstruction, scope, joinNote(note, "shadowed by AGENTS.override.md"), AgentCodex)
			})
			return
		}
		c.file(filepath.Join(dir, "AGENTS.md"), CategoryInstruction, scope, note, AgentCodex)
	}
	agentsFiles(e.CodexHome, ScopeGlobal, "")
	for _, d := range e.ProjectDirs() {
		agentsFiles(d, ScopeProject, "")
	}
	c.deferring(func() {
		for _, d := range e.AncestorDirs() {
			agentsFiles(d, ScopeAncestor, "above the project root — Codex reads AGENTS.md from the project root down, so this is likely not loaded")
		}
	})

	c.ruleDir(filepath.Join(e.CodexHome, "rules"), []string{".rules"}, ScopeGlobal, "execution policy rules", AgentCodex)

	for _, d := range e.ProjectDirs() {
		c.skills(filepath.Join(d, ".agents", "skills"), ScopeProject, "", AgentCodex)
		c.skills(filepath.Join(d, ".codex", "skills"), ScopeProject, "", AgentCodex)
	}
	c.skills(filepath.Join(e.AgentsDir, "skills"), ScopeGlobal, "", AgentCodex)
	c.skills(filepath.Join(e.CodexHome, "skills"), ScopeGlobal, "", AgentCodex)

	c.fileDir(filepath.Join(e.CodexHome, "prompts"), "", CategoryCommand, mdExts, ScopeGlobal, "custom prompts (/prompts:<name>)", AgentCodex)
	c.hooksFile(filepath.Join(e.CodexHome, "hooks.json"), ScopeGlobal, "", "", AgentCodex)

	c.mcpTOMLFile(filepath.Join(e.CodexHome, "config.toml"), ScopeGlobal, "", AgentCodex)
	c.mcpTOMLFile(filepath.Join(e.ProjectRoot, ".codex", "config.toml"), ScopeProject, "project config — only applied for trusted projects", AgentCodex)

	c.info("Codex system prompt", codexSystemPromptInfo, AgentCodex)
}

const codexSystemPromptInfo = `# Codex system prompt

Codex sends its own base instructions with every request. They are built into
the Codex CLI and vary by model and version, so they aren't stored where AYCA
can read them.

They may be adjusted or extended by:

- ` + "`instructions`, `developer_instructions` or `model_instructions_file`" + ` in
  ` + "`config.toml`" + ` (look in the file itself, not shown here)
- command-line flags and profiles chosen at launch (` + "`-c key=value`, `--profile`" + `)
- the session environment Codex adds itself (working directory, sandbox and
  approval policy, shell)
`
