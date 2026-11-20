package explain

import "github.com/R037/ayca/internal/discovery"

type m = discovery.Mechanism

// Discovery is the knowledge table: agent -> mechanism -> how that agent
// discovers items found this way. It covers every (agent, mechanism) pair
// the discovery adapters can produce. The wording documents behaviour as of
// effective.RulesBasis.
var Discovery = map[string]map[m]string{
	discovery.AgentClaude: {
		discovery.MechManagedInstructions: "Claude Code reads a managed CLAUDE.md from the system policy directory (for example /etc/claude-code/ on Linux). It is deployed by administrators, applies to every user on the machine and cannot be excluded.",
		discovery.MechManagedHooks:        "Hooks in managed-settings.json in the system policy directory are applied machine-wide by administrators and cannot be overridden by user or project settings.",
		discovery.MechManagedMCP:          "Claude Code reads MCP servers from managed-mcp.json in the system policy directory. Administrators use it to provide servers to every user.",
		discovery.MechUserInstructions:    "Claude Code reads ~/.claude/CLAUDE.md at the start of every session, whatever project you open.",
		discovery.MechDirInstructions:     "Claude Code starts in the working directory and walks up to the filesystem root, reading CLAUDE.md and .claude/CLAUDE.md in every directory on the way.",
		discovery.MechLocalInstructions:   "CLAUDE.local.md is read from the same directories as CLAUDE.md. It is meant for personal notes and is usually git-ignored.",
		discovery.MechNestedInstructions:  "CLAUDE.md files in subdirectories below the working directory are not read at startup. Claude Code reads one when it reads files in that subdirectory.",
		discovery.MechImport:              "Claude Code expands @path references inside CLAUDE.md files inline, following imports up to five levels deep. References inside code spans and code blocks are ignored.",
		discovery.MechAutoMemory:          "Claude Code keeps auto memory per project in ~/.claude/projects/<project>/memory/. MEMORY.md (first 200 lines) is loaded every session; other files are read when needed.",
		discovery.MechUserRules:           "Claude Code reads every Markdown file under ~/.claude/rules/ for all projects. A rule without paths frontmatter loads at launch; one with paths loads when matching files are used.",
		discovery.MechProjectRules:        "Claude Code reads Markdown files under .claude/rules/ in the project. A rule without paths frontmatter loads at launch; one with paths loads when matching files are used.",
		discovery.MechUserSkills:          "Claude Code scans ~/.claude/skills/<name>/SKILL.md. It lists each skill's name and description and loads the full SKILL.md when the skill is used.",
		discovery.MechProjectSkills:       "Claude Code scans .claude/skills/<name>/SKILL.md in the project. It lists each skill's name and description and loads the full SKILL.md when the skill is used.",
		discovery.MechPluginSkills:        "Enabled plugins contribute skills from their skills/ directory. Claude Code lists them like any other skill; a disabled plugin contributes nothing.",
		discovery.MechUserAgents:          "Claude Code reads subagent definitions (Markdown with frontmatter) from ~/.claude/agents/ and lists them so the main agent can delegate to them.",
		discovery.MechProjectAgents:       "Claude Code reads subagent definitions (Markdown with frontmatter) from .claude/agents/ in the project and lists them so the main agent can delegate to them.",
		discovery.MechPluginAgents:        "Enabled plugins contribute subagents from their agents/ directory.",
		discovery.MechUserCommands:        "Claude Code turns each Markdown file in ~/.claude/commands/ into a /name slash command available in every project.",
		discovery.MechProjectCommands:     "Claude Code turns each Markdown file in .claude/commands/ into a /name slash command for this project.",
		discovery.MechPluginCommands:      "Enabled plugins contribute slash commands from their commands/ directory.",
		discovery.MechUserOutputStyles:    "Claude Code reads output styles from ~/.claude/output-styles/. The one named by the outputStyle setting replaces part of the system prompt.",
		discovery.MechProjectOutputStyles: "Claude Code reads output styles from .claude/output-styles/. The one named by the outputStyle setting replaces part of the system prompt.",
		discovery.MechPluginOutputStyles:  "Enabled plugins contribute output styles from their output-styles/ directory.",
		discovery.MechUserHooks:           "Claude Code reads the hooks key of ~/.claude/settings.json and runs matching commands on agent events.",
		discovery.MechProjectHooks:        "Claude Code reads the hooks key of .claude/settings.json in the project (shared with the team) and runs matching commands on agent events.",
		discovery.MechLocalHooks:          "Claude Code reads the hooks key of .claude/settings.local.json, your personal and usually git-ignored project settings.",
		discovery.MechPluginHooks:         "Enabled plugins contribute hooks from hooks/hooks.json or their plugin.json manifest.",
		discovery.MechUserMCP:             "Claude Code reads MCP servers from the mcpServers key of ~/.claude.json (user scope) and offers them in every project.",
		discovery.MechProjectMCP:          "Claude Code reads MCP servers from .mcp.json at the project root, which is shared with the team. You are asked to approve project servers before they run.",
		discovery.MechLocalMCP:            "Claude Code reads per-project MCP servers stored under projects.<path> in ~/.claude.json. They are private to you.",
		discovery.MechPluginMCP:           "Enabled plugins contribute MCP servers from the .mcp.json file in the plugin.",
		discovery.MechBuiltinPrompt:       "Claude Code ships its own system prompt and tool definitions inside the program. They change between versions and are not stored anywhere AYCA can read.",
		discovery.MechSessionEnv:          "Claude Code adds facts about the environment to every session: working directory, platform, date, model and a git status snapshot.",
		discovery.MechConnectors:          "When you are signed in with a claude.ai account, the connectors enabled on that account can be added as MCP servers. They are configured in your account, not on disk.",
		discovery.MechLaunchFlags:         "Flags and environment variables given at launch, and the output of hooks, add context that is only known when Claude Code starts.",
	},
	discovery.AgentCodex: {
		discovery.MechUserInstructions:   "Codex reads AGENTS.md from its home directory ($CODEX_HOME, default ~/.codex) at the start of every session. AGENTS.override.md there takes precedence over AGENTS.md.",
		discovery.MechDirInstructions:    "Codex starts at the project root (the git root) and reads AGENTS.override.md, or AGENTS.md if there is none, in each directory down to the working directory. The combined text is capped (32 KiB by default).",
		discovery.MechNestedInstructions: "Codex does not read AGENTS.md files below the working directory at startup. Its built-in instructions tell the model to follow AGENTS.md files in the directories it works in.",
		discovery.MechExecPolicy:         "Codex reads *.rules files from $CODEX_HOME/rules/. They decide which shell commands may run, need approval or are forbidden, and are enforced by the program.",
		discovery.MechProjectSkills:      "Codex scans .agents/skills and .codex/skills in the project directories for <name>/SKILL.md. It lists the name and description and loads the rest when the skill is used.",
		discovery.MechSharedSkills:       "Codex scans ~/.agents/skills, a location shared between agents, for <name>/SKILL.md. It lists the name and description and loads the rest when the skill is used.",
		discovery.MechUserSkills:         "Codex scans $CODEX_HOME/skills for <name>/SKILL.md. It lists the name and description and loads the rest when the skill is used.",
		discovery.MechUserCommands:       "Codex turns each Markdown file in $CODEX_HOME/prompts/ into a custom prompt you run as /prompts:<name>.",
		discovery.MechUserHooks:          "Codex reads hooks from hooks.json in its home directory and runs matching commands on agent events.",
		discovery.MechUserMCP:            "Codex reads MCP servers from the [mcp_servers.<name>] tables in $CODEX_HOME/config.toml.",
		discovery.MechProjectMCP:         "Codex reads MCP servers from .codex/config.toml in the project, but only applies that file for projects you have marked as trusted.",
		discovery.MechBuiltinPrompt:      "Codex sends its own base instructions with every request. They are built into the CLI, vary by model and version, and are not stored anywhere AYCA can read.",
	},
	discovery.AgentCursor: {
		discovery.MechDirInstructions:    "Cursor reads AGENTS.md at the project root as a plain-Markdown alternative to rules, and includes it in the agent's context.",
		discovery.MechNestedInstructions: "Cursor also reads AGENTS.md files in subdirectories and applies them when the agent works in that directory.",
		discovery.MechLegacyRules:        "Cursor still reads a .cursorrules file at the project root. This format is deprecated in favour of .cursor/rules.",
		discovery.MechProjectRules:       "Cursor reads .cursor/rules/*.mdc and *.md in the project. Frontmatter sets the type: alwaysApply, globs for auto-attached, a description only for Agent Requested, or nothing for manual.",
		discovery.MechNestedRules:        "Cursor reads .cursor/rules in subdirectories and applies those rules when working in that directory.",
		discovery.MechProjectSkills:      "Cursor scans .cursor/skills/<name>/SKILL.md in the project and lists each skill's name and description.",
		discovery.MechUserSkills:         "Cursor scans ~/.cursor/skills/<name>/SKILL.md and lists each skill's name and description.",
		discovery.MechProjectCommands:    "Cursor turns each Markdown file in .cursor/commands/ into a /name command for this project.",
		discovery.MechUserCommands:       "Cursor turns each Markdown file in ~/.cursor/commands/ into a /name command in every project.",
		discovery.MechProjectHooks:       "Cursor reads .cursor/hooks.json in the project and runs matching commands on agent events.",
		discovery.MechUserHooks:          "Cursor reads ~/.cursor/hooks.json and runs matching commands on agent events.",
		discovery.MechProjectMCP:         "Cursor reads MCP servers from .cursor/mcp.json in the project.",
		discovery.MechUserMCP:            "Cursor reads MCP servers from ~/.cursor/mcp.json for every project.",
		discovery.MechAppSettings:        "Cursor's built-in system prompt, User Rules and generated memories live in the app's own settings, not in a file AYCA can read.",
	},
}

// AgentName is the display name of an agent.
func AgentName(agent string) string {
	switch agent {
	case discovery.AgentClaude:
		return "Claude Code"
	case discovery.AgentCodex:
		return "Codex"
	case discovery.AgentCursor:
		return "Cursor"
	}
	return agent
}
