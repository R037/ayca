// Package explain produces rule-based explanations of discovered context:
// what an item is, where it comes from, how an agent discovers it and when
// it loads. It uses a fixed knowledge table; there is no LLM and no network
// access, and it is read-only.
package explain

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/R037/ayca/internal/discovery"
	"github.com/R037/ayca/internal/effective"
	fmatter "github.com/R037/ayca/internal/frontmatter"
)

// Field is one labelled value.
type Field struct{ Label, Value string }

// Explanation is a structured, human-readable account of one item.
type Explanation struct {
	Title     string
	Agent     string           // the agent the explanation is for
	Kind      string           // e.g. "Skill", "Skill supporting file", "Instruction file", "@import"
	What      string           // "What is this?"
	Where     []Field          // Path, Real path, Also at, Scope, Provider, Part of
	Discovery string           // "How does the agent discover it?"
	Loads     []effective.Load // selected agent first, then the other agents that read it
	Related   []string
	Caveats   []string
}

const maxDesc = 200

// Explain explains the last item of chain (ancestors first) for agent.
func Explain(chain []*discovery.Item, agent string, ctx effective.Context) Explanation {
	if len(chain) == 0 {
		return Explanation{Agent: agent, Title: "Nothing selected"}
	}
	e := Explanation{Agent: agent, Title: title(chain)}

	e.Loads = loads(chain, agent, ctx)
	main := e.Loads[0]
	for _, l := range e.Loads {
		if l.Agent == agent {
			main = l
		}
	}
	e.Kind = kind(chain, agent, main)
	e.What = what(chain, agent, main)
	e.Where = where(chain, agent)
	e.Discovery = discoveryText(chain, agent)
	e.Related = related(chain, agent, ctx)
	e.Caveats = caveats(chain, e.Loads)
	return e
}

func title(chain []*discovery.Item) string {
	top, last := chain[0], chain[len(chain)-1]
	if len(chain) == 1 || last.HasMechanism(discovery.AgentClaude, discovery.MechImport) || top.Category == discovery.CategoryInstruction {
		return last.Name
	}
	if top.Category == discovery.CategorySkill {
		parts := []string{top.Name}
		for _, c := range chain[1:] {
			parts = append(parts, strings.TrimSuffix(c.Name, "/"))
		}
		return strings.Join(parts, "/")
	}
	return last.Name
}

// readers lists the agents that read the item (top-level agents of the chain).
func readers(chain []*discovery.Item) []string {
	var out []string
	for _, a := range discovery.AllAgents {
		if slices.Contains(chain[0].Agents, a) || slices.Contains(chain[len(chain)-1].Agents, a) {
			out = append(out, a)
		}
	}
	return out
}

func loads(chain []*discovery.Item, agent string, ctx effective.Context) []effective.Load {
	rs := readers(chain)
	var out []effective.Load
	if slices.Contains(rs, agent) {
		out = append(out, effective.ClassifyPath(chain, agent, ctx))
	} else {
		out = append(out, effective.Load{Agent: agent, State: effective.NotLoaded,
			Reason: AgentName(agent) + " does not read this location", Confidence: effective.ConfDocumented})
	}
	for _, a := range rs {
		if a != agent {
			out = append(out, effective.ClassifyPath(chain, a, ctx))
		}
	}
	return out
}

// mechanism returns the mechanism by which agent finds the item.
func mechanism(chain []*discovery.Item, agent string) (discovery.Mechanism, bool) {
	last, top := chain[len(chain)-1], chain[0]
	if len(chain) > 1 && last.HasMechanism(agent, discovery.MechImport) {
		return discovery.MechImport, true
	}
	if fs := top.FindingsFor(agent); len(fs) > 0 {
		return fs[0].Mechanism, true
	}
	return "", false
}

