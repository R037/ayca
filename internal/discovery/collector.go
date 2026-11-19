package discovery

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

const (
	maxChildDepth   = 4
	maxChildEntries = 300
)

// collector accumulates items from adapters, de-duplicating by resolved path
// and recording non-fatal problems as warnings.
type collector struct {
	env      Env
	items    []Item
	index    map[string]int
	warnings []Warning
	warned   map[string]bool
	// deferred makes every item added while it is set Deferred.
	deferred    bool
	outputStyle string
}

// deferring runs fn with every item it adds marked Deferred.
func (c *collector) deferring(fn func()) {
	prev := c.deferred
	c.deferred = true
	fn()
	c.deferred = prev
}

func newCollector(env Env) *collector {
	return &collector{env: env, index: map[string]int{}, warned: map[string]bool{}}
}

// warn records err unless it simply means "not there".
func (c *collector) warn(path string, err error) {
	if err == nil || errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscallENOTDIR) {
		return
	}
	if c.warned[path] {
		return
	}
	c.warned[path] = true
	c.warnings = append(c.warnings, Warning{Path: path, Err: err.Error()})
}

// stat follows symlinks; missing paths return ok=false silently, other
// failures (permissions, loops) become warnings.
func (c *collector) stat(path string) (fs.FileInfo, bool) {
	fi, err := os.Stat(path)
	if err != nil {
		c.warn(path, err)
		return nil, false
	}
	return fi, true
}

func realPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

// add merges it into the collection. Two items are the same when they resolve
// to the same real file (or, for virtual items, the same entry in the same
// real config file).
func (c *collector) add(it Item) {
	real := realPath(it.Path)
	key := real
	if it.Virtual {
		// A preset ID acts as an extra discriminator (e.g. two identical
		// hooks in one file).
		key = real + "#" + string(it.Category) + ":" + it.Name
		if it.ID != "" {
			key += "#" + it.ID
		}
	}
	if real != filepath.Clean(it.Path) {
		it.Target = real
	}
	// Anything inside a user-level config directory is global, even when the
	// project happens to be the home directory itself.
	// (Only for paths reached through the project walk: a project-scoped
	// plugin legitimately lives under ~/.claude/plugins.)
	if _, inProject := under(c.env.ProjectRoot, it.Path); inProject && c.env.isGlobalPath(it.Path) {
		it.Scope = ScopeGlobal
	}
	it.ID = key
	if c.deferred {
		it.DeferredFor = slices.Clone(it.Agents)
	}
	assignChildIDs(key, it.Children)
	if i, ok := c.index[key]; ok {
		ex := &c.items[i]
		// An agent loads it at startup if any location that found it for
		// that agent does.
		for _, a := range it.Agents {
			known := slices.Contains(ex.Agents, a)
			switch {
			case !it.Deferred(a):
				ex.DeferredFor = slices.DeleteFunc(ex.DeferredFor, func(x string) bool { return x == a })
			case !known && !ex.Deferred(a):
				ex.DeferredFor = append(ex.DeferredFor, a)
			}
			if !known {
				ex.Agents = append(ex.Agents, a)
			}
		}
		sortAgents(ex.Agents)
		if it.Path != ex.Path && !slices.Contains(ex.Aliases, it.Path) {
			ex.Aliases = append(ex.Aliases, it.Path)
		}
		if ex.Note == "" {
			ex.Note = it.Note
		}
		return
	}
	it.Agents = slices.Clone(it.Agents)
	sortAgents(it.Agents)
	c.index[key] = len(c.items)
	c.items = append(c.items, it)
}

func sortAgents(a []string) {
	order := func(s string) int {
		if i := slices.Index(AllAgents, s); i >= 0 {
			return i
		}
		return len(AllAgents)
	}
	sort.SliceStable(a, func(i, j int) bool { return order(a[i]) < order(a[j]) })
}

// assignChildIDs gives children without an ID one derived from their parent.
func assignChildIDs(parent string, items []Item) {
	for i := range items {
		if items[i].ID == "" {
			items[i].ID = parent + "/" + items[i].Name
		}
		assignChildIDs(items[i].ID, items[i].Children)
	}
}

