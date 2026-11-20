package discovery

import (
	"fmt"
	"strings"
)

// info adds an explanatory item for context that exists at runtime but has
// no file AYCA can read (built-in prompts, account connectors, ...).
func (c *collector) info(m Mechanism, name, markdown string, agents ...string) {
	c.via(m, func() { c.infoItem(name, markdown, agents) })
}

func (c *collector) infoItem(name, markdown string, agents []string) {
	c.add(Item{
		Name:     name,
		Category: CategoryRuntime,
		Scope:    ScopeSystem,
		Agents:   agents,
		Virtual:  true,
		Info:     markdown,
		Note:     "not inspectable from disk",
	})
}

func claudeRuntime(c *collector, s claudeSettings, claudeJSON map[string]any) {
	style := "default"
	if s.outputStyle != "" {
		style = s.outputStyle
	}
	c.info(MechBuiltinPrompt, "Claude Code system prompt", fmt.Sprintf(`# Claude Code system prompt

Claude Code sends its own system prompt with every request: how to behave, how
to use tools, and the definitions of every built-in tool plus every connected
MCP tool. It ships inside the Claude Code program and changes between
versions, so there's no file to show.

- **Output style:** %s. A custom output style (see *Output styles*) replaces
  the parts of the system prompt about software engineering.
- **Skills, subagents and commands:** their names and descriptions are listed
  in context so Claude can pick them. The full content loads only when one is
  used.
- **Launch flags:** `+"`--system-prompt`, `--append-system-prompt`"+` (and their
  `+"`-file`"+` variants) replace or extend it for a single run.
`, style), AgentClaude)

	c.info(MechSessionEnv, "Session environment", `# Session environment

When a session starts, Claude Code adds facts about the environment, such as:

- the working directory, and whether it's a git repository
- platform, OS version and shell
- today's date and the model in use
- a snapshot of `+"`git status`"+` and recent commits

These are computed fresh for every session. Directories added with
`+"`--add-dir`"+` or `+"`/add-dir`"+` also become accessible. Whether their CLAUDE.md
files load depends on the Claude Code version and settings.
`, AgentClaude)

	var b strings.Builder
	b.WriteString(`# claude.ai connectors

When you sign in to Claude Code with a claude.ai account, the connectors
enabled on that account (Gmail, Google Drive, Notion, Slack, ...) can be added
as MCP servers in every session. Their tools then appear in the system prompt.

They're configured in your claude.ai account, not in a local file, so AYCA
can't list which are active. Check ` + "`/mcp`" + ` inside Claude Code, or
claude.ai → Settings → Connectors.
`)
	if seen, ok := claudeJSON["claudeAiMcpEverConnected"].([]any); ok && len(seen) > 0 {
		b.WriteString("\n## Seen on this machine before\n\nFrom `~/.claude.json`. This is a history list: a connector here may since have been disconnected.\n\n")
		for _, v := range seen {
			if name, ok := v.(string); ok {
				fmt.Fprintf(&b, "- %s\n", name)
			}
		}
	}
	c.info(MechConnectors, "claude.ai connectors", b.String(), AgentClaude)

	c.info(MechLaunchFlags, "Launch flags & hook output", `# Launch flags & hook output

Some context is decided each time Claude Code starts, so AYCA can't see it:

- **Flags:** `+"`--append-system-prompt`, `--system-prompt`, `--mcp-config`, `--strict-mcp-config`, `--settings`, `--agents`, `--plugin-dir`, `--add-dir`"+`
- **Environment variables**, such as `+"`CLAUDE_CONFIG_DIR`"+` (AYCA honours this one
  if it's set in the shell you start AYCA from)
- **Hook output:** SessionStart and UserPromptSubmit hooks print text that's
  added to the context. AYCA shows which hooks exist and the scripts they run
  (see *Hooks*), but never runs them, so it can't show their output.
`, AgentClaude)
}