func discoveryText(chain []*discovery.Item, agent string) string {
	m, ok := mechanism(chain, agent)
	if !ok {
		others := readers(chain)
		if len(others) == 0 {
			return AgentName(agent) + " does not read this location."
		}
		var names []string
		for _, a := range others {
			names = append(names, AgentName(a))
		}
		return fmt.Sprintf("%s does not read this location. It is read by %s.", AgentName(agent), strings.Join(names, ", "))
	}
	txt := Discovery[agent][m]
	if txt == "" {
		txt = fmt.Sprintf("%s finds this through the %q rule.", AgentName(agent), m)
	}
	if len(chain) > 1 && m != discovery.MechImport {
		txt += " This item sits inside that location."
	}
	return txt
}

// ---------------------------------------------------------------- kind & what

func isNested(top *discovery.Item) bool {
	for _, f := range top.Found {
		if f.Mechanism == discovery.MechNestedInstructions || f.Mechanism == discovery.MechNestedRules {
			return true
		}
	}
	return false
}

func isExec(top *discovery.Item) bool {
	return top.HasMechanism(discovery.AgentCodex, discovery.MechExecPolicy)
}

func isScriptFile(it *discovery.Item) bool {
	switch strings.ToLower(filepath.Ext(it.Path)) {
	case ".sh", ".bash", ".zsh", ".py", ".rb", ".js", ".mjs", ".ts", ".pl", ".ps1":
		return true
	}
	return strings.Contains(filepath.ToSlash(it.Path), "/scripts/")
}

func kind(chain []*discovery.Item, agent string, l effective.Load) string {
	top, last := chain[0], chain[len(chain)-1]
	switch top.Category {
	case discovery.CategoryInstruction:
		switch {
		case len(chain) > 1:
			return "@import"
		case isNested(top):
			return "Instruction file (nested)"
		}
		return "Instruction file"
	case discovery.CategoryMemory:
		switch {
		case len(chain) == 1:
			return "Auto memory"
		case last.Name == "MEMORY.md":
			return "Memory index"
		}
		return "Memory topic file"
	case discovery.CategoryRule:
		switch {
		case isExec(top) && last.IsDir:
			return "Execution policy folder"
		case isExec(top):
			return "Execution policy rules"
		case last.IsDir:
			return "Rules folder"
		case top.HasMechanism(agent, discovery.MechLegacyRules):
			return "Rule (legacy)"
		}
		switch l.State {
		case effective.Startup:
			return "Rule (always loaded)"
		case effective.Conditional:
			return "Rule (conditional)"
		case effective.Listed:
			return "Rule (agent requested)"
		case effective.Invocation:
			return "Rule (manual)"
		}
		return "Rule"
	case discovery.CategorySkill:
		switch {
		case len(chain) == 1:
			return "Skill"
		case last.IsDir:
			return "Skill folder"
		case len(chain) == 2 && last.Name == "SKILL.md":
			return "Skill entry file"
		case isScriptFile(last):
			return "Skill script"
		}
		return "Skill supporting file"
	case discovery.CategoryAgent:
		if last.IsDir {
			return "Subagents folder"
		}
		return "Subagent"
	case discovery.CategoryCommand:
		switch {
		case last.IsDir:
			return "Commands folder"
		case agent == discovery.AgentCodex:
			return "Custom prompt"
		}
		return "Command"
	case discovery.CategoryOutputStyle:
		if last.IsDir {
			return "Output styles folder"
		}
		return "Output style"
	case discovery.CategoryHook:
		if len(chain) > 1 {
			return "Hook script"
		}
		return "Hook"
	case discovery.CategoryMCP:
		return "MCP server"
	case discovery.CategoryRuntime:
		return "Runtime context"
	}
	return "Item"
}

func description(path string) string {
	fields, _ := fmatter.ReadFile(path)
	d := strings.TrimSpace(fmatter.Get(fields, "description"))
	if r := []rune(d); len(r) > maxDesc {
		d = string(r[:maxDesc-1]) + "…"
	}
	return d
}

func withDesc(s, path string) string {
	if d := description(path); d != "" {
		return s + fmt.Sprintf(" Its description reads: \"%s\"", d)
	}
	return s
}