// file adds a regular file if it exists.
func (c *collector) file(path string, cat Category, scope Scope, note string, agents ...string) bool {
	fi, ok := c.stat(path)
	if !ok || !fi.Mode().IsRegular() {
		return false
	}
	c.add(Item{
		Name:     c.env.DisplayPath(path),
		Path:     path,
		Category: cat,
		Scope:    scope,
		Agents:   agents,
		Note:     note,
	})
	return true
}

// ruleDir adds a directory of rule files (see fileDir).
func (c *collector) ruleDir(dir string, exts []string, scope Scope, note string, agents ...string) {
	c.fileDir(dir, "", CategoryRule, exts, scope, note, agents...)
}

// fileDir adds dir as a group containing every file (recursively) whose
// extension is in exts. Nothing is added when no matching files exist.
// name overrides the displayed name (default: the directory's path).
func (c *collector) fileDir(dir, name string, cat Category, exts []string, scope Scope, note string, agents ...string) {
	fi, ok := c.stat(dir)
	if !ok || !fi.IsDir() {
		return
	}
	parentID := realPath(dir)
	children := c.listDir(dir, parentID, cat, scope, agents, func(name string) bool {
		return slices.Contains(exts, strings.ToLower(filepath.Ext(name)))
	}, 0)
	children = pruneEmptyDirs(children)
	if len(children) == 0 {
		return
	}
	if name == "" {
		name = c.env.DisplayPath(dir) + "/"
	}
	c.add(Item{
		Name:     name,
		Path:     dir,
		Category: cat,
		Scope:    scope,
		Agents:   agents,
		IsDir:    true,
		Note:     note,
		Children: children,
	})
}

// skills adds every skill (a directory containing SKILL.md) found under dir,
// looking up to three levels deep to cover grouped/synced layouts.
func (c *collector) skills(dir string, scope Scope, note string, agents ...string) {
	c.skillsAt(dir, scope, note, agents, 0)
}

func (c *collector) skillsAt(dir string, scope Scope, note string, agents []string, depth int) {
	if depth > 2 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		c.warn(dir, err)
		return
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		fi, ok := c.stat(p)
		if !ok || !fi.IsDir() {
			continue
		}
		skillFile := filepath.Join(p, "SKILL.md")
		if sf, ok := c.stat(skillFile); ok && sf.Mode().IsRegular() {
			id := realPath(p)
			c.add(Item{
				Name:     e.Name(),
				Path:     p,
				Category: CategorySkill,
				Scope:    scope,
				Agents:   agents,
				IsDir:    true,
				Note:     note,
				Children: c.listDir(p, id, CategorySkill, scope, agents, nil, 0),
			})
			continue
		}
		c.skillsAt(p, scope, note, agents, depth+1)
	}
}

// listDir returns child items for the contents of dir. keep filters files
// (nil keeps everything); directories are always descended into.
func (c *collector) listDir(dir, parentID string, cat Category, scope Scope, agents []string, keep func(string) bool, depth int) []Item {
	if depth >= maxChildDepth {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		c.warn(dir, err)
		return nil
	}
	// Directories first, then files, each alphabetical (os.ReadDir sorts by name).
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].IsDir() && !entries[j].IsDir()
	})
	var out []Item
	for _, e := range entries {
		if len(out) >= maxChildEntries {
			c.warnings = append(c.warnings, Warning{Path: dir, Err: "too many entries; listing truncated"})
			break
		}
		if e.Name() == ".git" || e.Name() == "node_modules" {
			continue
		}
		p := filepath.Join(dir, e.Name())
		fi, ok := c.stat(p)
		if !ok {
			continue
		}
		id := parentID + "/" + e.Name()
		if fi.IsDir() {
			out = append(out, Item{
				ID: id, Name: e.Name() + "/", Path: p, Category: cat, Scope: scope,
				Agents: agents, IsDir: true,
				Children: c.listDir(p, id, cat, scope, agents, keep, depth+1),
			})
			continue
		}
		if !fi.Mode().IsRegular() || (keep != nil && !keep(e.Name())) {
			continue
		}
		out = append(out, Item{ID: id, Name: e.Name(), Path: p, Category: cat, Scope: scope, Agents: agents})
	}
	return out
}

// pruneEmptyDirs drops directory children that ended up with no files.
func pruneEmptyDirs(items []Item) []Item {
	out := items[:0]
	for _, it := range items {
		if it.IsDir {
			it.Children = pruneEmptyDirs(it.Children)
			if len(it.Children) == 0 {
				continue
			}
		}
		out = append(out, it)
	}
	return out
}
