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

	"github.com/R037/ayca/internal/discovery"
	"github.com/R037/ayca/internal/frontmatter"
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
	// Frontmatter and Body are set for Markdown files: the parsed leading
	// front matter fields and the text after it (Text itself when there is
	// none).
	Frontmatter []frontmatter.Field
	Body        string
	Lines       int // number of lines in Text
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
	".ps1": "powershell", ".tsx": "tsx", ".jsx": "jsx", ".java": "java", ".kt": "kotlin",
	".swift": "swift", ".php": "php", ".c": "c", ".h": "c", ".cpp": "cpp", ".cs": "csharp",
	".ini": "ini", ".cfg": "ini",
}

// shebangLangs maps an interpreter name to a highlighting language.
var shebangLangs = map[string]string{
	"python": "python", "python2": "python", "python3": "python",
	"bash": "bash", "sh": "bash", "zsh": "bash", "dash": "bash", "ksh": "bash",
	"node": "javascript", "nodejs": "javascript", "deno": "typescript", "bun": "typescript",
	"ruby": "ruby", "perl": "perl",
}

// shebangLang returns the language named by a "#!" first line, or "".
func shebangLang(text string) string {
	first, _, _ := strings.Cut(text, "\n")
	rest, ok := strings.CutPrefix(strings.TrimSpace(first), "#!")
	if !ok {
		return ""
	}
	fields := strings.Fields(rest)
	for len(fields) > 0 {
		base := filepath.Base(fields[0])
		if base == "env" {
			fields = fields[1:]
			for len(fields) > 0 && strings.HasPrefix(fields[0], "-") {
				fields = fields[1:]
			}
			continue
		}
		l := shebangLangs[base]
		if l == "" {
			// python3.11 and friends
			if i := strings.IndexByte(base, '.'); i > 0 {
				l = shebangLangs[base[:i]]
			}
		}
		return l
	}
	return ""
}

func isEnvFile(base string) bool {
	return base == ".env" || strings.HasPrefix(base, ".env.") || strings.HasSuffix(base, ".env")
}

// LoadPath loads any file for preview (used for followed references).
func LoadPath(path string) Content { return loadFile(path) }

// Load reads the content to preview for it.
func Load(it *discovery.Item) Content {
	switch {
	case it.Info != "":
		return Content{Text: it.Info, Format: FormatMarkdown, Size: -1}
	case it.Missing:
		return Content{SourcePath: it.Path, Size: -1,
			Note: "This file is referenced with `" + it.Name + "` but does not exist, so nothing is imported."}
	case it.Virtual:
		data, _ := json.MarshalIndent(it.Config, "", "  ")
		note := "MCP server `" + it.Name + "` as configured in this file. Values that look like secrets are masked."
		if it.Category == discovery.CategoryHook {
			note = "Hook as configured in this file. AYCA never runs hooks; scripts it references are listed below it in the tree. Values that look like secrets are masked."
		}
		return Content{SourcePath: it.Path, Text: string(data), Format: FormatCode, Lang: "json", Size: -1, Note: note}
	case it.IsDir:
		// Folders whose meaning lives in one entry file preview that file.
		entry := map[discovery.Category]string{discovery.CategorySkill: "SKILL.md", discovery.CategoryMemory: "MEMORY.md"}[it.Category]
		if entry != "" {
			p := filepath.Join(it.Path, entry)
			if _, err := os.Stat(p); err == nil {
				return loadFile(p)
			}
		}
		return Content{SourcePath: it.Path, Text: listing(it), Format: FormatMarkdown, Size: -1}
	default:
		return loadFile(it.Path)
	}
}

func loadFile(path string) Content {
	c := Content{SourcePath: path, Size: -1}
	if isEnvFile(strings.ToLower(filepath.Base(path))) {
		if fi, err := os.Stat(path); err == nil {
			c.Size = fi.Size()
		}
		c.Note = "Environment file — values not shown."
		return c
	}
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
	case ext == ".md" || ext == ".mdc" || ext == ".markdown" || ext == ".mdx" || filepath.Base(path) == ".cursorrules":
		c.Format = FormatMarkdown
	case codeLangs[ext] != "":
		c.Format, c.Lang = FormatCode, codeLangs[ext]
	case ext == "":
		if l := shebangLang(c.Text); l != "" {
			c.Format, c.Lang = FormatCode, l
		}
	}
	c.Body = c.Text
	if c.Format == FormatMarkdown {
		if fm, body, ok := frontmatter.Split(c.Text); ok {
			c.Frontmatter, c.Body = frontmatter.Parse(fm), body
		}
	}
	c.Lines = countLines(c.Text)
	return c
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
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
			out.WriteString(r.markdownDoc(c.Text, width))
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

// markdownDoc renders Markdown. A leading front matter block becomes a
// compact key/value table, a separator, then the rendered body. Malformed
// front matter renders as part of the text.
func (r *Renderer) markdownDoc(text string, width int) string {
	fm, body, ok := frontmatter.Split(text)
	fields := frontmatter.Parse(fm)
	if !ok || len(fields) == 0 {
		return r.markdown(text, width)
	}
	var b strings.Builder
	b.WriteString("**Frontmatter**\n\n")
	for _, f := range fields {
		fmt.Fprintf(&b, "- **%s:** %s\n", cell(f.Key), cell(f.Value))
	}
	b.WriteString("\n---\n\n")
	b.WriteString(body)
	return r.markdown(b.String(), width)
}

func cell(s string) string { return strings.ReplaceAll(s, "\n", " ") }

func expandTabs(s string) string { return strings.ReplaceAll(s, "\t", "    ") }
