package tree

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/R037/ayca/internal/discovery"
)

// fsTrie is a directory trie holding what lives at each physical path.
type fsTrie struct {
	name     string
	path     string
	children map[string]*fsTrie
	own      []*Node           // item and link nodes located exactly here
	virtual  []*discovery.Item // virtual items configured in the file here
}

func newTrie(name, path string) *fsTrie {
	return &fsTrie{name: name, path: path, children: map[string]*fsTrie{}}
}

func (t *fsTrie) at(root, p string) *fsTrie {
	rel, err := filepath.Rel(root, p)
	if err != nil || rel == "." {
		return t
	}
	cur := t
	for _, seg := range strings.Split(rel, string(filepath.Separator)) {
		next := cur.children[seg]
		if next == nil {
			next = newTrie(seg, filepath.Join(cur.path, seg))
			cur.children[seg] = next
		}
		cur = next
	}
	return cur
}

type fsRoot struct {
	key, label, help, dir string
	trie                  *fsTrie
}

// BuildFilesystem shows the real directories and files behind the context.
// Symlinks and cross-agent aliases appear as link nodes pointing at the
// physical location. Node IDs are prefixed "fs:".
func BuildFilesystem(items []discovery.Item, env discovery.Env) []*Node {
	mk := func(key, label, help, dir string) *fsRoot {
		return &fsRoot{key: key, label: label, help: help, dir: dir, trie: newTrie("", dir)}
	}
	project := mk("project", "Project — "+env.Tilde(env.ProjectRoot), "The project directory and everything inside it.", env.ProjectRoot)
	parents := mk("parents", "Parent directories", "Directories above the project root.", "/")
	home := mk("home", "Home — ~", "Your home directory: user-level configuration of every agent.", env.Home)
	system := mk("system", "System — "+env.Tilde(env.ManagedDir), "Machine-wide managed policy.", env.ManagedDir)
	other := mk("other", "Elsewhere", "Other locations on disk.", "/")

	inDir := func(dir, p string) bool {
		if dir == "" {
			return false
		}
		rel, err := filepath.Rel(dir, p)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
	}
	classify := func(p string, sc discovery.Scope) *fsRoot {
		switch {
		case inDir(env.ProjectRoot, p):
			return project
		case sc == discovery.ScopeAncestor:
			return parents
		case inDir(env.Home, p):
			return home
		case inDir(env.ManagedDir, p):
			return system
		}
		return other
	}

	seen := map[string]bool{} // link paths already placed
	for i := range items {
		it := &items[i]
		if it.Category == discovery.CategoryRuntime || it.Path == "" {
			continue
		}
		phys := filepath.Clean(it.Path)
		if it.Target != "" {
			phys = filepath.Clean(it.Target)
		}
		var links []string
		add := func(p string) {
			p = filepath.Clean(p)
			if p != phys && !seen[p] && !containsStr(links, p) {
				links = append(links, p)
			}
		}
		add(it.Path)
		for _, a := range it.Aliases {
			add(a)
		}

		rt := classify(phys, it.Scope)
		node := rt.trie.at(rt.dir, phys)
		if it.Virtual {
			node.virtual = append(node.virtual, it)
		} else {
			n := ItemNode("fs:", it)
			n.Label = filepath.Base(phys)
			if len(links) > 0 {
				n.Badge = fmt.Sprintf("⇆ %d locations", len(links)+1)
			}
			node.own = append(node.own, n)
		}
		if it.Virtual {
			continue
		}
		for _, l := range links {
			seen[l] = true
			lr := classify(l, it.Scope)
			lr.trie.at(lr.dir, l).own = append(lr.trie.at(lr.dir, l).own, &Node{
				ID: "fs:link:" + l, Label: filepath.Base(l), Kind: KindItem, Scope: it.Scope,
				Item: it, Link: true, Badge: "→ " + env.Tilde(phys),
			})
		}
	}

	var roots []*Node
	for _, r := range []*fsRoot{project, parents, home, system, other} {
		kids := r.emitChildren(env)
		kids = append(r.trie.emitOwn(env), kids...)
		if len(kids) == 0 {
			continue
		}
		roots = append(roots, NewGroup("fs:root:"+r.key, r.label, r.help, kids...))
	}
	return roots
}

