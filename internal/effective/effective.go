// Package effective classifies how each AI coding agent loads a discovered
// item (at startup, listed, on demand, ...) and projects the discovered
// context onto a working directory. It is rule based and read-only: it
// documents how the agents are known to behave and never claims what a
// running session actually received.
package effective

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/R037/ayca/internal/discovery"
	fmatter "github.com/R037/ayca/internal/frontmatter"
)

// RulesBasis names the versions and sources the loading rules are based on.
const RulesBasis = "Claude Code 2.1.328 and Codex CLI 0.167.1 (installed versions); Cursor is not installed, so its rules come from the Cursor documentation"

// State is how an agent loads an item.
type State string

const (
	Startup     State = "startup"     // always loaded at session start
	Listed      State = "listed"      // name/description at startup, full content when invoked/used
	OnDemand    State = "on-demand"   // loaded when the agent works in a path / follows a pointer
	Conditional State = "conditional" // applies when a condition matches (file globs, nested dir)
	Invocation  State = "invocation"  // only when you explicitly invoke it
	Event       State = "event"       // runs on agent events (hooks), not context itself
	Enforced    State = "enforced"    // applied by the agent program, not sent to the model
	Disabled    State = "disabled"
	Shadowed    State = "shadowed"
	NotLoaded   State = "not-loaded" // found, but this agent likely ignores it here
	Unknown     State = "unknown"    // runtime-dependent
)

// Label is a short human label.
func (s State) Label() string {
	switch s {
	case Startup:
		return "Startup"
	case Listed:
		return "Listed (name + description)"
	case OnDemand:
		return "On demand"
	case Conditional:
		return "Conditional"
	case Invocation:
		return "Only when invoked"
	case Event:
		return "Runs on events"
	case Enforced:
		return "Enforced by the program"
	case Disabled:
		return "Disabled"
	case Shadowed:
		return "Shadowed"
	case NotLoaded:
		return "Not loaded"
	default:
		return "Unknown"
	}
}

// Describe is a one-sentence explanation of the state.
func (s State) Describe() string {
	switch s {
	case Startup:
		return "Loaded into the agent's context at the start of every session."
	case Listed:
		return "Only the name and description take up context at startup; the full content loads when it is invoked or used."
	case OnDemand:
		return "Not loaded at startup; loaded when the agent works in a matching path or follows a pointer to it."
	case Conditional:
		return "Applies only when a condition (file patterns or a nested directory) matches."
	case Invocation:
		return "Loaded only when you explicitly invoke it."
	case Event:
		return "Runs when an agent event fires; it is not context itself, though its output may be."
	case Enforced:
		return "Applied by the agent program itself and not sent to the model."
	case Disabled:
		return "Found on disk, but switched off in settings."
	case Shadowed:
		return "Found on disk, but another file takes precedence over it."
	case NotLoaded:
		return "Found on disk, but this agent is not expected to load it here."
	default:
		return "Whether and how this is loaded depends on the runtime and cannot be told from disk."
	}
}

// Confidence says how firmly a rule is established.
type Confidence string

const (
	ConfDocumented       Confidence = "documented"
	ConfVersionDependent Confidence = "version-dependent"
	ConfRuntimeDependent Confidence = "runtime-dependent"
)

// Load is the classification of one item for one agent.
type Load struct {
	Agent      string
	State      State
	Reason     string
	Confidence Confidence
}

// Context carries what classification needs besides the item.
type Context struct {
	Env         discovery.Env
	OutputStyle string
}

// ContextOf builds a Context from a discovery result.
func ContextOf(res discovery.Result) Context {
	return Context{Env: res.Env, OutputStyle: res.OutputStyle}
}

// Classify classifies a single top-level item.
func Classify(it *discovery.Item, agent string, ctx Context) Load {
	return ClassifyPath([]*discovery.Item{it}, agent, ctx)
}

func ld(agent string, s State, c Confidence, format string, a ...any) Load {
	return Load{Agent: agent, State: s, Reason: fmt.Sprintf(format, a...), Confidence: c}
}