func what(chain []*discovery.Item, agent string, l effective.Load) string {
	top, last := chain[0], chain[len(chain)-1]
	switch top.Category {
	case discovery.CategoryInstruction:
		return instructionWhat(chain)
	case discovery.CategoryMemory:
		switch {
		case len(chain) == 1:
			return "Claude Code's auto memory for this project: notes it wrote for itself in earlier sessions. MEMORY.md is an index; it links to topic files with the details."
		case last.Name == "MEMORY.md":
			return "The index of Claude Code's auto memory for this project. Its first 200 lines are added to every session."
		}
		return "A topic file of Claude Code's auto memory. MEMORY.md points to it; Claude reads it when the topic comes up."
	case discovery.CategoryRule:
		switch {
		case isExec(top):
			return "Execution policy rules for Codex: patterns that decide whether a shell command runs, needs your approval or is forbidden. The Codex program applies them; the model never reads them."
		case last.IsDir:
			return "A folder of rules: short Markdown instructions that the agent applies always, or only when they match."
		case top.HasMechanism(agent, discovery.MechLegacyRules):
			return "Cursor's old single-file rules format. It is plain text that applies to the whole project."
		}
		return "A rule: a short Markdown instruction the agent follows. " + ruleHow(l)
	case discovery.CategorySkill:
		return skillWhat(chain)
	case discovery.CategoryAgent:
		if last.IsDir {
			return "A folder of subagent definitions. Each file describes a specialised helper the main agent can delegate work to."
		}
		return withDesc("A subagent: a specialised helper with its own prompt and tools that the main agent can delegate work to.", last.Path)
	case discovery.CategoryCommand:
		switch {
		case last.IsDir:
			return "A folder of commands: reusable prompts you run by typing /name."
		case agent == discovery.AgentCodex:
			return withDesc("A custom prompt: a reusable prompt template you run as /prompts:"+stem(last.Path)+".", last.Path)
		}
		return withDesc("A command: a reusable prompt you run by typing /"+stem(last.Path)+".", last.Path)
	case discovery.CategoryOutputStyle:
		if last.IsDir {
			return "A folder of output styles. An output style changes how Claude Code responds by replacing part of its system prompt."
		}
		return withDesc("An output style: it changes how Claude Code responds by replacing part of its system prompt.", last.Path)
	case discovery.CategoryHook:
		if len(chain) > 1 {
			return "A script run by a hook. AYCA shows its source but never runs it."
		}
		return hookWhat(top)
	case discovery.CategoryMCP:
		return "An MCP server: a program or service that gives the agent extra tools. AYCA shows its configuration with secrets masked and never starts it."
	case discovery.CategoryRuntime:
		return "Context that exists when the agent runs but is not stored in a file AYCA can read. " + firstLine(top.Info)
	}
	return "A piece of agent context."
}

func ruleHow(l effective.Load) string {
	switch l.State {
	case effective.Startup:
		return "It has no condition, so it is always part of the context."
	case effective.Conditional:
		return "It applies only when its condition matches."
	case effective.Listed:
		return "The agent sees its description and decides whether to read it."
	case effective.Invocation:
		return "It applies only when you mention it."
	}
	return ""
}

func firstLine(info string) string {
	for _, ln := range strings.Split(info, "\n") {
		ln = strings.TrimSpace(ln)
		if ln != "" && !strings.HasPrefix(ln, "#") {
			return ln
		}
	}
	return ""
}

func instructionWhat(chain []*discovery.Item) string {
	last := chain[len(chain)-1]
	if len(chain) > 1 {
		return fmt.Sprintf("A file pulled into %s with an @-reference. Claude Code expands it inline, as if its text were written there.", chain[len(chain)-2].Name)
	}
	switch filepath.Base(last.Path) {
	case "CLAUDE.md":
		return "Claude Code's instruction file: standing guidance in Markdown about the project or your preferences, read at the start of a session."
	case "CLAUDE.local.md":
		return "Your personal, usually git-ignored instruction file for Claude Code in this project."
	case "AGENTS.override.md":
		return "A local override of AGENTS.md for Codex. When it exists in a directory, Codex reads it instead of AGENTS.md there."
	case "AGENTS.md":
		return "An AGENTS.md file: plain Markdown instructions for coding agents, shared between tools such as Codex and Cursor."
	}
	return "An instruction file the agent reads as standing guidance."
}

