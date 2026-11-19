package tree

import (
	"fmt"
	"slices"
	"strings"

	"github.com/R037/ayca/internal/discovery"
)

// contextCategories are the categories listed under Global/Project/Nested.
var contextCategories = []discovery.Category{
	discovery.CategoryInstruction, discovery.CategoryMemory, discovery.CategoryRule,
	discovery.CategorySkill, discovery.CategoryAgent, discovery.CategoryCommand,
	discovery.CategoryOutputStyle,
}

// BuildHierarchy arranges the items one agent reads the way that agent
// layers its context: managed policy, global, project, parent directories,
// nested, plugins, hooks, MCP servers and runtime. Node IDs are prefixed
// "h:<agent>:".
func BuildHierarchy(items []discovery.Item, agent string, env discovery.Env) []*Node {
	prefix := "h:" + agent + ":"
	var managed, global, project, parents, nested, plugins, hooks, mcp, runtime []*discovery.Item
	for i := range items {
		it := &items[i]
		if !slices.Contains(it.Agents, agent) {
			continue
		}
		isNested := it.HasMechanism(agent, discovery.MechNestedInstructions) || it.HasMechanism(agent, discovery.MechNestedRules)
		switch {
		case it.Category == discovery.CategoryRuntime:
			runtime = append(runtime, it)
		case it.Provider() != "":
			plugins = append(plugins, it)
		case it.Scope == discovery.ScopeSystem:
			managed = append(managed, it)
		case it.Category == discovery.CategoryHook:
			hooks = append(hooks, it)
		case it.Category == discovery.CategoryMCP:
			mcp = append(mcp, it)
		case isNested:
			nested = append(nested, it)
		case it.Scope == discovery.ScopeAncestor:
			parents = append(parents, it)
		case it.Scope == discovery.ScopeGlobal:
			global = append(global, it)
		case it.Scope == discovery.ScopeProject:
			project = append(project, it)
		}
	}

	var roots []*Node
	add := func(n *Node, keepEmpty bool) {
		if len(n.Children) == 0 && !keepEmpty {
			return
		}
		n.Label = fmt.Sprintf("%s (%d)", n.Label, countLeaves(n))
		roots = append(roots, n)
	}
	group := func(key, label string, kids []*Node) *Node {
		return NewGroup(prefix+"grp:"+key, label, helpFor(agent, key), kids...)
	}

	add(group("managed", "Managed policy", byCategory(prefix, "managed", managed, discovery.Categories)), false)
	add(group("global", "Global context "+globalHint(agent, env), byCategory(prefix, "global", global, contextCategories)), true)
	add(group("project", "Project context", byCategory(prefix, "project", project, contextCategories)), true)
	parentLabel := "Parent directories"
	if agent == discovery.AgentCodex {
		parentLabel = "Parent directories (not read by Codex)"
	}
	add(group("parents", parentLabel, byCategory(prefix, "parents", parents, contextCategories)), false)
	add(group("nested", "Nested / conditional context", byCategory(prefix, "nested", nested, contextCategories)), false)
	add(group("plugins", "Plugins", byProvider(prefix, agent, plugins)), false)
	add(group("hooks", "Hooks", byScope(prefix, "hooks", hooks)), true)
	add(group("mcp", "MCP servers", byScope(prefix, "mcp", mcp)), true)
	var rt []*Node
	for _, it := range runtime {
		rt = append(rt, hierItem(prefix, agent, it))
	}
	add(group("runtime", "Runtime context", rt), false)
	return roots
}

// hierItem builds an item node with a disabled / overridden badge.
func hierItem(prefix, agent string, it *discovery.Item) *Node {
	n := ItemNode(prefix, it)
	switch {
	case it.DisabledFor(agent):
		n.Badge = "disabled"
	case it.ShadowedFor(agent) != "":
		n.Badge = "overridden"
	}
	return n
}

func byCategory(prefix, key string, items []*discovery.Item, cats []discovery.Category) []*Node {
	agent := strings.Split(prefix, ":")[1]
	var out []*Node
	for _, cat := range cats {
		var kids []*Node
		for _, it := range items {
			if it.Category == cat {
				kids = append(kids, hierItem(prefix, agent, it))
			}
		}
		if len(kids) > 0 {
			out = append(out, NewGroup(prefix+"grp:"+key+"/"+string(cat),
				fmt.Sprintf("%s (%d)", categoryLabels[cat], len(kids)), "", kids...))
		}
	}
	return out
}

func byScope(prefix, key string, items []*discovery.Item) []*Node {
	agent := strings.Split(prefix, ":")[1]
	var out []*Node
	for _, sc := range discovery.Scopes {
		var kids []*Node
		for _, it := range items {
			if it.Scope == sc {
				kids = append(kids, hierItem(prefix, agent, it))
			}
		}
		if len(kids) > 0 {
			out = append(out, NewGroup(prefix+"grp:"+key+"/"+string(sc),
				fmt.Sprintf("%s (%d)", scopeLabels[sc], len(kids)), "", kids...))
		}
	}
	return out
}

func byProvider(prefix, agent string, items []*discovery.Item) []*Node {
	var order []string
	by := map[string][]*discovery.Item{}
	for _, it := range items {
		p := it.Provider()
		if _, ok := by[p]; !ok {
			order = append(order, p)
		}
		by[p] = append(by[p], it)
	}
	var out []*Node
	for _, p := range order {
		name := strings.TrimPrefix(p, "plugin:")
		disabled := true
		var kids []*Node
		for _, it := range by[p] {
			kids = append(kids, hierItem(prefix, agent, it))
			if !it.DisabledFor(agent) {
				disabled = false
			}
		}
		label := name
		if disabled {
			label += " (disabled)"
		}
		out = append(out, NewGroup(prefix+"grp:plugins/"+name, label,
			"Items contributed by the plugin `"+name+"`.", kids...))
	}
	return out
}

