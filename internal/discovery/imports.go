package discovery

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Claude Code expands "@path" references in CLAUDE.md files (and in the files
// they import), up to five levels deep. References inside code are ignored.
const maxImportDepth = 5

var (
	fencedCode   = regexp.MustCompile("(?s)(```|~~~).*?(```|~~~)")
	inlineCode   = regexp.MustCompile("`[^`\n]*`")
	importToken  = regexp.MustCompile(`(?:^|[\s(\[])@((?:~/|/|\.{1,2}/)?[A-Za-z0-9_.\-/~]+)`)
	hasExtension = regexp.MustCompile(`\.[A-Za-z0-9]{1,8}$`)
)

// resolveImports attaches imported files as children of Claude instruction
// files. Missing targets that clearly look like paths are kept (flagged
// Missing) so broken references are visible.
func (c *collector) resolveImports() {
	for i := range c.items {
		it := &c.items[i]
		if it.Category != CategoryInstruction || it.IsDir || it.Virtual || !slices.Contains(it.Agents, AgentClaude) {
			continue
		}
		seen := map[string]bool{realPath(it.Path): true}
		it.Children = append(it.Children, c.imports(it.Path, it.ID, it.Scope, seen, 0)...)
	}
}

func (c *collector) imports(file, parentID string, scope Scope, seen map[string]bool, depth int) []Item {
	if depth >= maxImportDepth {
		return nil
	}
	f, err := os.Open(file)
	if err != nil {
		return nil
	}
	data, _ := io.ReadAll(io.LimitReader(f, 1<<20))
	f.Close()

	text := fencedCode.ReplaceAllString(string(data), "")
	text = inlineCode.ReplaceAllString(text, "")
	var out []Item
	for _, m := range importToken.FindAllStringSubmatch(text, -1) {
		tok := strings.TrimRight(m[1], ".,;:)]")
		if tok == "" {
			continue
		}
		p := c.resolveImportPath(tok, filepath.Dir(file))
		id := parentID + "@" + tok
		if !fileExists(p) {
			if strings.Contains(tok, "/") || hasExtension.MatchString(tok) {
				if !seen["missing:"+p] {
					seen["missing:"+p] = true
					out = append(out, Item{ID: id, Name: "@" + tok, Path: p, Category: CategoryInstruction, Scope: scope,
						Agents: []string{AgentClaude}, Missing: true, Found: importFinding(p),
						Note: "imported by " + c.env.DisplayPath(file) + " — file not found"})
				}
			}
			continue
		}
		real := realPath(p)
		if seen[real] {
			continue // already imported (or an import cycle)
		}
		seen[real] = true
		child := Item{ID: id, Name: "@" + tok, Path: p, Category: CategoryInstruction, Scope: scope,
			Agents: []string{AgentClaude}, Found: importFinding(p), Note: "imported by " + c.env.DisplayPath(file)}
		child.Children = c.imports(p, id, scope, seen, depth+1)
		out = append(out, child)
	}
	return out
}

func (c *collector) resolveImportPath(tok, dir string) string {
	switch {
	case strings.HasPrefix(tok, "~/"):
		return filepath.Join(c.env.Home, tok[2:])
	case filepath.IsAbs(tok):
		return filepath.Clean(tok)
	default:
		return filepath.Join(dir, tok)
	}
}

func importFinding(p string) []Finding {
	return []Finding{{Agent: AgentClaude, Path: p, Mechanism: MechImport}}
}