func skillWhat(chain []*discovery.Item) string {
	top, last := chain[0], chain[len(chain)-1]
	skillMD := filepath.Join(top.Path, "SKILL.md")
	switch {
	case len(chain) == 1:
		return withDesc("A skill: a folder with a SKILL.md and optional supporting files that teaches the agent a repeatable task. The agent sees only its name and description until it uses it.", skillMD)
	case last.IsDir:
		return fmt.Sprintf("A folder inside the %s skill that groups supporting files.", top.Name)
	case len(chain) == 2 && last.Name == "SKILL.md":
		return withDesc("The entry file of the "+top.Name+" skill: frontmatter with its name and description, then the instructions the agent follows when it uses the skill.", last.Path)
	case isScriptFile(last):
		return fmt.Sprintf("A script bundled with the %s skill. SKILL.md tells the agent when to run it; the agent runs it rather than reading it, and AYCA never does.", top.Name)
	}
	return fmt.Sprintf("A supporting file of the %s skill: reference text or an asset that the agent reads only when SKILL.md points to it.", top.Name)
}

func hookWhat(top *discovery.Item) string {
	ev, _ := top.Config["event"].(string)
	s := "A hook: a command that the agent program runs automatically when an event happens"
	if ev != "" {
		s += " (" + ev + ")"
	}
	s += ". It is not context itself."
	if strings.EqualFold(ev, "SessionStart") || strings.EqualFold(ev, "UserPromptSubmit") {
		s += " What it prints is added to the agent's context."
	}
	return s
}

func stem(p string) string { return strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)) }

// ---------------------------------------------------------------- where

func scopeText(s discovery.Scope) string {
	switch s {
	case discovery.ScopeProject:
		return "Project (inside the project root)"
	case discovery.ScopeAncestor:
		return "Ancestor (a directory above the project root)"
	case discovery.ScopeGlobal:
		return "Global (your user-wide configuration)"
	case discovery.ScopeSystem:
		return "System (machine-wide policy or built into the agent)"
	}
	return string(s)
}

func where(chain []*discovery.Item, agent string) []Field {
	top, last := chain[0], chain[len(chain)-1]
	var out []Field
	if last.Path != "" {
		label := "Path"
		if last.Virtual {
			label = "Defined in"
		}
		out = append(out, Field{label, last.Path})
		real := last.Target
		if !last.Virtual {
			if r, err := filepath.EvalSymlinks(last.Path); err == nil {
				real = r
			}
		}
		if real != "" && real != filepath.Clean(last.Path) {
			out = append(out, Field{"Real path", real})
		}
	}
	for _, a := range last.Aliases {
		out = append(out, Field{"Also at", a})
	}
	if len(chain) == 1 {
		for _, a := range top.Aliases {
			if !slices.Contains(last.Aliases, a) {
				out = append(out, Field{"Also at", a})
			}
		}
	}
	out = append(out, Field{"Scope", scopeText(top.Scope)})
	if p := top.Provider(); p != "" {
		out = append(out, Field{"Provider", strings.Replace(p, "plugin:", "plugin ", 1)})
	}
	if len(chain) > 1 {
		var names []string
		for _, c := range chain[:len(chain)-1] {
			names = append(names, strings.TrimSuffix(c.Name, "/"))
		}
		out = append(out, Field{"Part of", strings.Join(names, " › ")})
	}
	return out
}

// ---------------------------------------------------------------- related

func capList(names []string, n int) string {
	if len(names) <= n {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:n], ", ") + fmt.Sprintf(", … (%d more)", len(names)-n)
}

func tildeAll(env discovery.Env, ps []string) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = env.Tilde(p)
	}
	return out
}

// capMore joins up to n names and appends "+K more" for the rest.
func capMore(names []string, n int) string {
	if len(names) <= n {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:n], ", ") + fmt.Sprintf(" +%d more", len(names)-n)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// files returns the relative names of all files below it.
