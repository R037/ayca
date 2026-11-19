// Package budget estimates how much of each agent's context window the
// discovered context takes up before the first prompt is sent. It reads
// files but never writes, and it is an estimate: token counts use the same
// ≈4 bytes per token rule as the preview, and what an agent actually loads
// can differ by version.
package budget

import (
	"bufio"
	"bytes"
	"fmt"
	fmatter "github.com/R037/ayca/internal/frontmatter"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/R037/ayca/internal/discovery"
)

// DefaultWindows are the context window sizes (in tokens) assumed per agent
// when none is given. They depend on the model in use; override with
// --window.
var DefaultWindows = map[string]int{
	discovery.AgentClaude: 200_000,
	discovery.AgentCodex:  272_000,
	discovery.AgentCursor: 200_000,
}

// How an entry counts towards the startup context.
const (
	HowFull        = "full text"
	HowDescription = "name + description"
	HowMemory      = "first 200 lines"
)

const (
	memoryLines     = 200       // Claude Code loads the first 200 lines of MEMORY.md
	codexProjectCap = 32 * 1024 // Codex's default project_doc_max_bytes
	maxRead         = 1 << 20
)

// Entry is one piece of context counted at startup.
type Entry struct {
	ItemID   string
	Category discovery.Category
	Name     string
	Path     string
	Tokens   int
	How      string
}

// Agent is the estimate for one agent.
type Agent struct {
	Agent   string
	Window  int
	Tokens  int
	Entries []Entry // largest first
	// NotCounted lists startup context AYCA knows exists but can't size.
	NotCounted []string
	// Notes explain caps or adjustments that were applied.
	Notes []string
}

// Percent is Tokens as a percentage of Window.
func (a Agent) Percent() float64 {
	if a.Window <= 0 {
		return 0
	}
	return float64(a.Tokens) * 100 / float64(a.Window)
}

// Subtotal is the tokens and entry count of one category.
type Subtotal struct {
	Category discovery.Category
	Tokens   int
	Count    int
}

// ByCategory sums the entries per category, in display order.
func (a Agent) ByCategory() []Subtotal {
	var out []Subtotal
	for _, cat := range discovery.Categories {
		st := Subtotal{Category: cat}
		for _, e := range a.Entries {
			if e.Category == cat {
				st.Tokens += e.Tokens
				st.Count++
			}
		}
		if st.Count > 0 {
			out = append(out, st)
		}
	}
	return out
}

// Report holds the estimate for every agent.
type Report struct {
	Agents []Agent
	// ByID maps an item ID to how it counts at startup (largest across
	// agents), for showing in an item's details.
	ByID map[string]Entry
}

// Estimate computes the startup context per agent. agents limits which
// agents are reported (all when empty). windows overrides DefaultWindows
// per agent; a missing entry falls back to the default.
func Estimate(res discovery.Result, agents []string, windows map[string]int) Report {
	if len(agents) == 0 {
		agents = discovery.AllAgents
	}
	rep := Report{ByID: map[string]Entry{}}
	for _, a := range agents {
		w := windows[a]
		if w <= 0 {
			w = DefaultWindows[a]
		}
		ag := Agent{Agent: a, Window: w}
		e := estimator{agent: a, outputStyle: res.OutputStyle, out: &ag}
		for i := range res.Items {
			it := &res.Items[i]
			if hasAgent(it, a) {
				e.item(it)
			}
		}
		e.finish()
		sort.SliceStable(ag.Entries, func(i, j int) bool { return ag.Entries[i].Tokens > ag.Entries[j].Tokens })
		for _, en := range ag.Entries {
			ag.Tokens += en.Tokens
			if ex, ok := rep.ByID[en.ItemID]; !ok || en.Tokens > ex.Tokens {
				rep.ByID[en.ItemID] = en
			}
		}
		rep.Agents = append(rep.Agents, ag)
	}
	return rep
}

type estimator struct {
	agent       string
	outputStyle string
	out         *Agent

	mcp, ctxHooks    int
	conditionalRules int
	codexProject     []Entry // AGENTS.md files subject to the 32 KiB cap
	styleFound       bool
}

func hasAgent(it *discovery.Item, a string) bool {
	for _, x := range it.Agents {
		if x == a {
			return true
		}
	}
	return false
}

func (e *estimator) add(it *discovery.Item, path string, tokens int, how string) {
	if tokens <= 0 {
		return
	}
	e.out.Entries = append(e.out.Entries, Entry{ItemID: it.ID, Category: it.Category, Name: it.Name, Path: path, Tokens: tokens, How: how})
}

