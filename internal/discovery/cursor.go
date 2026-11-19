package discovery

import "path/filepath"

func discoverCursor(c *collector) {
	e := c.env
	c.file(filepath.Join(e.ProjectRoot, "AGENTS.md"), CategoryInstruction, ScopeProject, "", AgentCursor)
	c.file(filepath.Join(e.ProjectRoot, ".cursorrules"), CategoryRule, ScopeProject, "legacy rules file", AgentCursor)
	for _, d := range e.ProjectDirs() {
		c.ruleDir(filepath.Join(d, ".cursor", "rules"), []string{".mdc", ".md"}, ScopeProject, "", AgentCursor)
		c.skills(filepath.Join(d, ".cursor", "skills"), ScopeProject, "", AgentCursor)
	}
	c.skills(filepath.Join(e.CursorDir, "skills"), ScopeGlobal, "", AgentCursor)

	c.fileDir(filepath.Join(e.ProjectRoot, ".cursor", "commands"), "", CategoryCommand, mdExts, ScopeProject, "", AgentCursor)
	c.fileDir(filepath.Join(e.CursorDir, "commands"), "", CategoryCommand, mdExts, ScopeGlobal, "", AgentCursor)

	c.hooksFile(filepath.Join(e.ProjectRoot, ".cursor", "hooks.json"), ScopeProject, "", "", AgentCursor)
	c.hooksFile(filepath.Join(e.CursorDir, "hooks.json"), ScopeGlobal, "", "", AgentCursor)

	c.mcpJSONFile(filepath.Join(e.ProjectRoot, ".cursor", "mcp.json"), ScopeProject, "", AgentCursor)
	c.mcpJSONFile(filepath.Join(e.CursorDir, "mcp.json"), ScopeGlobal, "", AgentCursor)

	c.info("Cursor system prompt & User Rules", cursorInfo, AgentCursor)
}

const cursorInfo = `# Cursor system prompt & User Rules

- **System prompt.** Cursor's built-in agent instructions are part of the app
  and change between releases.
- **User Rules** (Settings → Rules) apply to every project but are stored in
  Cursor's application settings, not in a file AYCA can read.
- **Memories** that Cursor generates are kept by the app as well.

Check them in Cursor's settings UI.
`
