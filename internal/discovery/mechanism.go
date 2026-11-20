package discovery

// Mechanism names the discovery rule that found an item. Keys are
// agent-agnostic; the agent is recorded next to it in a Finding, and the
// explain package maps (agent, mechanism) to an explanation.
type Mechanism string

const (
	MechManagedInstructions Mechanism = "managed-instructions" // Claude managed CLAUDE.md
	MechManagedHooks        Mechanism = "managed-hooks"
	MechManagedMCP          Mechanism = "managed-mcp"
	MechUserInstructions    Mechanism = "user-instructions"   // ~/.claude/CLAUDE.md, $CODEX_HOME/AGENTS(.override).md
	MechDirInstructions     Mechanism = "dir-instructions"    // CLAUDE.md / .claude/CLAUDE.md / AGENTS(.override).md in project and ancestor dirs; Cursor root AGENTS.md
	MechLocalInstructions   Mechanism = "local-instructions"  // CLAUDE.local.md (project/ancestor dirs)
	MechNestedInstructions  Mechanism = "nested-instructions" // below cwd (discoverNested)
	MechImport              Mechanism = "import"              // @path import (children of instruction items)
	MechAutoMemory          Mechanism = "auto-memory"
	MechUserRules           Mechanism = "user-rules"    // ~/.claude/rules
	MechProjectRules        Mechanism = "project-rules" // <dir>/.claude/rules, <dir>/.cursor/rules
	MechNestedRules         Mechanism = "nested-rules"  // nested .cursor/rules
	MechLegacyRules         Mechanism = "legacy-rules"  // .cursorrules
	MechExecPolicy          Mechanism = "exec-policy"   // Codex $CODEX_HOME/rules/*.rules
	MechUserSkills          Mechanism = "user-skills"   // ~/.claude/skills, $CODEX_HOME/skills, ~/.cursor/skills
	MechSharedSkills        Mechanism = "shared-skills" // ~/.agents/skills (Codex)
	MechProjectSkills       Mechanism = "project-skills"
	MechPluginSkills        Mechanism = "plugin-skills"

	MechUserAgents    Mechanism = "user-agents"
	MechProjectAgents Mechanism = "project-agents"
	MechPluginAgents  Mechanism = "plugin-agents"

	MechUserCommands    Mechanism = "user-commands" // incl. Codex prompts
	MechProjectCommands Mechanism = "project-commands"
	MechPluginCommands  Mechanism = "plugin-commands"

	MechUserOutputStyles    Mechanism = "user-output-styles"
	MechProjectOutputStyles Mechanism = "project-output-styles"
	MechPluginOutputStyles  Mechanism = "plugin-output-styles"

	MechUserHooks    Mechanism = "user-hooks"
	MechProjectHooks Mechanism = "project-hooks"
	MechLocalHooks   Mechanism = "local-hooks"
	MechPluginHooks  Mechanism = "plugin-hooks"

	MechUserMCP    Mechanism = "user-mcp"
	MechProjectMCP Mechanism = "project-mcp"
	MechLocalMCP   Mechanism = "local-mcp"
	MechPluginMCP  Mechanism = "plugin-mcp"

	MechBuiltinPrompt Mechanism = "builtin-prompt" // Claude Code / Codex system prompt
	MechSessionEnv    Mechanism = "session-env"    // Claude session environment
	MechConnectors    Mechanism = "connectors"     // claude.ai connectors
	MechLaunchFlags   Mechanism = "launch-flags"   // launch flags & hook output
	MechAppSettings   Mechanism = "app-settings"   // Cursor system prompt & User Rules
)

// AllMechanisms lists every mechanism (used by completeness tests).
var AllMechanisms = []Mechanism{
	MechManagedInstructions, MechManagedHooks, MechManagedMCP,
	MechUserInstructions, MechDirInstructions, MechLocalInstructions, MechNestedInstructions,
	MechImport, MechAutoMemory,
	MechUserRules, MechProjectRules, MechNestedRules, MechLegacyRules, MechExecPolicy,
	MechUserSkills, MechSharedSkills, MechProjectSkills, MechPluginSkills,
	MechUserAgents, MechProjectAgents, MechPluginAgents,
	MechUserCommands, MechProjectCommands, MechPluginCommands,
	MechUserOutputStyles, MechProjectOutputStyles, MechPluginOutputStyles,
	MechUserHooks, MechProjectHooks, MechLocalHooks, MechPluginHooks,
	MechUserMCP, MechProjectMCP, MechLocalMCP, MechPluginMCP,
	MechBuiltinPrompt, MechSessionEnv, MechConnectors, MechLaunchFlags, MechAppSettings,
}