// countLeaves counts item nodes below group nodes.
func countLeaves(n *Node) int {
	c := 0
	for _, k := range n.Children {
		if k.Kind == KindGroup {
			c += countLeaves(k)
		} else {
			c++
		}
	}
	return c
}

func globalHint(agent string, env discovery.Env) string {
	switch agent {
	case discovery.AgentClaude:
		return "(" + env.Tilde(env.ClaudeDir) + ")"
	case discovery.AgentCodex:
		return "(" + env.Tilde(env.CodexHome) + ", " + env.Tilde(env.AgentsDir) + ")"
	case discovery.AgentCursor:
		return "(" + env.Tilde(env.CursorDir) + ")"
	}
	return ""
}

// helpFor returns the per-agent teaching paragraph for a top-level group.
func helpFor(agent, key string) string {
	h := hierarchyHelp[agent][key]
	if h == "" {
		h = hierarchyHelp[""][key]
	}
	return h
}

var hierarchyHelp = map[string]map[string]string{
	"": {
		"hooks":   "Hooks are commands the agent runs on lifecycle events. They are configured in settings files, not read as text. Plugin hooks are listed under Plugins.",
		"mcp":     "MCP servers give the agent extra tools. Their definitions live in config files and are listed by scope. Secrets are masked. Plugin servers are listed under Plugins.",
		"plugins": "Plugins bundle skills, commands, hooks and MCP servers. Items a plugin contributes appear only here, grouped per plugin. Disabled plugins contribute nothing to a session.",
		"runtime": "Context that exists in a session but cannot be read from disk, such as built-in system prompts and connectors.",
	},
	discovery.AgentClaude: {
		"managed": "Managed policy is set by an administrator in a system directory. Claude Code loads it first, and users cannot exclude it. Use it for company-wide rules.",
		"global":  "Global context applies to every project you open with Claude Code. It comes from your Claude config directory: CLAUDE.md, rules, skills, subagents, commands and output styles. Claude loads CLAUDE.md and rules at session start; skills load only their name and description until used. Nothing was found here when this group is empty.",
		"project": "Project context is read from the project root and the working directory. Claude Code loads CLAUDE.md, CLAUDE.local.md, .claude/rules and project skills at session start. Auto memory for this project is also listed here.",
		"parents": "Claude Code walks up from the working directory and also loads CLAUDE.md files in parent directories. They are loaded at session start, so they cost context in every session below them.",
		"nested":  "CLAUDE.md files in subdirectories are not loaded at session start. Claude reads them on demand, when it reads files in that directory. Rules with `paths` globs are loaded in the same way, when matching files are touched.",
		"hooks":   "Hooks are commands Claude Code runs on lifecycle events such as PreToolUse or SessionStart. They come from settings.json files at user, project and local level. Plugin hooks are listed under Plugins.",
		"mcp":     "MCP servers give Claude extra tools. They come from ~/.claude.json (user and local scope) and .mcp.json (project scope). Secrets are masked. Plugin servers are listed under Plugins.",
	},
	discovery.AgentCodex: {
		"managed": "Managed configuration set by an administrator.",
		"global":  "Global context applies to every Codex session. Codex reads AGENTS.md (or AGENTS.override.md) from its home directory, plus prompts, exec-policy rules and skills there. Skills shared with other agents live in ~/.agents/skills. Nothing was found here when this group is empty.",
		"project": "Codex reads AGENTS.md from the project root down to the working directory, and AGENTS.override.md takes precedence over AGENTS.md in the same directory. Project skills are found in .agents/skills. Files are loaded at session start up to a size limit.",
		"parents": "Codex only walks from the project root down to the working directory, so AGENTS.md files in directories above the project are not read. They are listed to show what other tools may pick up.",
		"nested":  "AGENTS.md files in subdirectories below the working directory are not loaded at session start. Depending on the Codex version, the model is told by its built-in instructions to follow them when it works in those directories.",
		"hooks":   "Codex has no hook files that AYCA reads. Anything listed here comes from a shared configuration.",
		"mcp":     "MCP servers for Codex are defined in config.toml under [mcp_servers]. Secrets are masked.",
	},
	discovery.AgentCursor: {
		"managed": "Managed configuration set by an administrator.",
		"global":  "Global context applies to every Cursor project. Cursor keeps user-level skills and MCP servers in ~/.cursor; User Rules live in the app settings and cannot be read from disk. Nothing was found here when this group is empty.",
		"project": "Cursor reads AGENTS.md in the project root and rules in .cursor/rules. A rule is always applied, attached by glob, or picked by the agent from its description. Project skills are found in .cursor/skills and .agents/skills.",
		"parents": "Directories above the project root. Cursor does not walk up to read instruction files, so what is listed here is mostly informational.",
		"nested":  "Nested AGENTS.md and .cursor/rules directories apply when Cursor works with files in that directory. They are not part of the starting context.",
		"hooks":   "Hook configuration for Cursor.",
		"mcp":     "MCP servers for Cursor come from ~/.cursor/mcp.json and .cursor/mcp.json. Secrets are masked.",
	},
}