func files(it *discovery.Item, prefix string) []string {
	var out []string
	for i := range it.Children {
		c := &it.Children[i]
		n := prefix + strings.TrimSuffix(c.Name, "/")
		if c.IsDir {
			out = append(out, files(c, n+"/")...)
		} else {
			out = append(out, n)
		}
	}
	return out
}

var mdLink = regexp.MustCompile(`\]\(([^)\s#]+)(?:#[^)]*)?\)`)

// missingRefs lists relative Markdown links in a SKILL.md that do not exist.
func missingRefs(skillDir string) []string {
	b, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		return nil
	}
	var out []string
	for _, m := range mdLink.FindAllStringSubmatch(string(b), -1) {
		ref := m[1]
		if strings.Contains(ref, "://") || strings.HasPrefix(ref, "mailto:") || filepath.IsAbs(ref) {
			continue
		}
		if _, err := os.Stat(filepath.Join(skillDir, ref)); err != nil && !slices.Contains(out, ref) {
			out = append(out, ref)
		}
	}
	return out
}

func related(chain []*discovery.Item, agent string, ctx effective.Context) []string {
	top, last := chain[0], chain[len(chain)-1]
	var out []string

	if r, err := filepath.EvalSymlinks(last.Path); err == nil && !last.Virtual && last.Path != "" && r != filepath.Clean(last.Path) {
		out = append(out, fmt.Sprintf("%s is a symlink to %s.", ctx.Env.DisplayPath(last.Path), ctx.Env.DisplayPath(r)))
	}
	if len(last.Aliases) > 0 {
		out = append(out, fmt.Sprintf("The same item is reachable from %s: %s.", plural(len(last.Aliases)+1, "location", "locations"), capList(tildeAll(ctx.Env, append([]string{last.Path}, last.Aliases...)), 4)))
	}
	if p := top.Provider(); p != "" && top.DisabledFor(agent) {
		out = append(out, fmt.Sprintf("Provided by %s, which is disabled in settings, so none of its items load.", strings.Replace(p, "plugin:", "plugin ", 1)))
	} else if p != "" {
		out = append(out, fmt.Sprintf("Provided by %s.", strings.Replace(p, "plugin:", "plugin ", 1)))
	}
	if sh := top.ShadowedFor(agent); sh != "" {
		out = append(out, fmt.Sprintf("Shadowed by %s, which takes precedence for %s.", ctx.Env.DisplayPath(sh), AgentName(agent)))
	}
	if filepath.Base(last.Path) == "AGENTS.override.md" {
		sib := filepath.Join(filepath.Dir(last.Path), "AGENTS.md")
		if _, err := os.Stat(sib); err == nil {
			out = append(out, "Shadows AGENTS.md in the same directory for Codex, which reads this file instead.")
		}
	}
	if len(chain) > 1 && top.Category == discovery.CategoryInstruction {
		out = append(out, fmt.Sprintf("Imported by %s.", chain[len(chain)-2].Name))
	}
	if top.Category == discovery.CategoryInstruction && !last.IsDir {
		var ok, missing []string
		for i := range last.Children {
			c := &last.Children[i]
			if c.Missing {
				missing = append(missing, c.Name)
			} else {
				ok = append(ok, c.Name)
			}
		}
		if len(ok) > 0 {
			out = append(out, fmt.Sprintf("Imports %s: %s.", plural(len(ok), "file", "files"), capList(ok, 4)))
		}
		if len(missing) > 0 {
			out = append(out, fmt.Sprintf("Missing %s (file not found): %s.", plural(len(missing), "import", "imports"), capList(missing, 4)))
		}
	}
	if top.Category == discovery.CategorySkill && (len(chain) == 1 || (len(chain) == 2 && last.Name == "SKILL.md")) {
		var sup []string
		for _, f := range files(top, "") {
			if f != "SKILL.md" {
				sup = append(sup, f)
			}
		}
		if len(sup) > 0 {
			out = append(out, fmt.Sprintf("%s: %s.", plural(len(sup), "supporting file", "supporting files"), capMore(sup, 3)))
		}
		for _, ref := range missingRefs(top.Path) {
			out = append(out, fmt.Sprintf("SKILL.md points to %s, which does not exist.", ref))
		}
	}
	if len(chain) > 1 && top.Category == discovery.CategorySkill {
		out = append(out, fmt.Sprintf("Part of the %s skill.", top.Name))
	}
	if last.IsDir && top.Category != discovery.CategorySkill {
		if fs := files(last, ""); len(fs) > 0 {
			out = append(out, fmt.Sprintf("Contains %s: %s.", plural(len(fs), "file", "files"), capList(fs, 4)))
		}
	}
	if top.Category == discovery.CategoryHook && len(chain) == 1 {
		if m, _ := top.Config["matcher"].(string); m != "" {
			out = append(out, "Matcher: "+m+".")
		}
		if c, _ := top.Config["command"].(string); c != "" {
			out = append(out, "Command: "+c+".")
		}
		if n := len(top.Children); n > 0 {
			out = append(out, fmt.Sprintf("Runs %s that AYCA found: %s.", plural(n, "script", "scripts"), childNames(top)))
		}
	}
	if top.Category == discovery.CategoryMCP {
		for _, k := range []string{"command", "url", "type"} {
			if v, _ := top.Config[k].(string); v != "" {
				out = append(out, fmt.Sprintf("%s: %s.", strings.ToUpper(k[:1])+k[1:], v))
			}
		}
	}
	if others := otherReaders(chain, agent); len(others) > 0 {
		out = append(out, "Also read by "+strings.Join(others, ", ")+".")
	}
	return out
}

