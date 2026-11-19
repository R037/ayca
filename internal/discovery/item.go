// Package discovery finds the context (instructions, rules, skills, MCP
// servers) that AI coding agents may read for a project. It is strictly
// read-only: nothing in this package writes to the filesystem.
package discovery

// Category is the kind of context an item represents.
type Category string

const (
	CategoryInstruction Category = "instruction"
	CategoryRule        Category = "rule"
	CategorySkill       Category = "skill"
	CategoryMCP         Category = "mcp"
)

// Categories in display order.
var Categories = []Category{CategoryInstruction, CategoryRule, CategorySkill, CategoryMCP}

// Scope says where an item lives relative to the project.
type Scope string

const (
	ScopeProject  Scope = "project"  // inside the project root (incl. nested subdirectories)
	ScopeAncestor Scope = "ancestor" // in a directory above the project root
	ScopeGlobal   Scope = "global"   // user-wide configuration (~/.claude, ~/.codex, ...)
)

// Scopes in display order.
var Scopes = []Scope{ScopeProject, ScopeAncestor, ScopeGlobal}

// Agent identifiers.
const (
	AgentClaude = "claude"
	AgentCodex  = "codex"
	AgentCursor = "cursor"
)

// AllAgents lists the supported agents in display order.
var AllAgents = []string{AgentClaude, AgentCodex, AgentCursor}

// Item is one discovered piece of context. Items are produced by agent
// adapters and merged by the registry.
type Item struct {
	ID       string
	Name     string   // display name (relative or ~-shortened path, or server name)
	Path     string   // absolute path on disk; for MCP servers, the config file
	Aliases  []string // other paths that resolved to the same file (symlinks, shared dirs)
	Target   string   // resolved real path when Path is (or goes through) a symlink
	Category Category
	Scope    Scope
	Agents   []string // agents known to read this location
	IsDir    bool
	Virtual  bool // not a standalone file (e.g. an MCP server inside a config file)
	Note     string
	// Config holds the (secret-masked) configuration of a virtual MCP item.
	Config   map[string]any
	Children []Item
}

// Warning is a non-fatal problem encountered during discovery.
type Warning struct {
	Path string
	Err  string
}

// Result is the outcome of a discovery run.
type Result struct {
	Env      Env
	Items    []Item
	Warnings []Warning
}