func (e *estimator) item(it *discovery.Item) {
	if it.Deferred(e.agent) || it.Missing {
		return
	}
	switch it.Category {
	case discovery.CategoryInstruction:
		if e.agent == discovery.AgentCodex && it.Scope != discovery.ScopeGlobal {
			if t := fileTokens(it.Path); t > 0 {
				e.codexProject = append(e.codexProject, Entry{ItemID: it.ID, Category: it.Category, Name: it.Name, Path: it.Path, Tokens: t, How: HowFull})
			}
			return
		}
		e.add(it, it.Path, fileTokens(it.Path), HowFull)
		if e.agent == discovery.AgentClaude {
			e.imports(it.Children)
		}
	case discovery.CategoryMemory:
		if e.agent == discovery.AgentClaude && it.IsDir {
			p := filepath.Join(it.Path, "MEMORY.md")
			e.add(it, p, headTokens(p, memoryLines), HowMemory)
		}
	case discovery.CategoryRule:
		e.files(it, e.rule)
	case discovery.CategorySkill:
		if it.IsDir {
			skill := filepath.Join(it.Path, "SKILL.md")
			// A skill only you can invoke is not listed to the model.
			if strings.EqualFold(frontmatter(skill)["disable-model-invocation"], "true") {
				return
			}
			e.add(it, skill, descTokens(skill, it.Name), HowDescription)
		}
	case discovery.CategoryAgent:
		if e.agent == discovery.AgentClaude {
			e.files(it, func(f *discovery.Item) { e.add(f, f.Path, descTokens(f.Path, stem(f.Path)), HowDescription) })
		}
	case discovery.CategoryCommand:
		// Claude Code lists custom commands that have a description so it
		// can invoke them; Codex prompts and Cursor commands load only when
		// typed.
		if e.agent == discovery.AgentClaude {
			e.files(it, func(f *discovery.Item) {
				if m := frontmatter(f.Path); m["description"] != "" {
					e.add(f, f.Path, descTokens(f.Path, stem(f.Path)), HowDescription)
				}
			})
		}
	case discovery.CategoryOutputStyle:
		if e.agent == discovery.AgentClaude && e.outputStyle != "" {
			e.files(it, func(f *discovery.Item) {
				name := frontmatter(f.Path)["name"]
				if !e.styleFound && (strings.EqualFold(name, e.outputStyle) || strings.EqualFold(stem(f.Path), e.outputStyle)) {
					e.styleFound = true
					e.add(f, f.Path, bodyTokens(f.Path), HowFull)
				}
			})
		}
	case discovery.CategoryHook:
		if ev, _ := it.Config["event"].(string); ev == "SessionStart" || ev == "UserPromptSubmit" {
			e.ctxHooks++
		}
	case discovery.CategoryMCP:
		e.mcp++
	}
}

// imports counts files pulled in by @imports (recursively).
func (e *estimator) imports(children []discovery.Item) {
	for i := range children {
		c := &children[i]
		if c.Missing || c.Category != discovery.CategoryInstruction || c.IsDir {
			continue
		}
		e.add(c, c.Path, fileTokens(c.Path), HowFull+" (@import)")
		e.imports(c.Children)
	}
}

// files calls fn for every file in it (it itself, or the files below a
// directory item).
func (e *estimator) files(it *discovery.Item, fn func(*discovery.Item)) {
	if !it.IsDir {
		fn(it)
		return
	}
	for i := range it.Children {
		e.files(&it.Children[i], fn)
	}
}

// rule decides whether a rule file is always in context.
func (e *estimator) rule(f *discovery.Item) {
	base := filepath.Base(f.Path)
	ext := strings.ToLower(filepath.Ext(f.Path))
	switch {
	case ext == ".rules":
		return // Codex execution policy: enforced by the CLI, not sent to the model
	case base == ".cursorrules":
		e.add(f, f.Path, fileTokens(f.Path), HowFull)
	case e.agent == discovery.AgentCursor:
		fm := frontmatter(f.Path)
		switch {
		case strings.EqualFold(fm["alwaysApply"], "true"):
			e.add(f, f.Path, bodyTokens(f.Path), HowFull)
		case fm["description"] != "" && fm["globs"] == "":
			e.add(f, f.Path, tokens(len(fm["description"])+len(base)+4), "description (agent decides)")
		default:
			e.conditionalRules++
		}
	default: // Claude Code: rules with `paths:` apply only to matching files
		if fm := frontmatter(f.Path); fm["paths"] != "" {
			e.conditionalRules++
			return
		}
		e.add(f, f.Path, fileTokens(f.Path), HowFull)
	}
}

