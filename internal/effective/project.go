package effective

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/R037/ayca/internal/discovery"
)

// GroupKey identifies one of the five projection groups.
type GroupKey string

const (
	GroupApplicable   GroupKey = "applicable"
	GroupDiscoverable GroupKey = "discoverable"
	GroupConditional  GroupKey = "conditional"
	GroupNotLoaded    GroupKey = "not-loaded"
	GroupUnknown      GroupKey = "unknown"
)

// Entry is one classified item in a projection.
type Entry struct {
	Chain []*discovery.Item // ancestors first; the last item is the entry
	Load  Load
}

// Item returns the entry's own item (the last of its chain).
func (e Entry) Item() *discovery.Item { return e.Chain[len(e.Chain)-1] }

// Group is a set of entries that load the same way.
type Group struct {
	Key     GroupKey
	Label   string
	Help    string
	Entries []Entry
}

// Projection is the effective context of one agent from one working directory.
type Projection struct {
	Agent   string
	WorkDir string
	Groups  []Group // always all five, in order, possibly empty
	// Excluded lists items in the full inventory for this agent that are
	// not found from WorkDir.
	Excluded []Entry
}

var groupOrder = []Group{
	{Key: GroupApplicable, Label: "Applicable — loaded at startup",
		Help: "Read at the start of every session from this working directory. These take up context before you type anything."},
	{Key: GroupDiscoverable, Label: "Discoverable — listed, loaded when used",
		Help: "Only the name and description are in context at startup. The full content loads when the agent or you invoke it."},
	{Key: GroupConditional, Label: "Conditional / on demand",
		Help: "Loaded only when a condition is met: the agent works in a matching directory or file, you invoke it, or an event fires."},
	{Key: GroupNotLoaded, Label: "Not loaded",
		Help: "Found on disk but not loaded: disabled, shadowed by another file, ignored by this agent here, or enforced by the program rather than sent to the model."},
	{Key: GroupUnknown, Label: "Unknown / runtime-dependent",
		Help: "Depends on things AYCA cannot see from disk, such as MCP servers starting, account connectors or launch flags."},
}

func groupOf(s State) GroupKey {
	switch s {
	case Startup:
		return GroupApplicable
	case Listed:
		return GroupDiscoverable
	case OnDemand, Conditional, Invocation, Event:
		return GroupConditional
	case Disabled, Shadowed, NotLoaded, Enforced:
		return GroupNotLoaded
	}
	return GroupUnknown
}

// expandable categories whose directories are shown as their individual files.
func expands(it *discovery.Item) bool {
	if !it.IsDir {
		return false
	}
	switch it.Category {
	case discovery.CategoryRule, discovery.CategoryAgent, discovery.CategoryCommand,
		discovery.CategoryOutputStyle, discovery.CategoryMemory:
		return true
	}
	return false
}

// chains returns the entry chains for a top-level item.
func chains(top *discovery.Item) [][]*discovery.Item {
	switch {
	case top.Category == discovery.CategoryInstruction && !top.IsDir:
		out := [][]*discovery.Item{{top}}
		var walk func(chain []*discovery.Item)
		walk = func(chain []*discovery.Item) {
			for i := range chain[len(chain)-1].Children {
				c := append(slices.Clone(chain), &chain[len(chain)-1].Children[i])
				out = append(out, c)
				walk(c)
			}
		}
		walk([]*discovery.Item{top})
		return out
	case expands(top):
		return leaves([]*discovery.Item{top})
	}
	return [][]*discovery.Item{{top}}
}

func entriesFor(res discovery.Result, agent string, classify func(chain []*discovery.Item) Load) []Entry {
	var out []Entry
	for i := range res.Items {
		top := &res.Items[i]
		if !slices.Contains(top.Agents, agent) {
			continue
		}
		for _, c := range chains(top) {
			out = append(out, Entry{Chain: c, Load: classify(c)})
		}
	}
	return out
}

