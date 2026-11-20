package preview

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// RefKind says how a reference was written.
type RefKind int

const (
	RefLink   RefKind = iota // markdown [t](x), <autolink>, [id]: x or bare URL
	RefPath                  // path-like token in prose or inline code
	RefImport                // @path in instruction files
)

// RefStatus is what a reference resolves to.
type RefStatus int

const (
	RefOK       RefStatus = iota // regular file
	RefDir                       // directory
	RefMissing                   // does not exist
	RefOutside                   // exists but outside the allowed roots; not opened
	RefExternal                  // URL; never fetched
)

func (s RefStatus) String() string {
	switch s {
	case RefOK:
		return "ok"
	case RefDir:
		return "directory"
	case RefMissing:
		return "missing"
	case RefOutside:
		return "outside"
	case RefExternal:
		return "external"
	}
	return "unknown"
}

// Ref is one reference found in a document.
type Ref struct {
	Text   string // as written
	Target string // absolute, cleaned (URL for external)
	Kind   RefKind
	Status RefStatus
	Line   int // 1-based line in Content.Text where it first appears
}

// RefOptions configures References.
type RefOptions struct {
	Home    string                // expands "~/" targets
	Allowed func(abs string) bool // may an existing target be opened? nil = yes
}

var (
	reInlineLink = regexp.MustCompile(`!?\[[^\]\n]*\]\(\s*(<[^>\n]*>|[^)\s]*)(?:\s+(?:"[^"]*"|'[^']*'))?\s*\)`)
	reRefDef     = regexp.MustCompile(`^\s{0,3}\[[^\]\n]+\]:\s*(<[^>]*>|\S+)`)
	reAutolink   = regexp.MustCompile(`<((?:https?|ftp|mailto):[^>\s]+)>`)
	reBareURL    = regexp.MustCompile(`(?:https?|ftp)://[^\s)>\]` + "`" + `]+`)
	reScheme     = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9+.\-]*):`)
	reInlineCode = regexp.MustCompile("`[^`\n]*`")
	reImport     = regexp.MustCompile(`(?:^|[\s(\[])@((?:~/|/|\.{1,2}/)?[A-Za-z0-9_.\-/~]+)`)
	rePathTok    = regexp.MustCompile(`[A-Za-z0-9_.\-+%~/]+`)
	reExt        = regexp.MustCompile(`\.[A-Za-z0-9]{1,8}$`)
	reLetter     = regexp.MustCompile(`[A-Za-z]`)
	reInstr      = regexp.MustCompile(`(?i)^(claude|agents|gemini)(\.local)?\.md$`)
)

// References extracts local file references from Markdown and plain-text
// content, resolving relative targets against the directory of c.SourcePath.
// Results are de-duplicated by Target in order of first appearance. Nothing
// is opened beyond a stat, and URLs are never fetched.
func References(c Content, opt RefOptions) []Ref {
	if c.Binary || c.Text == "" || (c.Format != FormatMarkdown && c.Format != FormatText) {
		return nil
	}
	dir := filepath.Dir(c.SourcePath)
	instr := reInstr.MatchString(filepath.Base(c.SourcePath))
	var out []Ref
	seen := map[string]bool{}
	add := func(text, raw string, kind RefKind, line int, mustExist bool) {
		target, status, ok := resolve(raw, dir, opt, kind == RefLink)
		if !ok {
			return
		}
		if mustExist && (status == RefMissing || status == RefExternal) {
			return
		}
		if seen[target] {
			return
		}
		seen[target] = true
		out = append(out, Ref{Text: text, Target: target, Kind: kind, Status: status, Line: line})
	}

	inFence, marker := false, ""
	for i, line := range strings.Split(strings.ReplaceAll(c.Text, "\r\n", "\n"), "\n") {
		n := i + 1
		t := strings.TrimLeft(line, " \t")
		if m := fenceMarker(t); m != "" {
			if !inFence {
				inFence, marker = true, m
			} else if m[0] == marker[0] {
				inFence = false
			}
			continue
		}
		if inFence {
			for _, tok := range pathTokens(line) {
				add(tok, tok, RefPath, n, true)
			}
			continue
		}

		// Links first; blank them out so their targets are not re-read.
		var spans [][2]int
		if c.Format == FormatMarkdown {
			if m := reRefDef.FindStringSubmatchIndex(line); m != nil {
				add(line[m[2]:m[3]], line[m[2]:m[3]], RefLink, n, false)
				spans = append(spans, [2]int{m[0], m[1]})
			}
			for _, m := range reInlineLink.FindAllStringSubmatchIndex(line, -1) {
				if raw := line[m[2]:m[3]]; raw != "" {
					add(raw, raw, RefLink, n, false)
				}
				spans = append(spans, [2]int{m[0], m[1]})
			}
			for _, m := range reAutolink.FindAllStringSubmatchIndex(line, -1) {
				add(line[m[2]:m[3]], line[m[2]:m[3]], RefLink, n, false)
				spans = append(spans, [2]int{m[0], m[1]})
			}
		}
		rest := blank(line, spans)
		spans = spans[:0]
		for _, m := range reBareURL.FindAllStringIndex(rest, -1) {
			u := strings.TrimRight(rest[m[0]:m[1]], ".,;:!?")
			add(u, u, RefLink, n, false)
			spans = append(spans, [2]int{m[0], m[1]})
		}
		rest = blank(rest, spans)

		// @imports live outside inline code.
		noCode := reInlineCode.ReplaceAllStringFunc(rest, func(s string) string { return strings.Repeat(" ", len(s)) })
		for _, m := range reImport.FindAllStringSubmatch(noCode, -1) {
			tok := strings.TrimRight(m[1], ".,;:)]")
			if tok == "" {
				continue
			}
			add("@"+tok, tok, RefImport, n, !instr || !(strings.Contains(tok, "/") || reExt.MatchString(tok)))
		}

		for _, tok := range pathTokens(rest) {
			if !pathLike(tok) {
				add(tok, tok, RefPath, n, true) // bare name: only if it exists
				continue
			}
			add(tok, tok, RefPath, n, false)
		}
	}
	return out
}