func (e *estimator) finish() {
	if len(e.codexProject) > 0 {
		total := 0
		for _, en := range e.codexProject {
			total += en.Tokens
		}
		budget := tokens(codexProjectCap)
		for _, en := range e.codexProject {
			if total > budget {
				en.Tokens = en.Tokens * budget / total
				en.How += " (capped)"
			}
			e.out.Entries = append(e.out.Entries, en)
		}
		if total > budget {
			e.out.Notes = append(e.out.Notes, "Codex reads at most 32 KiB of project AGENTS.md files (project_doc_max_bytes); the rest is cut off.")
		}
	}
	e.out.NotCounted = append(e.out.NotCounted, "the built-in system prompt and tool definitions")
	if e.mcp > 0 {
		e.out.NotCounted = append(e.out.NotCounted, fmt.Sprintf("tool definitions from %d MCP server%s", e.mcp, plural(e.mcp)))
	}
	if e.ctxHooks > 0 {
		e.out.NotCounted = append(e.out.NotCounted, fmt.Sprintf("output of %d SessionStart/UserPromptSubmit hook%s", e.ctxHooks, plural(e.ctxHooks)))
	}
	if e.conditionalRules > 0 {
		e.out.Notes = append(e.out.Notes, fmt.Sprintf("%d conditional rule%s (file patterns or manual) left out; they load only when they apply.", e.conditionalRules, plural(e.conditionalRules)))
	}
	if e.agent == discovery.AgentClaude && e.outputStyle != "" && !e.styleFound {
		e.out.Notes = append(e.out.Notes, "Output style "+e.outputStyle+" is built in or not found on disk; not counted.")
	}
}

// ---------------------------------------------------------------- reading

func tokens(n int) int { return (n + 3) / 4 }

func read(path string) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	b, _ := io.ReadAll(io.LimitReader(f, maxRead))
	if bytes.IndexByte(b[:min(len(b), 8000)], 0) >= 0 {
		return nil // binary
	}
	return b
}

func fileTokens(path string) int { return tokens(len(read(path))) }

// headTokens counts the first n lines of a file.
func headTokens(path string, n int) int {
	sc := bufio.NewScanner(bytes.NewReader(read(path)))
	sc.Buffer(make([]byte, 64*1024), maxRead)
	size := 0
	for i := 0; i < n && sc.Scan(); i++ {
		size += len(sc.Bytes()) + 1
	}
	return tokens(size)
}

// bodyTokens counts a file without its front matter.
func bodyTokens(path string) int {
	_, body := fmatter.ReadFile(path)
	return tokens(len(body))
}

// descTokens is what a "name: description" listing of the file costs.
func descTokens(path, fallbackName string) int {
	fm := frontmatter(path)
	name := fm["name"]
	if name == "" {
		name = fallbackName
	}
	return tokens(len(name) + len(fm["description"]) + 4)
}

func stem(p string) string { return strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)) }

// frontmatter reads the top-level front matter fields of a file.
func frontmatter(path string) map[string]string {
	fields, _ := fmatter.ReadFile(path)
	return fmatter.Map(fields)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// Human formats a token count compactly (950, 12.3k, 1.2M).
func Human(n int) string {
	switch {
	case n >= 1_000_000:
		return trimZero(fmt.Sprintf("%.1f", float64(n)/1e6)) + "M"
	case n >= 1000:
		return trimZero(fmt.Sprintf("%.1f", float64(n)/1e3)) + "k"
	default:
		return fmt.Sprint(n)
	}
}

func trimZero(s string) string { return strings.TrimSuffix(s, ".0") }

// ParseWindow parses a window size such as 200000, 200k or 1m.
func ParseWindow(s string) (int, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	mult := 1
	switch {
	case strings.HasSuffix(s, "k"):
		mult, s = 1000, strings.TrimSuffix(s, "k")
	case strings.HasSuffix(s, "m"):
		mult, s = 1_000_000, strings.TrimSuffix(s, "m")
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f <= 0 {
		return 0, fmt.Errorf("invalid window size %q (examples: 200k, 1m, 400000)", s)
	}
	return int(f * float64(mult)), nil
}