func containsStr(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func (r *fsRoot) emitChildren(env discovery.Env) []*Node { return r.trie.emitKids(env) }

func (t *fsTrie) sortedKids() []*fsTrie {
	var out []*fsTrie
	for _, c := range t.children {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

func (t *fsTrie) pure() bool { return len(t.own) == 0 && len(t.virtual) == 0 }

func (t *fsTrie) emitKids(env discovery.Env) []*Node {
	var out []*Node
	for _, c := range t.sortedKids() {
		out = append(out, c.emit(env)...)
	}
	return out
}

// emitOwn returns the nodes for what lives at t itself.
func (t *fsTrie) emitOwn(env discovery.Env) []*Node {
	out := append([]*Node(nil), t.own...)
	if len(t.virtual) > 0 {
		cfg := &Node{ID: "fs:cfg:" + t.path, Label: filepath.Base(t.path), Kind: KindDir,
			Badge: "config file", Path: t.path,
			Help: "Config file `" + t.path + "`. It defines the hooks or MCP servers listed below; AYCA reads it but never changes it."}
		for _, v := range t.virtual {
			cfg.Children = append(cfg.Children, ItemNode("fs:", v))
		}
		out = append(out, cfg)
	}
	return out
}

func (t *fsTrie) emit(env discovery.Env) []*Node {
	if t.pure() {
		label, cur := t.name, t
		for len(cur.children) == 1 {
			c := cur.sortedKids()[0]
			if !c.pure() {
				break
			}
			label += "/" + c.name
			cur = c
		}
		return []*Node{{ID: "fs:dir:" + cur.path, Label: label + "/", Kind: KindDir, Path: cur.path,
			Help: dirHelp(cur.path, env), Children: cur.emitKids(env)}}
	}
	out := t.emitOwn(env)
	if len(t.children) > 0 {
		kids := t.emitKids(env)
		host := -1
		for i, n := range out {
			if n.Kind == KindItem && !n.Link {
				host = i
				break
			}
		}
		if host >= 0 {
			out[host].Children = append(out[host].Children, kids...)
		} else {
			out = append(out, &Node{ID: "fs:dir:" + t.path, Label: t.name + "/", Kind: KindDir, Path: t.path,
				Help: dirHelp(t.path, env), Children: kids})
		}
	}
	return out
}

// dirHelp describes a directory in Markdown.
func dirHelp(p string, env discovery.Env) string {
	known := map[string]string{
		env.ClaudeDir:                                       "Claude Code user config",
		filepath.Join(env.ClaudeDir, "skills"):              "user skills (Claude Code)",
		filepath.Join(env.ClaudeDir, "rules"):               "user rules (Claude Code)",
		filepath.Join(env.ClaudeDir, "agents"):              "user subagents (Claude Code)",
		env.CodexHome:                                       "Codex home",
		filepath.Join(env.CodexHome, "skills"):              "user skills (Codex)",
		filepath.Join(env.CodexHome, "prompts"):             "custom prompts (Codex)",
		env.CursorDir:                                       "Cursor user config",
		filepath.Join(env.CursorDir, "skills"):              "user skills (Cursor)",
		env.AgentsDir:                                       "shared agent files",
		filepath.Join(env.AgentsDir, "skills"):              "shared skills (Codex)",
		env.ManagedDir:                                      "Claude Code managed policy",
		filepath.Join(env.ProjectRoot, ".claude"):           "Claude Code project config",
		filepath.Join(env.ProjectRoot, ".cursor"):           "Cursor project config",
		filepath.Join(env.ProjectRoot, ".agents"):           "shared agent files (project)",
		filepath.Join(env.ProjectRoot, ".agents", "skills"): "project skills (Codex, Cursor)",
		filepath.Join(env.ProjectRoot, ".cursor", "rules"):  "project rules (Cursor)",
		filepath.Join(env.ProjectRoot, ".claude", "rules"):  "project rules (Claude Code)",
		filepath.Join(env.ProjectRoot, ".claude", "skills"): "project skills (Claude Code)",
	}
	s := "`" + p + "`"
	if d, ok := known[p]; ok && p != "" {
		s += "\n\n" + strings.ToUpper(d[:1]) + d[1:] + "."
	}
	return s
}