func fenceMarker(t string) string {
	for _, m := range []string{"```", "~~~"} {
		if strings.HasPrefix(t, m) {
			return m
		}
	}
	return ""
}

func blank(s string, spans [][2]int) string {
	if len(spans) == 0 {
		return s
	}
	b := []byte(s)
	for _, sp := range spans {
		for i := sp[0]; i < sp[1]; i++ {
			b[i] = ' '
		}
	}
	return string(b)
}

// pathTokens returns candidate file-name tokens (with an extension) in s.
func pathTokens(s string) []string {
	var out []string
	for _, m := range rePathTok.FindAllStringIndex(s, -1) {
		tok := strings.TrimRight(s[m[0]:m[1]], ".,;:-")
		tok = strings.TrimLeft(tok, "-")
		if m[0] > 0 && s[m[0]-1] == '@' {
			continue // an import, handled separately
		}
		if tok == "" || tok == "." || tok == ".." || strings.Contains(tok, "//") || !reExt.MatchString(tok) {
			continue
		}
		ext := tok[strings.LastIndexByte(tok, '.')+1:]
		if !reLetter.MatchString(ext) || strings.HasSuffix(tok, "/") {
			continue
		}
		out = append(out, tok)
	}
	return out
}

// pathLike reports whether tok clearly looks like a path (so a missing
// target is worth showing) rather than a bare file name.
func pathLike(tok string) bool {
	return strings.Contains(tok, "/")
}

// resolve turns raw into an absolute target and status. ok=false means the
// reference should be ignored.
func resolve(raw, dir string, opt RefOptions, link bool) (target string, st RefStatus, ok bool) {
	raw = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(raw), "<"), ">")
	if m := reScheme.FindStringSubmatch(raw); m != nil && len(m[1]) > 1 {
		switch strings.ToLower(m[1]) {
		case "http", "https", "mailto", "ftp":
			return raw, RefExternal, true
		}
		return "", 0, false
	}
	if link {
		if i := strings.IndexByte(raw, '#'); i >= 0 {
			raw = raw[:i]
		}
		if i := strings.IndexByte(raw, '?'); i >= 0 {
			raw = raw[:i]
		}
		if u, err := url.PathUnescape(raw); err == nil {
			raw = u
		}
	}
	if raw == "" {
		return "", 0, false
	}
	switch {
	case raw == "~" || strings.HasPrefix(raw, "~/"):
		if opt.Home == "" {
			return "", 0, false
		}
		raw = filepath.Join(opt.Home, raw[1:])
	case strings.HasPrefix(raw, "~"):
		return "", 0, false
	case !filepath.IsAbs(raw):
		raw = filepath.Join(dir, raw)
	}
	target = filepath.Clean(raw)
	fi, err := os.Stat(target)
	switch {
	case err != nil:
		return target, RefMissing, true
	case opt.Allowed != nil && !opt.Allowed(target):
		return target, RefOutside, true
	case fi.IsDir():
		return target, RefDir, true
	}
	return target, RefOK, true
}
