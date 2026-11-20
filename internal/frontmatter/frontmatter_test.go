package frontmatter

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSplit(t *testing.T) {
	tests := []struct {
		name, in, fm, body string
		ok                 bool
	}{
		{"none", "# Title\ntext", "", "# Title\ntext", false},
		{"basic", "---\na: 1\n---\nbody\n", "a: 1", "body\n", true},
		{"crlf", "---\r\na: 1\r\n---\r\nbody\r\n", "a: 1", "body\n", true},
		{"empty block", "---\n---\nbody", "", "body", true},
		{"no body", "---\na: 1\n---", "a: 1", "", true},
		{"unterminated", "---\na: 1\nbody", "", "---\na: 1\nbody", false},
		{"dashes not at start", "x\n---\na: 1\n---\n", "", "x\n---\na: 1\n---\n", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fm, body, ok := Split(tc.in)
			if fm != tc.fm || body != tc.body || ok != tc.ok {
				t.Errorf("got (%q, %q, %v), want (%q, %q, %v)", fm, body, ok, tc.fm, tc.body, tc.ok)
			}
		})
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name, in string
		want     []Field
	}{
		{"empty", "", nil},
		{"simple", "name: foo\ndescription: Does things", []Field{{"name", "foo"}, {"description", "Does things"}}},
		{"quoted", "a: \"x y\"\nb: 'z'", []Field{{"a", "x y"}, {"b", "z"}}},
		{"colon in value", "description: Use when: asked about x", []Field{{"description", "Use when: asked about x"}}},
		{"block literal", "d: |\n  one\n  two\nn: x", []Field{{"d", "one two"}, {"n", "x"}}},
		{"block folded strip", "d: >-\n  one\n  two", []Field{{"d", "one two"}}},
		{"list", "tools:\n  - Read\n  - Bash\nn: x", []Field{{"tools", "Read, Bash"}, {"n", "x"}}},
		{"list unindented", "paths:\n- a/**\n- \"b/*\"", []Field{{"paths", "a/**, b/*"}}},
		{"inline list", "tools: [Read, \"Bash\", Grep]", []Field{{"tools", "Read, Bash, Grep"}}},
		{"continuation", "d: first\n  second", []Field{{"d", "first second"}}},
		{"crlf", "a: 1\r\nb: 2", []Field{{"a", "1"}, {"b", "2"}}},
		{"blank lines and junk", "\nnokey\na: 1", []Field{{"a", "1"}}},
		{"empty value", "a:\nb: 1", []Field{{"a", ""}, {"b", "1"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Parse(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestGetMap(t *testing.T) {
	f := []Field{{"Name", "A"}, {"name", "B"}, {"alwaysApply", "true"}}
	if Get(f, "name") != "B" {
		t.Error("exact match should win")
	}
	if Get(f, "alwaysapply") != "true" {
		t.Error("case-insensitive fallback")
	}
	if Get(f, "none") != "" {
		t.Error("missing key")
	}
	m := Map(f)
	if m["Name"] != "A" || m["name"] != "B" || len(m) != 3 {
		t.Errorf("map = %v", m)
	}
}

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.md")
	os.WriteFile(p, []byte("---\nname: x\n---\nhello"), 0o644)
	f, body := ReadFile(p)
	if Get(f, "name") != "x" || body != "hello" {
		t.Errorf("got %v %q", f, body)
	}
	os.WriteFile(p, []byte("no fm"), 0o644)
	if f, body := ReadFile(p); f != nil || body != "no fm" {
		t.Errorf("got %v %q", f, body)
	}
	os.WriteFile(p, []byte("a\x00b"), 0o644)
	if f, body := ReadFile(p); f != nil || body != "" {
		t.Errorf("binary: got %v %q", f, body)
	}
	if f, body := ReadFile(filepath.Join(dir, "missing")); f != nil || body != "" {
		t.Error("missing file")
	}
}
