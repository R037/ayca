// Package frontmatter reads the small subset of YAML front matter used by
// agent context files (skills, rules, commands, subagents).
package frontmatter

import (
	"bytes"
	"io"
	"os"
	"strings"
)

// Field is one top-level front matter entry.
type Field struct{ Key, Value string }

const maxRead = 1 << 20

// Split separates a leading YAML front matter block ("---\n...\n---") from
// the body. CRLF tolerant. ok=false when there is none.
func Split(text string) (fm, body string, ok bool) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return "", text, false
	}
	rest := text[4:]
	var end int
	if strings.HasPrefix(rest, "---") && (len(rest) == 3 || rest[3] == '\n') {
		end = 0 // empty block
	} else {
		end = strings.Index(rest, "\n---")
		if end < 0 {
			return "", text, false
		}
		end++ // keep the newline out of fm below
	}
	fm = strings.TrimSuffix(rest[:end], "\n")
	after := rest[end+3:]
	if i := strings.IndexByte(after, '\n'); i >= 0 {
		after = after[i+1:]
	} else {
		after = ""
	}
	return fm, after, true
}

// Parse reads top-level "key: value" pairs in order. Quotes are trimmed.
// Block scalars (| > |- >-) are joined into one string with single spaces.
// List items ("- x" lines, or inline [a, b]) are joined with ", ".
// Indented continuation lines are appended to the current key with a space.
func Parse(fm string) []Field {
	var out []Field
	cur := -1
	listy := false // current key's value is built from list items
	for _, line := range strings.Split(strings.ReplaceAll(fm, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		first := line[0]
		if first == ' ' || first == '\t' || first == '-' {
			if cur < 0 {
				continue
			}
			t := strings.TrimSpace(line)
			f := &out[cur]
			if item, isItem := strings.CutPrefix(t, "- "); isItem || t == "-" {
				item = unquote(strings.TrimSpace(item))
				if f.Value == "" || !listy {
					f.Value = join(f.Value, item, " ")
				} else {
					f.Value += ", " + item
				}
				listy = true
				continue
			}
			f.Value = join(f.Value, t, " ")
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			cur = -1
			continue
		}
		v = strings.TrimSpace(v)
		listy = false
		switch {
		case v == "|" || v == ">" || v == "|-" || v == ">-" || v == "|+" || v == ">+":
			v = ""
		case strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]"):
			var parts []string
			for _, p := range strings.Split(v[1:len(v)-1], ",") {
				if p = unquote(strings.TrimSpace(p)); p != "" {
					parts = append(parts, p)
				}
			}
			v = strings.Join(parts, ", ")
		default:
			v = unquote(v)
		}
		out = append(out, Field{Key: strings.TrimSpace(k), Value: v})
		cur = len(out) - 1
	}
	return out
}

func unquote(s string) string { return strings.Trim(s, `"'`) }

func join(a, b, sep string) string {
	if a == "" {
		return b
	}
	return a + sep + b
}

// Get returns the value of key: exact match first, then case-insensitive.
func Get(fields []Field, key string) string {
	for _, f := range fields {
		if f.Key == key {
			return f.Value
		}
	}
	for _, f := range fields {
		if strings.EqualFold(f.Key, key) {
			return f.Value
		}
	}
	return ""
}

// Map returns the fields as a map (the last occurrence of a key wins).
func Map(fields []Field) map[string]string {
	m := make(map[string]string, len(fields))
	for _, f := range fields {
		m[f.Key] = f.Value
	}
	return m
}

// ReadFile reads up to 1 MiB of path and returns its fields and body;
// binary or unreadable files give zero values.
func ReadFile(path string) (fields []Field, body string) {
	f, err := os.Open(path)
	if err != nil {
		return nil, ""
	}
	defer f.Close()
	b, _ := io.ReadAll(io.LimitReader(f, maxRead))
	if bytes.IndexByte(b[:min(len(b), 8000)], 0) >= 0 {
		return nil, ""
	}
	fm, body, ok := Split(string(b))
	if !ok {
		return nil, body
	}
	return Parse(fm), body
}
