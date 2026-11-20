package preview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/R037/ayca/internal/frontmatter"
	"github.com/R037/ayca/internal/testfixture"
)

func find(refs []Ref, suffix string) *Ref {
	for i := range refs {
		if strings.HasSuffix(refs[i].Target, suffix) {
			return &refs[i]
		}
	}
	return nil
}

func TestFrontmatterSeparate(t *testing.T) {
	f := testfixture.New(t)
	p := f.H(".claude/skills/frontend-design/SKILL.md")
	c := LoadPath(p)
	if frontmatter.Get(c.Frontmatter, "name") != "frontend-design" || len(c.Frontmatter) != 3 {
		t.Fatalf("%+v", c.Frontmatter)
	}
	if strings.Contains(c.Body, "allowed-tools") || !strings.HasPrefix(strings.TrimSpace(c.Body), "# Frontend design") {
		t.Fatalf("body: %q", c.Body)
	}
	r := NewRendererStyle("notty")
	for _, w := range []int{30, 80} {
		out := ansi.Strip(r.Render(c, w, false))
		if !strings.Contains(out, "Frontmatter") || !strings.Contains(out, "distinctive") || strings.Contains(out, "---\nname") {
			t.Errorf("w=%d:\n%s", w, out)
		}
		for _, l := range strings.Split(out, "\n") {
			if ansi.StringWidth(l) > w {
				t.Errorf("w=%d wide line %q", w, l)
			}
		}
	}
	raw, _ := os.ReadFile(p)
	if got := r.Render(c, 10000, true); strings.TrimRight(got, "\n") != strings.TrimRight(string(raw), "\n") {
		t.Errorf("raw differs: %q", got)
	}
	bad := Content{Text: "---\nname: x\nno end", Format: FormatMarkdown}
	if out := ansi.Strip(r.Render(bad, 40, false)); strings.Contains(out, "Frontmatter") {
		t.Errorf("malformed: %q", out)
	}
}

func TestReferences(t *testing.T) {
	f := testfixture.New(t)
	sk := f.H(".claude/skills/frontend-design")
	c := LoadPath(filepath.Join(sk, "SKILL.md"))
	refs := References(c, RefOptions{Home: f.Home})
	want := map[string]RefStatus{
		"references/patterns.md": RefOK, "references/accessibility.md": RefOK,
		"scripts/validate.sh": RefOK, "assets/template.json": RefOK,
		"references/missing.md": RefMissing, "https://example.com/design-guide": RefExternal,
	}
	for suf, st := range want {
		r := find(refs, suf)
		if r == nil || r.Status != st || r.Line == 0 {
			t.Errorf("%s: %+v", suf, r)
		}
	}
	if len(refs) != len(want) {
		t.Errorf("refs: %+v", refs)
	}
	if r := find(refs, "validate.sh"); r.Kind != RefPath {
		t.Errorf("kind %v", r.Kind)
	}
	if r := find(refs, "patterns.md"); r.Kind != RefLink {
		t.Errorf("kind %v", r.Kind)
	}
	out := References(c, RefOptions{Home: f.Home, Allowed: func(string) bool { return false }})
	if r := find(out, "patterns.md"); r.Status != RefOutside {
		t.Errorf("outside: %+v", r)
	}
	if r := find(out, "missing.md"); r.Status != RefMissing {
		t.Errorf("missing stays missing: %+v", r)
	}

	pc := LoadPath(filepath.Join(sk, "references/patterns.md"))
	prefs := References(pc, RefOptions{Home: f.Home})
	if r := find(prefs, filepath.Join(sk, "SKILL.md")); r == nil || r.Status != RefOK {
		t.Errorf("%+v", prefs)
	}
	if r := find(prefs, "references/accessibility.md"); r == nil || r.Status != RefOK {
		t.Errorf("%+v", prefs)
	}

	pdf := LoadPath(f.H(".agents/skills/pdf/SKILL.md"))
	if r := find(References(pdf, RefOptions{Home: f.Home}), "pdf/reference.md"); r == nil || r.Status != RefOK {
		t.Errorf("reference.md")
	}
}

