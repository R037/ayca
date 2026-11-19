package discovery

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// Adapter discovers the context locations of one coding agent.
type Adapter struct {
	Name     string
	discover func(c *collector)
}

// Adapters lists all supported agents. Adding an agent means adding an entry
// here and a discover function (see claude.go, codex.go, cursor.go).
var Adapters = []Adapter{
	{AgentClaude, discoverClaude},
	{AgentCodex, discoverCodex},
	{AgentCursor, discoverCursor},
}

// Discover runs the adapters for the requested agents (all when empty) and
// returns de-duplicated items. It never modifies the filesystem and never
// executes anything it finds (hooks, MCP servers, scripts).
func Discover(env Env, agents []string) Result {
	c := newCollector(env)
	enabled := func(a string) bool { return len(agents) == 0 || slices.Contains(agents, a) }
	for _, a := range Adapters {
		if enabled(a.Name) {
			a.discover(c)
		}
	}
	c.deferring(func() { discoverNested(c, enabled) })
	if enabled(AgentClaude) {
		c.resolveImports()
	}
	return Result{Env: env, Items: c.items, Warnings: c.warnings, OutputStyle: c.outputStyle}
}

// ---------------------------------------------------------------- nested

const maxNestedDirs = 4000

// skipDirs are never descended into when looking for nested context.
var skipDirs = map[string]bool{
	"node_modules": true, "vendor": true, "dist": true, "build": true, "target": true,
	"venv": true, "__pycache__": true, "Pods": true, "DerivedData": true,
}

// discoverNested walks below the working directory for context files that
// agents load on demand when they work inside a subdirectory.
func discoverNested(c *collector, enabled func(string) bool) {
	e := c.env
	visited, stopped := 0, false
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if depth > 6 || stopped {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			c.warn(dir, err)
			return
		}
		for _, ent := range entries {
			if !ent.IsDir() { // symlinked dirs are deliberately not followed
				continue
			}
			name := ent.Name()
			if strings.HasPrefix(name, ".") || skipDirs[name] {
				continue
			}
			if visited >= maxNestedDirs {
				c.warnings = append(c.warnings, Warning{Path: e.Cwd, Err: "nested scan stopped after 4000 directories"})
				stopped = true
				return
			}
			visited++
			sub := filepath.Join(dir, name)
			rel, _ := filepath.Rel(e.ProjectRoot, sub)
			note := "nested — loaded on demand when the agent works in " + rel + "/"
			if enabled(AgentClaude) {
				c.via(MechNestedInstructions, func() {
					c.file(filepath.Join(sub, "CLAUDE.md"), CategoryInstruction, ScopeProject, note, AgentClaude)
					c.file(filepath.Join(sub, "CLAUDE.local.md"), CategoryInstruction, ScopeProject, note, AgentClaude)
				})
			}
			var agentsReaders []string
			if enabled(AgentCodex) {
				agentsReaders = append(agentsReaders, AgentCodex)
			}
			if enabled(AgentCursor) {
				agentsReaders = append(agentsReaders, AgentCursor)
			}
			if len(agentsReaders) > 0 {
				c.via(MechNestedInstructions, func() {
					c.file(filepath.Join(sub, "AGENTS.md"), CategoryInstruction, ScopeProject, note, agentsReaders...)
				})
			}
			if enabled(AgentCursor) {
				c.via(MechNestedRules, func() {
					c.ruleDir(filepath.Join(sub, ".cursor", "rules"), []string{".mdc", ".md"}, ScopeProject, note, AgentCursor)
				})
			}
			walk(sub, depth+1)
		}
	}
	// Scanning everything below $HOME or / is slow and mostly noise.
	if e.Cwd == e.Home || filepath.Dir(e.Cwd) == e.Cwd {
		return
	}
	walk(e.Cwd, 0)
}

// ---------------------------------------------------------------- helpers

var mdExts = []string{".md"}

func joinNote(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + "; " + b
}

func sortStrings(s []string) { sort.Strings(s) }

// uniqueDirs returns dirs without duplicates, keeping order.
func uniqueDirs(dirs ...string) []string {
	var out []string
	for _, d := range dirs {
		if d != "" && !slices.Contains(out, d) {
			out = append(out, d)
		}
	}
	return out
}