func allIDs(items []discovery.Item, set map[string]bool) {
	for i := range items {
		set[items[i].ID] = true
		allIDs(items[i].Children, set)
	}
}

// Project classifies `at` (a discovery run with Env.Cwd set to the working
// directory) for agent. full is a discovery run at the project root, used
// for Excluded; pass at again when they are the same.
func Project(at, full discovery.Result, agent string) Projection {
	ctx := ContextOf(at)
	p := Projection{Agent: agent, WorkDir: at.Env.Cwd}
	for _, g := range groupOrder {
		p.Groups = append(p.Groups, g)
	}
	for _, e := range entriesFor(at, agent, func(c []*discovery.Item) Load { return ClassifyPath(c, agent, ctx) }) {
		k := groupOf(e.Load.State)
		for i := range p.Groups {
			if p.Groups[i].Key == k {
				p.Groups[i].Entries = append(p.Groups[i].Entries, e)
			}
		}
	}

	present := map[string]bool{}
	allIDs(at.Items, present)
	fctx := ContextOf(full)
	for _, e := range entriesFor(full, agent, func(c []*discovery.Item) Load { return ClassifyPath(c, agent, fctx) }) {
		if present[e.Item().ID] {
			continue
		}
		e.Load = Load{Agent: agent, State: NotLoaded, Confidence: ConfDocumented, Reason: excludedReason(at.Env, e.Item())}
		p.Excluded = append(p.Excluded, e)
	}
	return p
}

// excludedReason explains why an item is not found from the working directory.
func excludedReason(env discovery.Env, it *discovery.Item) string {
	rel, ok := under(env.ProjectRoot, it.Path)
	if !ok {
		return "Not found from this working directory"
	}
	// Keep the leading plain directories (stop at .claude, .cursor, ...).
	var keep []string
	for _, seg := range strings.Split(filepath.Dir(rel), string(filepath.Separator)) {
		if strings.HasPrefix(seg, ".") {
			break
		}
		keep = append(keep, seg)
	}
	dir := "."
	if len(keep) > 0 {
		dir = strings.Join(keep, "/")
	}
	if dir == "." {
		return "Not found from this working directory"
	}
	cwd := "the project root"
	if r, ok := under(env.ProjectRoot, env.Cwd); ok {
		cwd = r + "/"
	}
	return fmt.Sprintf("In %s/, which is not on the path from the project root to %s — not loaded from this working directory", dir, cwd)
}

// projectDirOf returns the project directory an instruction or rule item
// belongs to ("" when it is not a project-scoped one).
func projectDirOf(it *discovery.Item, root string) string {
	if it.Scope != discovery.ScopeProject {
		return ""
	}
	if it.Category != discovery.CategoryInstruction && it.Category != discovery.CategoryRule {
		return ""
	}
	d := filepath.Dir(it.Path) // for a rules dir: .../.claude/rules -> .../.claude
	if strings.HasPrefix(filepath.Base(d), ".") && d != root {
		d = filepath.Dir(d)
	}
	if d != root {
		if _, ok := under(root, d); !ok {
			return ""
		}
	}
	return d
}

// CandidateDirs lists working directories worth projecting: the project
// root, the launch cwd and every directory between, and each project
// directory containing discovered instructions or rules. Absolute, deduped,
// root first, then sorted.
func CandidateDirs(base, full discovery.Result) []string {
	root := full.Env.ProjectRoot
	set := map[string]bool{}
	for d := base.Env.Cwd; ; d = filepath.Dir(d) {
		if d == root {
			break
		}
		if _, ok := under(root, d); !ok {
			break
		}
		set[d] = true
	}
	for _, res := range []discovery.Result{full, base} {
		for i := range res.Items {
			if d := projectDirOf(&res.Items[i], root); d != "" {
				set[d] = true
			}
		}
	}
	delete(set, root)
	dirs := make([]string, 0, len(set))
	for d := range set {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	return append([]string{root}, dirs...)
}