// ClassifyPath classifies the last item of chain (ancestors first, e.g.
// [skillDir, SKILL.md] or [CLAUDE.md, @import, @nested-import]) for agent.
func ClassifyPath(chain []*discovery.Item, agent string, ctx Context) Load {
	if len(chain) == 0 {
		return ld(agent, Unknown, ConfRuntimeDependent, "no item given")
	}
	last, top := chain[len(chain)-1], chain[0]

	if last.Missing {
		return ld(agent, NotLoaded, ConfDocumented, "file not found")
	}
	for _, it := range chain {
		if it.DisabledFor(agent) {
			if p := providerName(it); p != "" {
				return ld(agent, Disabled, ConfDocumented, "Provided by %s, which is disabled in settings", p)
			}
			return ld(agent, Disabled, ConfDocumented, "Disabled in settings")
		}
		if sh := it.ShadowedFor(agent); sh != "" {
			return ld(agent, Shadowed, ConfDocumented, "Shadowed by %s, which takes precedence", displayPath(ctx.Env, sh))
		}
	}

	fs := top.FindingsFor(agent)
	if len(fs) == 0 && !slices.Contains(top.Agents, agent) {
		return ld(agent, NotLoaded, ConfDocumented, "%s does not read this location", agentName(agent))
	}
	if len(chain) > 1 && isImport(chain, agent) {
		parent := ClassifyPath(chain[:len(chain)-1], agent, ctx)
		parent.Reason = fmt.Sprintf("imported into %s (expanded inline, up to 5 levels)", chain[len(chain)-2].Name)
		if parent.State == Disabled || parent.State == Shadowed {
			parent.State = NotLoaded
		}
		return parent
	}

	var best *Load
	for _, f := range fs {
		if l, ok := byMechanism(chain, agent, f.Mechanism, ctx); ok && (best == nil || rank(l.State) < rank(best.State)) {
			l := l
			best = &l
		}
	}
	if best != nil {
		return *best
	}
	if top.Deferred(agent) {
		return Load{Agent: agent, State: NotLoaded, Reason: orDefault(top.Note, "not part of the starting context"), Confidence: ConfDocumented}
	}
	return ld(agent, Unknown, ConfRuntimeDependent, "no loading rule known for this item")
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func rank(s State) int {
	order := []State{Startup, Listed, Conditional, OnDemand, Invocation, Event, Enforced, Unknown, NotLoaded, Shadowed, Disabled}
	if i := slices.Index(order, s); i >= 0 {
		return i
	}
	return len(order)
}

func isImport(chain []*discovery.Item, agent string) bool {
	last := chain[len(chain)-1]
	if last.HasMechanism(agent, discovery.MechImport) {
		return true
	}
	parent := chain[len(chain)-2]
	return parent.Category == discovery.CategoryInstruction && !parent.IsDir &&
		last.Category == discovery.CategoryInstruction && !last.IsDir
}

func providerName(it *discovery.Item) string {
	p := it.Provider()
	if name, ok := strings.CutPrefix(p, "plugin:"); ok {
		return "plugin " + name
	}
	return p
}

func agentName(a string) string {
	switch a {
	case discovery.AgentClaude:
		return "Claude Code"
	case discovery.AgentCodex:
		return "Codex"
	case discovery.AgentCursor:
		return "Cursor"
	}
	return a
}

// displayPath shortens p relative to the project root or home.
func displayPath(env discovery.Env, p string) string {
	if env.ProjectRoot != "" {
		if rel, ok := under(env.ProjectRoot, p); ok {
			return rel
		}
	}
	return env.Tilde(p)
}

// relDir renders dir relative to the project root ("the project root" for the root).
func relDir(env discovery.Env, dir string) string {
	if dir == env.ProjectRoot {
		return "the project root"
	}
	return displayPath(env, dir) + "/"
}

func under(dir, p string) (string, bool) {
	rel, err := filepath.Rel(dir, p)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return rel, true
}

// nestedDir is the directory a nested item applies to: the directory
// holding the file, or the directory holding .cursor/rules.
func nestedDir(it *discovery.Item) string {
	p := it.Path
	if it.IsDir && filepath.Base(p) == "rules" && strings.HasPrefix(filepath.Base(filepath.Dir(p)), ".") {
		return filepath.Dir(filepath.Dir(p))
	}
	if it.IsDir {
		return p
	}
	return filepath.Dir(p)
}

func fmOf(path string) map[string]string {
	fields, _ := fmatter.ReadFile(path)
	return fmatter.Map(fields)
}

func stem(p string) string { return strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)) }

// ---------------------------------------------------------------- dispatch

func byMechanism(chain []*discovery.Item, agent string, m discovery.Mechanism, ctx Context) (Load, bool) {
	// Rules shared by every agent: hooks, MCP servers, skills.
	top := chain[0]
	switch top.Category {
	case discovery.CategoryHook:
		return hookLoad(chain, agent), true
	case discovery.CategoryMCP:
		return mcpLoad(agent, m), true
	case discovery.CategorySkill:
		return skillLoad(chain, agent), true
	}
	switch agent {
	case discovery.AgentClaude:
		return claudeLoad(chain, m, ctx)
	case discovery.AgentCodex:
		return codexLoad(chain, m, ctx)
	case discovery.AgentCursor:
		return cursorLoad(chain, m, ctx)
	}
	return Load{}, false
}

