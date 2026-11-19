// Package preview loads and renders the content of a discovered item. It
// only ever reads files.
package preview

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/x/ansi"

	"ayca/internal/discovery"
)

// MaxBytes caps how much of a file is read for preview.
const MaxBytes = 1 << 20

// Format is how content is interpreted.
type Format int

const (
	FormatText Format = iota
	FormatMarkdown
	FormatCode
)

// Content is an item's loaded (not yet rendered) content.
type Content struct {
	SourcePath string // the file actually shown (SKILL.md for a skill folder)
	Text       string
	Format     Format
	Lang       string // for FormatCode
	Size       int64  // bytes on disk (-1 when not a single file)
	Truncated  bool
	Binary     bool
	Note       string // explanation shown instead of / above content
}

// Tokens is a rough token estimate (≈4 bytes per token), clearly an
// approximation: real counts depend on each model's tokenizer.
func (c Content) Tokens() int {
	if c.Binary || c.Text == "" {
		return 0
	}
	return (len(c.Text) + 3) / 4
}

var codeLangs = map[string]string{
	".json": "json", ".jsonc": "json", ".toml": "toml", ".yaml": "yaml", ".yml": "yaml",
	".rules": "python", ".py": "python", ".sh": "bash", ".bash": "bash", ".zsh": "bash",
	".js": "javascript", ".mjs": "javascript", ".cjs": "javascript", ".ts": "typescript",
	".go": "go", ".rb": "ruby", ".rs": "rust", ".html": "html", ".css": "css", ".xml": "xml",
	".sql": "sql", ".lua": "lua",
}

// Load reads the content to preview for it.
func Load(it *discovery.Item) Content {
	switch {
	case it.Virtual:
		data, _ := json.MarshalIndent(it.Config, "", "  ")
		return Content{SourcePath: it.Path, Text: string(data), Format: FormatCode, Lang: "json", Size: -1,
			Note: "MCP server `" + it.Name + "` as configured in this file. Values that look like secrets are masked."}
	case it.IsDir:
		if it.Category == discovery.CategorySkill {
			skill := filepath.Join(it.Path, "SKILL.md")
			if _, err := os.Stat(skill); err == nil {
				return loadFile(skill)
			}
		}
		return Content{SourcePath: it.Path, Text: listing(it), Format: FormatMarkdown, Size: -1}
	default:
		return loadFile(it.Path)
	}
}

func loadFile(path string) Content {
	c := Content{SourcePath: path, Size: -1}
	f, err := os.Open(path)
	if err != nil {
		c.Note = "Cannot read file: " + err.Error()
		return c
	}
	defer f.Close()
	if fi, err := f.Stat(); err == nil {
		c.Size = fi.Size()
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if err != nil {
		c.Note = "Cannot read file: " + err.Error()
		return c
	}
	if len(data) > MaxBytes {
		data, c.Truncated = data[:MaxBytes], true
	}
	if bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0 {
		c.Binary = true
		c.Note = "Binary file — not shown."
		return c
	}
	c.Text = string(data)
	ext := strings.ToLower(filepath.Ext(path))
	switch {
	case ext == ".md" || ext == ".mdc" || ext == ".markdown" || filepath.Base(path) == ".cursorrules":
		c.Format = FormatMarkdown
	case codeLangs[ext] != "":
		c.Format, c.Lang = FormatCode, codeLangs[ext]
	}
	return c
}

func listing(it *discovery.Item) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", it.Name)
	var walk func(items []discovery.Item, indent string)
	walk = func(items []discovery.Item, indent string) {
		for _, c := range items {
			fmt.Fprintf(&b, "%s- `%s`\n", indent, c.Name)
			walk(c.Children, indent+"  ")
		}
	}
	walk(it.Children, "")
	if len(it.Children) == 0 {
		b.WriteString("_(empty)_\n")
	}
	return b.String()
}

// Renderer renders content for a given width, caching Glamour renderers.
type Renderer struct {
	style string
	mu    sync.Mutex
	byW   map[int]*glamour.TermRenderer
}

// NewRenderer creates a renderer using Glamour's dark or light style.
func NewRenderer(dark bool) *Renderer {
	style := "light"
	if dark {
		style = "dark"
	}
	return &Renderer{style: style, byW: map[int]*glamour.TermRenderer{}}
}

// NewRendererStyle creates a renderer with an explicit Glamour style name
// ("dark", "light", "notty", ...).
func NewRendererStyle(style string) *Renderer {
	return &Renderer{style: style, byW: map[int]*glamour.TermRenderer{}}
}

func (r *Renderer) glamour(width int) (*glamour.TermRenderer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if g, ok := r.byW[width]; ok {
		return g, nil
	}
	g, err := glamour.NewTermRenderer(glamour.WithStandardStyle(r.style), glamour.WithWordWrap(width))
	if err != nil {
		return nil, err
	}
	r.byW[width] = g
	return g, nil
}

// Render produces terminal output for c at the given width. raw shows the
// file text as-is (wrapped); otherwise Markdown is rendered and structured
// formats are syntax-highlighted.
func (r *Renderer) Render(c Content, width int, raw bool) string {
	width = max(width, 20)
	var out strings.Builder
	if c.Note != "" && !raw {
		out.WriteString(r.markdown("> "+c.Note, width))
	}
	if c.Binary || c.Text == "" {
		if c.Note == "" {
			out.WriteString(ansi.Wrap("(empty file)", width, ""))
		} else if raw {
			out.WriteString(ansi.Wrap(c.Note, width, ""))
		}
		return out.String()
	}
	if raw {
		out.WriteString(ansi.Wrap(expandTabs(c.Text), width, ""))
	} else {
		switch c.Format {
		case FormatMarkdown:
			out.WriteString(r.markdown(frontmatterAsCode(c.Text), width))
		case FormatCode:
			out.WriteString(r.markdown(fence(c.Text, c.Lang), width))
		default:
			out.WriteString(ansi.Wrap(expandTabs(c.Text), width, ""))
		}
	}
	if c.Truncated {
		out.WriteString(fmt.Sprintf("\n\n… truncated at %d KiB\n", MaxBytes/1024))
	}
	return out.String()
}

func (r *Renderer) markdown(md string, width int) string {
	g, err := r.glamour(width)
	if err == nil {
		if s, err := g.Render(md); err == nil {
			return s
		}
	}
	return ansi.Wrap(md, width, "") // fall back to plain text
}

func fence(text, lang string) string {
	f := "```"
	for strings.Contains(text, f) {
		f += "`"
	}
	return f + lang + "\n" + strings.TrimRight(text, "\n") + "\n" + f + "\n"
}

// frontmatterAsCode turns a leading YAML front matter block (common in
// SKILL.md and .mdc files) into a fenced YAML block so it renders as data
// instead of a horizontal rule followed by run-on text.
func frontmatterAsCode(md string) string {
	norm := strings.ReplaceAll(md, "\r\n", "\n")
	if !strings.HasPrefix(norm, "---\n") {
		return md
	}
	end := strings.Index(norm[4:], "\n---")
	if end < 0 {
		return md
	}
	fm := norm[4 : 4+end]
	rest := norm[4+end+4:]
	rest = strings.TrimPrefix(rest, "\n")
	return fence(fm, "yaml") + "\n" + rest
}

func expandTabs(s string) string { return strings.ReplaceAll(s, "\t", "    ") }
