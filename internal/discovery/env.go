package discovery

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Env describes the directories discovery runs against. Tests construct it
// directly so they never touch the real home directory.
type Env struct {
	Cwd         string // where ayca was started (or --path)
	ProjectRoot string // nearest git root at or above Cwd, else Cwd
	Home        string
	ClaudeDir   string // $CLAUDE_CONFIG_DIR or ~/.claude
	ClaudeJSON  string // ~/.claude.json (or inside $CLAUDE_CONFIG_DIR)
	CodexHome   string // $CODEX_HOME or ~/.codex
	CursorDir   string // ~/.cursor
	AgentsDir   string // ~/.agents (shared skills location used by Codex)
	ManagedDir  string // Claude Code managed policy directory (system-wide)
}

// managedDir is where Claude Code reads administrator-managed policy files.
func managedDir() string {
	switch runtime.GOOS {
	case "darwin":
		return "/Library/Application Support/ClaudeCode"
	case "windows":
		return `C:\Program Files\ClaudeCode`
	default:
		return "/etc/claude-code"
	}
}

// NewEnv builds an Env for the given start path using the real user home and
// environment variables.
func NewEnv(path string) (Env, error) {
	if path == "" {
		var err error
		if path, err = os.Getwd(); err != nil {
			return Env{}, err
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return Env{}, err
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return Env{}, err
	}
	if !fi.IsDir() {
		return Env{}, errors.New(abs + " is not a directory")
	}
	home, _ := os.UserHomeDir()
	return NewEnvWith(abs, home, os.Getenv), nil
}

// NewEnvWith builds an Env from explicit inputs (used by tests).
func NewEnvWith(cwd, home string, getenv func(string) string) Env {
	cwd = filepath.Clean(cwd)
	e := Env{
		Cwd:         cwd,
		ProjectRoot: findProjectRoot(cwd),
		Home:        home,
		ClaudeDir:   filepath.Join(home, ".claude"),
		ClaudeJSON:  filepath.Join(home, ".claude.json"),
		CodexHome:   filepath.Join(home, ".codex"),
		CursorDir:   filepath.Join(home, ".cursor"),
		AgentsDir:   filepath.Join(home, ".agents"),
		ManagedDir:  managedDir(),
	}
	if v := getenv("CLAUDE_CONFIG_DIR"); v != "" {
		e.ClaudeDir = v
		e.ClaudeJSON = filepath.Join(v, ".claude.json")
	}
	if v := getenv("CODEX_HOME"); v != "" {
		e.CodexHome = v
	}
	return e
}

// WithCwd returns a copy of e that starts at dir. The project root is kept
// when dir is the root or inside it, otherwise it is searched from dir.
func (e Env) WithCwd(dir string) Env {
	dir = filepath.Clean(dir)
	e.Cwd = dir
	if dir != e.ProjectRoot {
		if _, ok := under(e.ProjectRoot, dir); !ok {
			e.ProjectRoot = findProjectRoot(dir)
		}
	}
	return e
}

// findProjectRoot walks up from dir looking for a .git entry (directory or
// worktree file). Without one, dir itself is the project root.
func findProjectRoot(dir string) string {
	for d := dir; ; {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return dir
		}
		d = parent
	}
}

// ProjectDirs returns the directories from the project root down to Cwd
// (inclusive, root first).
func (e Env) ProjectDirs() []string {
	var dirs []string
	for d := e.Cwd; ; d = filepath.Dir(d) {
		dirs = append([]string{d}, dirs...)
		if d == e.ProjectRoot || filepath.Dir(d) == d {
			break
		}
	}
	return dirs
}

// AncestorDirs returns the directories above the project root, nearest
// first, up to the filesystem root.
func (e Env) AncestorDirs() []string {
	var dirs []string
	for d := filepath.Dir(e.ProjectRoot); ; d = filepath.Dir(d) {
		if d == e.ProjectRoot {
			break
		}
		dirs = append(dirs, d)
		if filepath.Dir(d) == d {
			break
		}
	}
	return dirs
}

// Tilde shortens a path under the home directory to ~/...
func (e Env) Tilde(p string) string {
	if e.Home == "" {
		return p
	}
	if p == e.Home {
		return "~"
	}
	if rel, ok := under(e.Home, p); ok {
		return "~/" + rel
	}
	return p
}

// DisplayPath renders a path relative to the project root when it is inside
// it, otherwise ~-shortened.
func (e Env) DisplayPath(p string) string {
	if rel, ok := under(e.ProjectRoot, p); ok {
		return rel
	}
	return e.Tilde(p)
}

// under reports whether p is strictly inside dir, returning the relative path.
func under(dir, p string) (string, bool) {
	rel, err := filepath.Rel(dir, p)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return rel, true
}

// scopeOf classifies a path that lives in a project or ancestor directory.
func (e Env) scopeOf(p string) Scope {
	if p == e.ProjectRoot {
		return ScopeProject
	}
	if _, ok := under(e.ProjectRoot, p); ok {
		return ScopeProject
	}
	return ScopeAncestor
}

// isGlobalPath reports whether p is inside one of the user-level agent
// configuration locations.
func (e Env) isGlobalPath(p string) bool {
	if p == e.ClaudeJSON {
		return false // holds both user- and project(local)-scoped entries
	}
	for _, d := range []string{e.ClaudeDir, e.CodexHome, e.CursorDir, e.AgentsDir} {
		if d == "" {
			continue
		}
		if _, ok := under(d, p); ok {
			return true
		}
	}
	return false
}