func hookEvent(it *discovery.Item) string {
	ev, _ := it.Config["event"].(string)
	return ev
}

func hookLoad(chain []*discovery.Item, agent string) Load {
	top := chain[0]
	if len(chain) > 1 {
		return ld(agent, Event, ConfDocumented, "Runs as part of the hook; the script itself is not sent to the model")
	}
	ev := hookEvent(top)
	reason := fmt.Sprintf("Runs on the %s event, not part of the context", orDefault(ev, "configured"))
	if strings.EqualFold(ev, "SessionStart") || strings.EqualFold(ev, "UserPromptSubmit") {
		reason += "; its output is added to the agent's context"
	}
	return ld(agent, Event, ConfDocumented, "%s", reason)
}

func mcpLoad(agent string, m discovery.Mechanism) Load {
	reason := "Tools are offered once the server starts; AYCA never starts servers"
	switch {
	case agent == discovery.AgentClaude:
		if m == discovery.MechProjectMCP {
			reason += "; project servers in .mcp.json need your approval"
		}
		reason += "; Claude Code may defer tool definitions (tool search) when there are many"
	case agent == discovery.AgentCodex && m == discovery.MechProjectMCP:
		reason += "; project config is only applied for trusted projects"
	}
	return ld(agent, Unknown, ConfRuntimeDependent, "%s", reason)
}

func skillName(top *discovery.Item) string {
	if n := fmOf(filepath.Join(top.Path, "SKILL.md"))["name"]; n != "" {
		return n
	}
	return top.Name
}

func isScript(it *discovery.Item) bool {
	switch strings.ToLower(filepath.Ext(it.Path)) {
	case ".sh", ".bash", ".zsh", ".py", ".rb", ".js", ".mjs", ".ts", ".pl", ".ps1":
		return true
	}
	for p := filepath.Dir(it.Path); p != "/" && p != "."; p = filepath.Dir(p) {
		if filepath.Base(p) == "scripts" {
			return true
		}
	}
	return false
}

func skillLoad(chain []*discovery.Item, agent string) Load {
	top, last := chain[0], chain[len(chain)-1]
	name := skillName(top)
	if len(chain) > 1 {
		switch {
		case last.IsDir:
			return ld(agent, OnDemand, ConfDocumented, "Supporting folder; its files are read only if SKILL.md directs the agent to them")
		case len(chain) == 2 && last.Name == "SKILL.md":
			return ld(agent, Invocation, ConfDocumented, "Full text loads when the skill is invoked")
		case isScript(last):
			return ld(agent, OnDemand, ConfDocumented, "Script: run, not read, when SKILL.md tells the agent to execute it; AYCA never runs it")
		}
		return ld(agent, OnDemand, ConfDocumented, "Supporting file; read only if SKILL.md directs the agent to it")
	}
	if strings.EqualFold(fmOf(filepath.Join(top.Path, "SKILL.md"))["disable-model-invocation"], "true") {
		return ld(agent, Invocation, ConfDocumented, "Only you can invoke it with /%s (disable-model-invocation is set)", name)
	}
	return ld(agent, Listed, ConfDocumented, "Name and description listed at startup; SKILL.md loads when the skill is invoked; supporting files only when SKILL.md points to them")
}

// ---------------------------------------------------------------- Claude Code

