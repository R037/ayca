package discovery

import "path/filepath"

func discoverCursor(c *collector) {
	e := c.env
	c.via(MechDirInstructions, func() {
		c.file(filepath.Join(e.ProjectRoot, "AGENTS.md"), CategoryInstruction, ScopeProject, "", AgentCursor)
	})
	c.via(MechLegacyRules, func() {
		c.file(filepath.Join(e.ProjectRoot, ".cursorrules"), CategoryRule, ScopeProject, "legacy rules file", AgentCursor)
	})
	for _, d := range e.ProjectDirs() {
		c.via(MechProjectRules, func() {
			c.ruleDir(filepath.Join(d, ".cursor", "rules"), []string{".mdc", ".md"}, ScopeProject, "", AgentCursor)
		})
		c.via(MechProjectSkills, func() { c.skills(filepath.Join(d, ".cursor", "skills"), ScopeProject, "", AgentCursor) })
	}
	c.via(MechUserSkills, func() { c.skills(filepath.Join(e.CursorDir, "skills"), ScopeGlobal, "", AgentCursor) })

	c.via(MechProjectCommands, func() {
		c.fileDir(filepath.Join(e.ProjectRoot, ".cursor", "commands"), "", CategoryCommand, mdExts, ScopeProject, "", AgentCursor)
	})
	c.via(MechUserCommands, func() {
		c.fileDir(filepath.Join(e.CursorDir, "commands"), "", CategoryCommand, mdExts, ScopeGlobal, "", AgentCursor)
	})

	c.via(MechProjectHooks, func() {
		c.hooksFile(filepath.Join(e.ProjectRoot, ".cursor", "hooks.json"), ScopeProject, "", "", AgentCursor)
	})
	c.via(MechUserHooks, func() { c.hooksFile(filepath.Join(e.CursorDir, "hooks.json"), ScopeGlobal, "", "", AgentCursor) })

	c.via(MechProjectMCP, func() {
		c.mcpJSONFile(filepath.Join(e.ProjectRoot, ".cursor", "mcp.json"), ScopeProject, "", AgentCursor)
	})
	c.via(MechUserMCP, func() { c.mcpJSONFile(filepath.Join(e.CursorDir, "mcp.json"), ScopeGlobal, "", AgentCursor) })

	c.info(MechAppSettings, "Cursor system prompt & User Rules", cursorInfo, AgentCursor)
}

const cursorInfo = `# Cursor system prompt & User Rules

- **System prompt.** Cursor's built-in agent instructions are part of the app
  and change between releases.
- **User Rules** (Settings → Rules) apply to every project but are stored in
  Cursor's application settings, not in a file AYCA can read.
- **Memories** that Cursor generates are kept by the app as well.

Check them in Cursor's settings UI.
`
