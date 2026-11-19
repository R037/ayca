// Package testfixture writes a realistic multi-agent context layout (a home
// directory, a workspace and a project) so tests across packages run against
// the same world. It is imported by tests and by the demo command only.
package testfixture

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/R037/ayca/internal/discovery"
)

// Fixture is a written layout. All paths are absolute.
type Fixture struct {
	Root       string // top directory
	Home       string // Root/home
	Workspace  string // Root/ws (ancestor of the project)
	Project    string // Root/ws/proj (git root)
	ManagedDir string // Root/etc/claude-code
}

// New builds the fixture in t.TempDir().
func New(t testing.TB) *Fixture {
	t.Helper()
	f, err := Write(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// Env returns a discovery.Env rooted at cwd with HOME = f.Home, an empty
// environment and ManagedDir = f.ManagedDir.
func (f *Fixture) Env(cwd string) discovery.Env {
	env := discovery.NewEnvWith(cwd, f.Home, func(string) string { return "" })
	env.ManagedDir = f.ManagedDir
	return env
}

// P joins rel onto the project directory.
func (f *Fixture) P(rel string) string { return filepath.Join(f.Project, rel) }

// H joins rel onto the home directory.
func (f *Fixture) H(rel string) string { return filepath.Join(f.Home, rel) }

// Write builds the fixture under dir.
func Write(dir string) (*Fixture, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	f := &Fixture{
		Root:       root,
		Home:       filepath.Join(root, "home"),
		Workspace:  filepath.Join(root, "ws"),
		Project:    filepath.Join(root, "ws", "proj"),
		ManagedDir: filepath.Join(root, "etc", "claude-code"),
	}
	w := &writer{}
	f.layout(w)
	if w.err != nil {
		return nil, w.err
	}
	return f, nil
}

type writer struct{ err error }

func (w *writer) file(path, content string, mode os.FileMode) {
	if w.err != nil {
		return
	}
	if w.err = os.MkdirAll(filepath.Dir(path), 0o755); w.err == nil {
		w.err = os.WriteFile(path, []byte(content), mode)
	}
}

func (w *writer) text(path, content string)   { w.file(path, content, 0o644) }
func (w *writer) script(path, content string) { w.file(path, content, 0o755) }

func (w *writer) json(path string, v any) {
	if w.err != nil {
		return
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		w.err = err
		return
	}
	w.text(path, string(b)+"\n")
}

func (w *writer) symlink(target, link string) {
	if w.err != nil {
		return
	}
	if w.err = os.MkdirAll(filepath.Dir(link), 0o755); w.err == nil {
		w.err = os.Symlink(target, link)
	}
}

func (f *Fixture) layout(w *writer) {
	h, p := f.H, f.P

	// --- managed policy
	w.text(filepath.Join(f.ManagedDir, "CLAUDE.md"), "# Company policy\n\n- Never commit secrets.\n- Follow the security review process for anything touching auth.\n")

	// --- Claude user level, with an import cycle
	w.text(h(".claude/CLAUDE.md"), "# Personal preferences\n\nAnswer concisely. Prefer small, reviewable changes.\n\nStyle guide: @~/.claude/shared/style.md\n")
	w.text(h(".claude/shared/style.md"), "# Style\n\nUse descriptive names. Avoid abbreviations.\n\nBack to the root file: @~/.claude/CLAUDE.md\n")
	w.text(h(".claude/rules/security.md"), "# Security\n\n- Validate all external input.\n- Never log credentials or tokens.\n- Use parameterised queries.\n")

	// --- Claude user skill with references, scripts, assets and a loop
	sk := h(".claude/skills/frontend-design")
	w.text(filepath.Join(sk, "SKILL.md"), "---\nname: frontend-design\ndescription: Build distinctive, production-grade frontend interfaces. Use when asked to design or restyle web pages and components.\nallowed-tools:\n  - Read\n  - Write\n  - Bash\n---\n\n# Frontend design\n\nPick a clear aesthetic direction before writing any code.\n\n- Read the [patterns](references/patterns.md) for layout and component rules.\n- Read the [a11y](references/accessibility.md) checklist before shipping.\n- Run `scripts/validate.sh` to lint the result.\n- Start from `assets/template.json` for theme tokens.\n- See the [old guide](references/missing.md) (removed).\n- Upstream docs: https://example.com/design-guide\n")
	w.text(filepath.Join(sk, "references/patterns.md"), "# Patterns\n\nBack to the [skill](../SKILL.md). Accessibility rules live in [a11y](accessibility.md).\n\n- Use a 4px spacing scale.\n- Keep components small.\n")
	w.text(filepath.Join(sk, "references/accessibility.md"), "# Accessibility\n\n- Contrast ratio of at least 4.5:1.\n- Every control is keyboard reachable.\n")
	w.script(filepath.Join(sk, "scripts/validate.sh"), "#!/usr/bin/env bash\nset -euo pipefail\necho \"validating $1\"\n")
	w.script(filepath.Join(sk, "scripts/check"), "#!/usr/bin/env python3\nimport sys\nprint(\"checking\", sys.argv[1:])\n")
	w.text(filepath.Join(sk, "assets/template.json"), "{\n  \"colors\": {\"primary\": \"#0b5fff\"},\n  \"spacing\": 4\n}\n")
	w.symlink("..", filepath.Join(sk, "assets/loop"))

	// --- shared skill (real dir under ~/.agents) symlinked into ~/.claude
	pdf := h(".agents/skills/pdf")
	w.text(filepath.Join(pdf, "SKILL.md"), "---\nname: pdf\ndescription: Extract text and tables from PDF files. Use when the user mentions a .pdf.\n---\n\n# PDF\n\nRun `scripts/extract.py <file>` and read [reference.md](reference.md) for options.\n")
	w.script(filepath.Join(pdf, "scripts/extract.py"), "#!/usr/bin/env python3\nimport sys\nprint(open(sys.argv[1], 'rb').read()[:100])\n")
	w.text(filepath.Join(pdf, "reference.md"), "# PDF reference\n\nOptions: --pages, --layout.\n")
	w.symlink("../../.agents/skills/pdf", h(".claude/skills/pdf"))

	// --- genuinely separate Cursor install with the same name
	w.text(h(".cursor/skills/pdf/SKILL.md"), "---\nname: pdf\ndescription: Cursor flavour of the PDF skill. Summarise PDF documents.\n---\n\n# PDF (Cursor)\n\nSummarise the document in five bullets.\n")

	// --- subagent and command
	w.text(h(".claude/agents/reviewer.md"), "---\nname: reviewer\ndescription: Reviews diffs for correctness and style. Use after code changes.\ntools: Read, Grep, Glob\n---\n\nYou are a careful code reviewer. Report only real problems.\n")
	w.text(h(".claude/commands/ship.md"), "---\ndescription: Run tests, then commit and push\n---\n\nRun the test suite. If it passes, commit with a clear message and push.\n")

	// --- settings + hook, plugin
	w.json(h(".claude/settings.json"), map[string]any{
		"hooks": map[string]any{
			"SessionStart": []any{map[string]any{"hooks": []any{
				map[string]any{"type": "command", "command": "~/.claude/hooks/session-start.sh"},
			}}},
		},
		"enabledPlugins": map[string]any{"demo@market": false},
	})
	w.script(h(".claude/hooks/session-start.sh"), "#!/usr/bin/env bash\necho \"Today is $(date +%F)\"\n")

	plug := h(".claude/plugins/cache/market/demo/1.0.0")
	w.json(h(".claude/plugins/installed_plugins.json"), map[string]any{
		"version": 2,
		"plugins": map[string]any{
			"demo@market": []any{map[string]any{"scope": "user", "installPath": plug}},
		},
	})
	w.text(filepath.Join(plug, "skills/demo-skill/SKILL.md"), "---\nname: demo-skill\ndescription: Demonstrates a plugin-provided skill.\n---\n\n# Demo skill\n\nSay hello.\n")
	w.json(filepath.Join(plug, "hooks/hooks.json"), map[string]any{
		"hooks": map[string]any{
			"PostToolUse": []any{map[string]any{"matcher": "Write|Edit", "hooks": []any{
				map[string]any{"type": "command", "command": "${CLAUDE_PLUGIN_ROOT}/hooks/fmt.sh"},
			}}},
		},
	})
	w.script(filepath.Join(plug, "hooks/fmt.sh"), "#!/usr/bin/env bash\ngofmt -l .\n")

	// --- ~/.claude.json: user and local MCP servers
	w.json(h(".claude.json"), map[string]any{
		"mcpServers": map[string]any{
			"linear": map[string]any{
				"type": "http", "url": "https://mcp.linear.app/mcp",
				"headers": map[string]any{"Authorization": "Bearer sk-ant-api03-FAKEFAKEFAKE"},
			},
		},
		"projects": map[string]any{
			f.Project: map[string]any{"mcpServers": map[string]any{
				"db": map[string]any{
					"command": "npx", "args": []any{"-y", "@modelcontextprotocol/server-postgres"},
					"env": map[string]any{"DB_PASSWORD": "hunter2"},
				},
			}},
		},
	})

	// --- Codex
	w.text(h(".codex/AGENTS.md"), "# Codex personal instructions\n\nExplain commands before running them.\n")
	w.text(h(".codex/config.toml"), "model = \"gpt-5\"\n\n[mcp_servers.github]\ncommand = \"npx\"\nargs = [\"-y\", \"@modelcontextprotocol/server-github\"]\nenv = { GITHUB_TOKEN = \"ghp_FAKEFAKEFAKEFAKEFAKEFAKEFAKEFAKE1234\" }\n")
	w.text(h(".codex/prompts/review.md"), "---\ndescription: Review the current branch\n---\n\nReview the diff against main and list issues by severity.\n")
	w.text(h(".codex/rules/default.rules"), "prefix_rule(pattern=[\"git\", \"push\"], decision=\"prompt\")\n")

	// --- Cursor global
	w.json(h(".cursor/mcp.json"), map[string]any{"mcpServers": map[string]any{
		"search": map[string]any{
			"command": "npx", "args": []any{"-y", "search-mcp"},
			"env": map[string]any{"SEARCH_API_KEY": "sk-search-FAKEFAKEFAKE"},
		},
	}})

	// --- ancestor
	w.text(filepath.Join(f.Workspace, "CLAUDE.md"), "# Workspace conventions\n\nAll services here share one monorepo layout.\n")
	w.text(filepath.Join(f.Workspace, "AGENTS.md"), "# Workspace agents file\n\nShared guidance for every project in this workspace.\n")

	// --- project
	w.text(p(".git/HEAD"), "ref: refs/heads/main\n")
	w.text(p("CLAUDE.md"), "# Project\n\nA small web platform.\n\nArchitecture: @docs/architecture.md\nOld notes: @docs/missing.md\n")
	w.text(p("docs/architecture.md"), "# Architecture\n\nServices in `services/`, frontend in `web/`.\n")
	w.text(p("CLAUDE.local.md"), "# Local notes\n\nMy sandbox database runs on port 5433.\n")
	w.text(p("AGENTS.md"), "# Agents\n\nRun `make test` before finishing.\n")
	w.text(p("AGENTS.override.md"), "# Agents (override)\n\nTemporary: skip the lint step.\n")
	w.text(p(".claude/rules/style.md"), "# Code style\n\n- Keep functions short.\n- Prefer early returns.\n")
	w.text(p(".claude/rules/backend.md"), "---\npaths:\n  - \"services/**\"\n---\n\n# Backend\n\nHandlers return typed errors.\n")
	w.text(p(".claude/skills/deploy/SKILL.md"), "---\nname: deploy\ndescription: Deploy the project to staging. Use when asked to deploy or release.\n---\n\n# Deploy\n\nRun `scripts/deploy.sh staging`.\n")
	w.script(p(".claude/skills/deploy/scripts/deploy.sh"), "#!/usr/bin/env bash\nset -euo pipefail\necho \"deploying to $1\"\n")
	w.json(p(".claude/settings.json"), map[string]any{"hooks": map[string]any{
		"PreToolUse": []any{map[string]any{"matcher": "Bash", "hooks": []any{
			map[string]any{"type": "command", "command": ".claude/hooks/guard.sh"},
		}}},
	}})
	w.script(p(".claude/hooks/guard.sh"), "#!/usr/bin/env bash\n# Blocks dangerous commands.\nexit 0\n")
	w.json(p(".mcp.json"), map[string]any{"mcpServers": map[string]any{
		"filesystem": map[string]any{
			"command": "npx", "args": []any{"-y", "@modelcontextprotocol/server-filesystem", "."},
			"env": map[string]any{"API_KEY": "sk-live-FAKE"},
		},
	}})
	w.text(p(".cursor/rules/always.mdc"), "---\ndescription: Project-wide conventions\nalwaysApply: true\n---\n\nUse TypeScript strict mode.\n")
	w.text(p(".cursor/rules/typescript.mdc"), "---\ndescription: TypeScript conventions\nglobs: \"**/*.ts\"\nalwaysApply: false\n---\n\nPrefer `unknown` over `any`.\n")
	w.text(p(".cursor/rules/agent-picks.mdc"), "---\ndescription: Database migration conventions; apply when writing migrations\n---\n\nMigrations must be reversible.\n")
	w.text(p(".agents/skills/lint/SKILL.md"), "---\nname: lint\ndescription: Run the linters and fix findings.\n---\n\n# Lint\n\nRun `make lint` and fix what it reports.\n")

	// --- nested
	w.text(p("services/api/CLAUDE.md"), "# API service\n\nHandlers live in `internal/http`.\n")
	w.text(p("services/api/AGENTS.md"), "# API service (agents)\n\nRun `go test ./...` in this directory.\n")
	w.text(p("services/payments/CLAUDE.md"), "# Payments service\n\nNever store card numbers.\n")
	w.text(p("web/CLAUDE.md"), "# Web frontend\n\nComponents are function components.\n")
	w.text(p("web/.cursor/rules/react.mdc"), "---\ndescription: React conventions\nglobs: \"**/*.tsx\"\n---\n\nNo class components.\n")
}