func claudeLoad(chain []*discovery.Item, m discovery.Mechanism, ctx Context) (Load, bool) {
	const a = discovery.AgentClaude
	top, last := chain[0], chain[len(chain)-1]
	switch m {
	case discovery.MechManagedInstructions:
		return ld(a, Startup, ConfDocumented, "Managed policy CLAUDE.md is always loaded and cannot be excluded"), true
	case discovery.MechUserInstructions:
		return ld(a, Startup, ConfDocumented, "Your user-level CLAUDE.md is loaded in every session"), true
	case discovery.MechDirInstructions, discovery.MechLocalInstructions:
		where := relDir(ctx.Env, filepath.Dir(top.Path))
		if base := filepath.Base(filepath.Dir(top.Path)); base == ".claude" {
			where = relDir(ctx.Env, filepath.Dir(filepath.Dir(top.Path)))
		}
		return ld(a, Startup, ConfDocumented, "Claude walks up from the working directory to / loading CLAUDE.md files; this one is in %s", where), true
	case discovery.MechNestedInstructions:
		return ld(a, OnDemand, ConfDocumented, "Loaded when Claude reads files in %s", relDir(ctx.Env, nestedDir(top))), true
	case discovery.MechAutoMemory:
		if len(chain) == 1 {
			return ld(a, Startup, ConfDocumented, "MEMORY.md (first 200 lines) loads every session; topic files are read on demand"), true
		}
		if !last.IsDir && last.Name == "MEMORY.md" && len(chain) == 2 {
			return ld(a, Startup, ConfDocumented, "First 200 lines of MEMORY.md load every session"), true
		}
		return ld(a, OnDemand, ConfDocumented, "Memory topic file; Claude reads it on demand"), true
	case discovery.MechUserRules, discovery.MechProjectRules:
		if last.IsDir {
			return dirLoad(chain, a, ctx), true
		}
		if p := fmOf(last.Path)["paths"]; p != "" {
			return ld(a, Conditional, ConfDocumented, "When Claude works with files matching %s", p), true
		}
		return ld(a, Startup, ConfDocumented, "No paths frontmatter, so it is always loaded"), true
	case discovery.MechUserAgents, discovery.MechProjectAgents, discovery.MechPluginAgents:
		if last.IsDir {
			return ld(a, Listed, ConfDocumented, "Subagent names and descriptions are listed; each body becomes the subagent's prompt when it runs"), true
		}
		return ld(a, Listed, ConfDocumented, "Name and description listed; the body becomes the subagent's prompt when it runs"), true
	case discovery.MechUserCommands, discovery.MechProjectCommands, discovery.MechPluginCommands:
		if last.IsDir {
			return dirLoad(chain, a, ctx), true
		}
		return claudeCommand(last), true
	case discovery.MechUserOutputStyles, discovery.MechProjectOutputStyles, discovery.MechPluginOutputStyles:
		if last.IsDir {
			return dirLoad(chain, a, ctx), true
		}
		return outputStyle(last, ctx), true
	case discovery.MechBuiltinPrompt, discovery.MechSessionEnv:
		return ld(a, Startup, ConfDocumented, "Built into Claude Code; content not inspectable"), true
	case discovery.MechConnectors, discovery.MechLaunchFlags:
		return ld(a, Unknown, ConfRuntimeDependent, "Decided by your account or launch flags at runtime; AYCA cannot see it"), true
	}
	return Load{}, false
}

func claudeCommand(f *discovery.Item) Load {
	name := stem(f.Path)
	if fmOf(f.Path)["description"] != "" {
		return ld(discovery.AgentClaude, Listed, ConfDocumented, "Description listed so Claude can invoke /%s itself; the full text loads when it runs", name)
	}
	return ld(discovery.AgentClaude, Invocation, ConfDocumented, "No description, so it loads only when you run /%s", name)
}

func outputStyle(f *discovery.Item, ctx Context) Load {
	const a = discovery.AgentClaude
	if s := ctx.OutputStyle; s != "" {
		if strings.EqualFold(fmOf(f.Path)["name"], s) || strings.EqualFold(stem(f.Path), s) {
			return ld(a, Startup, ConfDocumented, "Active output style (%s)", s)
		}
	}
	return ld(a, NotLoaded, ConfDocumented, "Not the active output style")
}

// ---------------------------------------------------------------- Codex

func codexLoad(chain []*discovery.Item, m discovery.Mechanism, ctx Context) (Load, bool) {
	const a = discovery.AgentCodex
	top, last := chain[0], chain[len(chain)-1]
	switch m {
	case discovery.MechUserInstructions:
		return ld(a, Startup, ConfDocumented, "Your personal Codex AGENTS.md is loaded in every session"), true
	case discovery.MechDirInstructions:
		if top.Scope == discovery.ScopeAncestor {
			return ld(a, NotLoaded, ConfDocumented, "Above the project root; Codex starts at the root"), true
		}
		return ld(a, Startup, ConfDocumented, "Codex concatenates AGENTS.md files from the project root down to the working directory (32 KiB cap)"), true
	case discovery.MechNestedInstructions:
		return ld(a, OnDemand, ConfVersionDependent, "Codex does not load it at startup; its built-in instructions tell the model to follow AGENTS.md files in directories it works in (%s)", relDir(ctx.Env, nestedDir(top))), true
	case discovery.MechExecPolicy:
		return ld(a, Enforced, ConfDocumented, "Execution policy enforced by the Codex CLI on shell commands; not sent to the model"), true
	case discovery.MechUserCommands:
		if last.IsDir {
			return ld(a, Invocation, ConfDocumented, "Custom prompts run only when you type /prompts:<name>"), true
		}
		return ld(a, Invocation, ConfDocumented, "Runs only when you type /prompts:%s", stem(last.Path)), true
	case discovery.MechBuiltinPrompt:
		return ld(a, Startup, ConfDocumented, "Built into the Codex CLI; content not inspectable"), true
	}
	return Load{}, false
}

