package preview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/R037/ayca/internal/discovery"
)

func write(t *testing.T, p, s string) string {
	t.Helper()
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadFormats(t *testing.T) {
	d := t.TempDir()
	cases := []struct {
		name   string
		format Format
		lang   string
	}{
		{"CLAUDE.md", FormatMarkdown, ""},
		{"rule.mdc", FormatMarkdown, ""},
		{"config.toml", FormatCode, "toml"},
		{"x.json", FormatCode, "json"},
		{"default.rules", FormatCode, "python"},
		{"notes.txt", FormatText, ""},
	}
	for _, c := range cases {
		p := write(t, filepath.Join(d, c.name), "content")
		got := Load(&discovery.Item{Path: p})
		if got.Format != c.format || got.Lang != c.lang || got.Size != 7 || got.Text != "content" {
			t.Errorf("%s: %+v", c.name, got)
		}
	}
}

func TestLoadSkillDirShowsSkillMD(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "pdf", "SKILL.md"), "# PDF skill")
	got := Load(&discovery.Item{Path: filepath.Join(d, "pdf"), IsDir: true, Category: discovery.CategorySkill})
	if got.Text != "# PDF skill" || got.SourcePath != filepath.Join(d, "pdf", "SKILL.md") {
		t.Fatalf("%+v", got)
	}
}

func TestLoadBinaryMissingAndTruncated(t *testing.T) {
	d := t.TempDir()
	bin := write(t, filepath.Join(d, "x.bin"), "ab\x00cd")
	if got := Load(&discovery.Item{Path: bin}); !got.Binary || got.Text != "" {
		t.Errorf("binary: %+v", got)
	}
	if got := Load(&discovery.Item{Path: filepath.Join(d, "missing.md")}); !strings.Contains(got.Note, "Cannot read") {
		t.Errorf("missing: %+v", got)
	}
	big := write(t, filepath.Join(d, "big.md"), strings.Repeat("a", MaxBytes+10))
	if got := Load(&discovery.Item{Path: big}); !got.Truncated || len(got.Text) != MaxBytes {
		t.Errorf("big: truncated=%v len=%d", got.Truncated, len(got.Text))
	}
}

func TestVirtualMCPShowsConfigOnly(t *testing.T) {
	it := &discovery.Item{Name: "github", Path: "/x/.mcp.json", Virtual: true,
		Config: map[string]any{"command": "gh", "env": map[string]any{"T": discovery.Masked}}}
	got := Load(it)
	if got.Format != FormatCode || got.Lang != "json" || !strings.Contains(got.Text, `"command": "gh"`) {
		t.Fatalf("%+v", got)
	}
}

func TestRenderRawAndRendered(t *testing.T) {
	r := NewRendererStyle("dark")
	c := Content{Text: "# Hello\n\n- **bold** item", Format: FormatMarkdown}
	raw := r.Render(c, 40, true)
	if !strings.Contains(raw, "# Hello") || !strings.Contains(raw, "**bold**") {
		t.Errorf("raw view altered text: %q", raw)
	}
	rendered := ansi.Strip(r.Render(c, 40, false))
	if strings.Contains(rendered, "**bold**") || !strings.Contains(rendered, "bold item") {
		t.Errorf("markdown not rendered: %q", rendered)
	}
	for _, line := range strings.Split(r.Render(Content{Text: strings.Repeat("word ", 50)}, 30, true), "\n") {
		if ansi.StringWidth(line) > 30 {
			t.Fatalf("line wider than 30: %q", line)
		}
	}
}

func TestTokensEstimate(t *testing.T) {
	if got := (Content{Text: strings.Repeat("a", 400)}).Tokens(); got != 100 {
		t.Fatalf("tokens = %d", got)
	}
}
