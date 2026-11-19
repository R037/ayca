// Package discovery finds the context (instructions, rules, skills, MCP
// servers) that AI coding agents may read for a project. It is strictly
// read-only: nothing in this package writes to the filesystem.
package discovery

import "slices"

// Category is the kind of context an item represents.
type Category string

const (
	CategoryInstruction Category = "instruction"
	CategoryMemory      Category = "memory"
	CategoryRule        Category = "rule"
	CategorySkill       Category = "skill"
	CategoryAgent       Category = "agent"   // subagent definitions
	CategoryCommand     Category = "command" // slash commands / custom prompts
	CategoryOutputStyle Category = "output-style"
	CategoryHook        Category = "hook"
	CategoryMCP         Category = "mcp"
	CategoryRuntime     Category = "runtime" // context that exists but cannot be read from disk
)

// Categories in display order.
var Categories = []Category{
	CategoryInstruction, CategoryMemory, CategoryRule, CategorySkill, CategoryAgent,
	CategoryCommand, CategoryOutputStyle, CategoryHook, CategoryMCP, CategoryRuntime,
}

// CoreCategories are always shown, even when empty, so users can see they
// were checked. Other categories appear only when something was found.
var CoreCategories = []Category{CategoryInstruction, CategoryRule, CategorySkill, CategoryMCP}

// Scope says where an item lives relative to the project.
type Scope string

const (
	ScopeProject  Scope = "project"  // inside the project root (incl. nested subdirectories)
	ScopeAncestor Scope = "ancestor" // in a directory above the project root
	ScopeGlobal   Scope = "global"   // user-wide configuration (~/.claude, ~/.codex, ...)
	ScopeSystem   Scope = "system"   // machine-wide managed policy, or built into the agent
)

// Scopes in display order.
var Scopes = []Scope{ScopeProject, ScopeAncestor, ScopeGlobal, ScopeSystem}

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
	Missing  bool // referenced (e.g. by an @import) but not found on disk
	// DeferredFor lists the agents for which this item is not part of a
	// session's starting context: nested files loaded on demand, shadowed
	// files, files the agent likely ignores, and disabled plugins.
	DeferredFor []string
	Note        string
	// Info is Markdown explaining context that cannot be read from disk.
	Info string
	// Config holds the (secret-masked) configuration of a virtual item
	// (an MCP server or a hook).
	Config   map[string]any
	Children []Item
	// Found records how each agent finds the item (see Finding).
	Found []Finding
}

// Finding records one way an agent finds an item: the path it looks at and
// the discovery rule involved. A merged item has one Finding per
// (agent, path, mechanism).
type Finding struct {
	Agent      string
	Path       string
	Mechanism  Mechanism
	Provider   string // "plugin:<name>" when contributed by a plugin, else ""
	Disabled   bool   // the providing plugin is disabled in settings
	ShadowedBy string // path of a file that takes precedence for this agent (Codex AGENTS.override.md)
}

// FindingsFor returns the findings of one agent.
func (it *Item) FindingsFor(agent string) []Finding {
	var out []Finding
	for _, f := range it.Found {
		if f.Agent == agent {
			out = append(out, f)
		}
	}
	return out
}

// Provider returns the first non-empty Finding.Provider ("" when none).
func (it *Item) Provider() string {
	for _, f := range it.Found {
		if f.Provider != "" {
			return f.Provider
		}
	}
	return ""
}

// DisabledFor reports whether the agent has findings and all are disabled.
func (it *Item) DisabledFor(agent string) bool {
	fs := it.FindingsFor(agent)
	if len(fs) == 0 {
		return false
	}
	for _, f := range fs {
		if !f.Disabled {
			return false
		}
	}
	return true
}

// ShadowedFor returns the path of the file that shadows the item for agent,
// but only when every finding of that agent is shadowed ("" otherwise).
func (it *Item) ShadowedFor(agent string) string {
	fs := it.FindingsFor(agent)
	if len(fs) == 0 {
		return ""
	}
	for _, f := range fs {
		if f.ShadowedBy == "" {
			return ""
		}
	}
	return fs[0].ShadowedBy
}

// HasMechanism reports whether agent has a finding via mechanism m.
func (it *Item) HasMechanism(agent string, m Mechanism) bool {
	for _, f := range it.Found {
		if f.Agent == agent && f.Mechanism == m {
			return true
		}
	}
	return false
}

// Deferred reports whether the item is not loaded at session start by agent.
func (it *Item) Deferred(agent string) bool { return slices.Contains(it.DeferredFor, agent) }

// DeferredForAll reports whether no agent that reads the item loads it at
// session start.
func (it *Item) DeferredForAll() bool {
	if len(it.Agents) == 0 {
		return false
	}
	for _, a := range it.Agents {
		if !it.Deferred(a) {
			return false
		}
	}
	return true
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
	// OutputStyle is Claude Code's active output style from settings
	// ("" when none is set).
	OutputStyle string
}