func TestReferencesFenceImportsAndHome(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "scripts/extract.py"), "x")
	write(t, filepath.Join(d, "home/notes.md"), "x")
	p := write(t, filepath.Join(d, "CLAUDE.md"), "See @docs/missing.md and @~/notes.md\n\n`@docs/incode.md`\n\n```\npython scripts/extract.py out/result.txt\n```\n[a]: other.md \"T\"\n[sp](my%20file.md#top) [s](#only)\n")
	c := LoadPath(p)
	refs := References(c, RefOptions{Home: filepath.Join(d, "home")})
	if r := find(refs, "docs/missing.md"); r == nil || r.Kind != RefImport || r.Status != RefMissing {
		t.Errorf("import: %+v", refs)
	}
	if r := find(refs, "home/notes.md"); r == nil || r.Status != RefOK {
		t.Errorf("home: %+v", refs)
	}
	if find(refs, "incode.md") != nil || find(refs, "result.txt") != nil {
		t.Errorf("unexpected: %+v", refs)
	}
	if r := find(refs, "scripts/extract.py"); r == nil || r.Status != RefOK {
		t.Errorf("fence existing: %+v", refs)
	}
	if find(refs, "other.md") == nil || find(refs, "my file.md") == nil {
		t.Errorf("links: %+v", refs)
	}
	for _, r := range refs {
		if strings.Contains(r.Target, "#") {
			t.Errorf("anchor kept: %+v", r)
		}
	}
}

func TestScriptsAndEnv(t *testing.T) {
	f := testfixture.New(t)
	c := LoadPath(f.H(".claude/skills/frontend-design/scripts/check"))
	if c.Format != FormatCode || c.Lang != "python" || c.Lines != 3 {
		t.Errorf("%+v", c)
	}
	d := t.TempDir()
	for in, want := range map[string]string{
		"#!/bin/bash\n": "bash", "#!/usr/bin/env -S node\n": "javascript", "#!/usr/bin/env deno\n": "typescript",
		"#!/usr/bin/perl\n": "perl", "echo hi": "",
	} {
		if got := LoadPath(write(t, filepath.Join(d, "s"), in)).Lang; got != want {
			t.Errorf("%q: %q", in, got)
		}
	}
	e := LoadPath(write(t, filepath.Join(d, ".env"), "SECRET=abc"))
	if e.Text != "" || !strings.Contains(e.Note, "values not shown") {
		t.Errorf("%+v", e)
	}
	if LoadPath(write(t, filepath.Join(d, "a.tsx"), "x")).Lang != "tsx" {
		t.Error("tsx")
	}
}

func TestSearchHelpers(t *testing.T) {
	r := "\x1b[1mHello World\x1b[0m\nsecond\nworld again"
	if got := FindLines(r, "WORLD"); len(got) != 2 || got[0] != 0 || got[1] != 2 {
		t.Errorf("%v", got)
	}
	if FindLines(r, "") != nil {
		t.Error("empty")
	}
	if LineOf(r, "second") != 1 || LineOf(r, "nope") != -1 {
		t.Error("LineOf")
	}
	h := HighlightLine(r, 1, lipgloss.NewStyle().Bold(true))
	lines := strings.Split(h, "\n")
	if lines[0] != strings.Split(r, "\n")[0] || lines[2] != "world again" || ansi.Strip(lines[1]) != "second" {
		t.Errorf("%q", h)
	}
	if HighlightLine(r, 9, lipgloss.NewStyle()) != r {
		t.Error("oob")
	}
}

func TestBreadcrumb(t *testing.T) {
	parts := []string{"~/.claude", "skills", "frontend-design", "SKILL.md"}
	if got := Breadcrumb(parts, 100); got != "~/.claude › skills › frontend-design › SKILL.md" {
		t.Errorf("%q", got)
	}
	got := Breadcrumb(parts, 30)
	if !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "SKILL.md") || ansi.StringWidth(got) > 30 {
		t.Errorf("%q", got)
	}
	got = Breadcrumb(parts, 6)
	if ansi.StringWidth(got) > 6 || !strings.HasSuffix(got, "L.md") {
		t.Errorf("%q", got)
	}
}
