package discovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSymlinkCycleIsNotFollowed(t *testing.T) {
	f := newFixture(t)
	sk := filepath.Join(f.home, ".claude", "skills", "demo")
	f.write(filepath.Join(sk, "SKILL.md"), "---\nname: demo\ndescription: d\n---\n")
	f.write(filepath.Join(sk, "assets", "a.txt"), "a")
	if err := os.Symlink("..", filepath.Join(sk, "assets", "loop")); err != nil {
		t.Fatal(err)
	}
	// A non-cyclic symlinked directory is still followed.
	f.write(filepath.Join(f.root, "shared", "s.md"), "s")
	if err := os.Symlink(filepath.Join(f.root, "shared"), filepath.Join(sk, "linked")); err != nil {
		t.Fatal(err)
	}
	res := f.discover(f.proj)
	var skill *Item
	for i := range res.Items {
		if res.Items[i].Category == CategorySkill && res.Items[i].Name == "demo" {
			skill = &res.Items[i]
		}
	}
	if skill == nil {
		t.Fatal("skill missing")
	}
	var walk func(items []Item, prefix string, out *[]string)
	walk = func(items []Item, prefix string, out *[]string) {
		for _, it := range items {
			*out = append(*out, prefix+it.Name)
			walk(it.Children, prefix+it.Name, out)
		}
	}
	var all []string
	walk(skill.Children, "", &all)
	joined := strings.Join(all, "\n")
	if strings.Contains(joined, "loop/assets") || strings.Contains(joined, "loop/SKILL") {
		t.Errorf("cycle followed:\n%s", joined)
	}
	if !strings.Contains(joined, "linked/s.md") {
		t.Errorf("non-cyclic symlink dir not followed:\n%s", joined)
	}
	var loop *Item
	for i := range skill.Children {
		if skill.Children[i].Name == "assets/" {
			for j := range skill.Children[i].Children {
				if c := &skill.Children[i].Children[j]; c.Name == "loop/" {
					loop = c
				}
			}
		}
	}
	if loop == nil || !loop.IsDir || len(loop.Children) != 0 || !strings.Contains(loop.Note, "not followed (cycle)") {
		t.Errorf("loop node = %+v", loop)
	}
}

func TestProjectHookScriptRelativeToProjectRoot(t *testing.T) {
	f := newFixture(t)
	f.write(filepath.Join(f.proj, ".claude", "hooks", "guard.sh"), "exit 0")
	f.write(filepath.Join(f.proj, ".claude", "settings.json"),
		`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":".claude/hooks/guard.sh"}]}]}}`)
	res := f.discover(f.proj)
	for _, it := range res.Items {
		if it.Category == CategoryHook && strings.HasPrefix(it.Name, "PreToolUse") {
			if len(it.Children) != 1 || it.Children[0].Name != "guard.sh" {
				t.Errorf("children = %+v", it.Children)
			}
			return
		}
	}
	t.Error("hook missing")
}
