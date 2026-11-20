package preview

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// FindLines returns the indices of lines in rendered (ANSI) output whose
// visible text contains query (case-insensitive). Empty query gives nil.
func FindLines(rendered, query string) []int {
	if query == "" {
		return nil
	}
	q := strings.ToLower(query)
	var out []int
	for i, l := range strings.Split(rendered, "\n") {
		if strings.Contains(strings.ToLower(ansi.Strip(l)), q) {
			out = append(out, i)
		}
	}
	return out
}

// HighlightLine re-renders line i as plain text in style; other lines are
// untouched.
func HighlightLine(rendered string, i int, style lipgloss.Style) string {
	lines := strings.Split(rendered, "\n")
	if i < 0 || i >= len(lines) {
		return rendered
	}
	lines[i] = style.Render(ansi.Strip(lines[i]))
	return strings.Join(lines, "\n")
}

// LineOf returns the index of the first rendered line whose visible text
// contains s, or -1.
func LineOf(rendered, s string) int {
	if s == "" {
		return -1
	}
	for i, l := range strings.Split(rendered, "\n") {
		if strings.Contains(ansi.Strip(l), s) {
			return i
		}
	}
	return -1
}

// Breadcrumb joins parts with " › ", dropping/truncating from the left with
// "…" so the last part (the current file) stays visible within width.
func Breadcrumb(parts []string, width int) string {
	const sep = " › "
	full := strings.Join(parts, sep)
	if ansi.StringWidth(full) <= width || len(parts) == 0 {
		return full
	}
	for i := 1; i < len(parts); i++ {
		s := "…" + sep + strings.Join(parts[i:], sep)
		if ansi.StringWidth(s) <= width {
			return s
		}
	}
	last := parts[len(parts)-1]
	if width <= 1 {
		return "…"
	}
	w := ansi.StringWidth(last)
	return ansi.TruncateLeft(last, w-(width-1), "…")
}