func childNames(it *discovery.Item) string {
	var n []string
	for i := range it.Children {
		n = append(n, it.Children[i].Name)
	}
	return capList(n, 4)
}

func otherReaders(chain []*discovery.Item, agent string) []string {
	var out []string
	for _, a := range readers(chain) {
		if a != agent {
			out = append(out, AgentName(a))
		}
	}
	return out
}

// ---------------------------------------------------------------- caveats

func caveats(chain []*discovery.Item, ls []effective.Load) []string {
	var out []string
	for _, l := range ls {
		if l.Confidence != effective.ConfDocumented {
			out = append(out, fmt.Sprintf("%s: this is %s. Basis: %s; other versions may behave differently.", AgentName(l.Agent), l.Confidence, effective.RulesBasis))
			break
		}
	}
	switch chain[0].Category {
	case discovery.CategoryHook:
		out = append(out, "AYCA never runs hooks or their scripts, so it cannot show their output.")
	case discovery.CategoryMCP:
		out = append(out, "AYCA never starts MCP servers, so it cannot show which tools they offer.")
	}
	out = append(out, "AYCA shows what is configured on disk. It does not know what a running session actually received.")
	return out
}

// ---------------------------------------------------------------- markdown

// Markdown renders the explanation as a Markdown panel.
func (e Explanation) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n*%s*\n\n", e.Title, e.Kind)
	b.WriteString("## What is this?\n\n" + e.What + "\n\n")
	b.WriteString("## Where does it come from?\n\n")
	for _, f := range e.Where {
		fmt.Fprintf(&b, "- **%s:** %s\n", f.Label, md(f.Value))
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "## How does %s discover it?\n\n%s\n\n", AgentName(e.Agent), e.Discovery)
	b.WriteString("## When does it load?\n\n")
	for _, l := range e.Loads {
		fmt.Fprintf(&b, "- **%s — %s:** %s", AgentName(l.Agent), l.State.Label(), l.Reason)
		if l.Confidence != effective.ConfDocumented {
			fmt.Fprintf(&b, " *(%s)*", l.Confidence)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n## How does it relate to other context?\n\n")
	if len(e.Related) == 0 {
		b.WriteString("Nothing notable: it stands on its own.\n")
	}
	for _, r := range e.Related {
		b.WriteString("- " + r + "\n")
	}
	b.WriteString("\n## Limits\n\n")
	for _, c := range e.Caveats {
		b.WriteString("- " + c + "\n")
	}
	return b.String()
}

// md wraps a value that looks like a path in code ticks.
func md(v string) string {
	if strings.HasPrefix(v, "/") || strings.HasPrefix(v, "~") {
		return "`" + v + "`"
	}
	return v
}