// ---------------------------------------------------------------- Cursor

func cursorLoad(chain []*discovery.Item, m discovery.Mechanism, ctx Context) (Load, bool) {
	const a = discovery.AgentCursor
	top, last := chain[0], chain[len(chain)-1]
	switch m {
	case discovery.MechDirInstructions:
		return ld(a, Startup, ConfDocumented, "Root AGENTS.md is included in every chat"), true
	case discovery.MechNestedInstructions:
		return ld(a, OnDemand, ConfDocumented, "Applies when working in %s", relDir(ctx.Env, nestedDir(top))), true
	case discovery.MechLegacyRules:
		return ld(a, Startup, ConfVersionDependent, "Legacy .cursorrules format, deprecated"), true
	case discovery.MechProjectRules, discovery.MechNestedRules:
		if last.IsDir {
			return dirLoad(chain, a, ctx), true
		}
		l := cursorRule(last)
		if m == discovery.MechNestedRules && l.State == Startup {
			l = ld(a, Conditional, ConfDocumented, "Applies when working in %s", relDir(ctx.Env, nestedDir(top)))
		}
		return l, true
	case discovery.MechProjectCommands, discovery.MechUserCommands:
		if last.IsDir {
			return ld(a, Invocation, ConfDocumented, "Commands run only when you type /<name>"), true
		}
		return ld(a, Invocation, ConfDocumented, "Runs only when you type /%s", stem(last.Path)), true
	case discovery.MechAppSettings:
		return ld(a, Unknown, ConfRuntimeDependent, "Stored in Cursor's app settings; AYCA cannot read it"), true
	}
	return Load{}, false
}

func cursorRule(f *discovery.Item) Load {
	const a = discovery.AgentCursor
	fm := fmOf(f.Path)
	switch {
	case strings.EqualFold(fm["alwaysApply"], "true"):
		return ld(a, Startup, ConfDocumented, "Always Apply: alwaysApply is true")
	case fm["globs"] != "":
		return ld(a, Conditional, ConfDocumented, "Auto-attached when files matching %s are in context", fm["globs"])
	case fm["description"] != "":
		return ld(a, Listed, ConfDocumented, "Agent Requested: the agent sees the description and decides")
	}
	return ld(a, Invocation, ConfDocumented, "Manual: only when you @-mention it")
}

// ---------------------------------------------------------------- directories

// leaves returns the chains of every file below the last item of chain.
func leaves(chain []*discovery.Item) [][]*discovery.Item {
	last := chain[len(chain)-1]
	var out [][]*discovery.Item
	for i := range last.Children {
		c := append(slices.Clone(chain), &last.Children[i])
		if last.Children[i].IsDir {
			out = append(out, leaves(c)...)
		} else {
			out = append(out, c)
		}
	}
	return out
}

// dirLoad summarises a directory of rule, command, output-style or
// exec-policy files by classifying every file in it.
func dirLoad(chain []*discovery.Item, agent string, ctx Context) Load {
	var best *Load
	counts := map[State]int{}
	conf := ConfDocumented
	for _, lc := range leaves(chain) {
		l := ClassifyPath(lc, agent, ctx)
		counts[l.State]++
		if l.Confidence != ConfDocumented {
			conf = l.Confidence
		}
		if best == nil || rank(l.State) < rank(best.State) {
			l := l
			best = &l
		}
	}
	if best == nil {
		return ld(agent, Unknown, ConfDocumented, "Folder with no files")
	}
	var parts []string
	for _, s := range []State{Startup, Listed, Conditional, OnDemand, Invocation, Enforced, NotLoaded} {
		n := counts[s]
		if n == 0 {
			continue
		}
		word := map[State]string{Startup: "always loaded", Listed: "listed", Conditional: "conditional",
			OnDemand: "on demand", Invocation: "manual", Enforced: "enforced", NotLoaded: "not loaded"}[s]
		if chain[0].Category == discovery.CategoryOutputStyle {
			word = map[State]string{Startup: "active", NotLoaded: "not the active style"}[s]
		}
		parts = append(parts, fmt.Sprintf("%d %s", n, word))
	}
	return Load{Agent: agent, State: best.State, Reason: strings.Join(parts, ", "), Confidence: conf}
}
