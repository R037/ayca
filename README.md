# AYCA — All Your Context Are Belong To Us

*Know what your agents know.*

AYCA is a local-first terminal UI that shows every piece of context that can
influence an AI coding agent in your project: instruction files and their
`@imports`, auto memory, rules, skills, subagents, commands, output styles,
hooks and MCP servers. It covers project, parent-directory, user-global and
system-wide (managed policy) locations for **Claude Code**, **Codex** and
**Cursor**. It also lists the context that exists at runtime but has no file
to read.

```
AYCA — All Your Context Are Belong To Us  Know what your agents know.
Project: ~/projects/my-app  ·  12 items
╭────────────────────────────────────╮╭──────────────────────────────────────────╮
│▼ Instructions (3)                  ││Path     CLAUDE.md                        │
│  ▼ Project (3)                     ││File     252 B · ~63 tokens (est.)        │
│    ├─ • CLAUDE.md claude           ││Source   Instructions · project · claude  │
│    ├─ • AGENTS.md codex·cursor     ││──────────────────────────────────────────│
│    └─ • web/CLAUDE.md claude       ││  Project Instructions                    │
│▼ Rules (1)                         ││                                          │
│  ▼ Project (1)                     ││  ## Architecture                         │
│    └─ ▸ .cursor/rules/ cursor      ││  This project uses a Go backend…         │
│▼ Skills (2)                        ││                                          │
│  ▼ Global (2)                      ││                                          │
│    ├─ ▸ pdf claude·codex ⇆2        ││                                          │
│    └─ ▸ frontend-design claude     ││                                          │
│▼ MCP Servers (2)                   ││                                          │
│  ▼ Project (2)                     ││                                          │
│    ├─ • filesystem claude          ││                                          │
│    └─ • github claude              ││                                          │
╰────────────────────────────────────╯╰──────────────────────────────────────────╯
↑↓/jk navigate  ←→/hl collapse/expand  enter view  tab switch pane  / search  q quit
```

AYCA is **read-only**. It never modifies, creates or deletes files, never runs
agents, hooks or MCP servers, and makes no network calls.

## Install

Requires Go 1.24.2 or later.

```sh
go install github.com/R037/ayca/cmd/ayca@latest
```

Or from a clone, which also stamps the version:

```sh
git clone https://github.com/R037/ayca && cd ayca
make install            # installs to ~/.local/bin/ayca
```

Make sure `~/.local/bin` is on your `PATH`. To install somewhere else, use
`make install PREFIX=/usr/local`. To build without installing, run
`make build`; the binary ends up at `./bin/ayca`.

> If you use a version manager such as mise or asdf, prefer `make install`.
> `go install` puts the binary in the toolchain's own directory, and the
> shim then only works in projects that use that Go version.

## Usage

```sh
ayca                         # inspect the current directory
ayca --path ~/code/my-app    # inspect another project
ayca --agent claude,cursor   # only show context for some agents
ayca --window 1m             # compare against a 1M-token context window
ayca --list                  # print the discovered context as plain text (no TUI)
ayca --version
```

### Keyboard shortcuts

| Key | Context tree | Preview pane |
| --- | --- | --- |
| `↑` `↓` / `k` `j` | Move | Scroll |
| `←` / `h` | Collapse, or jump to parent | Back to tree |
| `→` / `l` | Expand, or step into the first child | |
| `space` | Toggle expand/collapse | |
| `enter` | Focus preview (on a file); toggle (on a group) | |
| `pgup` `pgdn` / `ctrl+u` `ctrl+d` | Page | Page |
| `g` `G` / `home` `end` | First / last | Top / bottom |
| `tab` | Switch between tree and preview | Switch |
| `/` | Search by name or path (`enter` keeps the filter, `esc` clears it) | |
| `esc` | Clear the active filter | Back to tree |
| `v` | Toggle rendered and raw view | Toggle |
| `c` | Show the startup context budget per agent | |
| `w` | Show discovery warnings (unreadable or malformed files) | |
| `r` | Rescan | Rescan |
| `q` / `ctrl+c` | Quit | Quit |

### Reading the tree

- Items are grouped by **category** (Instructions, Memory, Rules, Skills,
  Subagents, Commands, Output styles, Hooks, MCP Servers, Runtime), then by
  **scope**. Categories other than Instructions, Rules, Skills and MCP Servers
  appear only when something was found. The scopes are:
  - **Project** (green): inside the project root. The project root is the
    nearest git root, or the current directory if there isn't one.
  - **Ancestor directories** (yellow): above the project root.
  - **Global** (blue): user-wide configuration.
  - **System** (red): machine-wide managed policy set by an administrator,
    plus context built into the agents themselves.
- The tags after a name (`claude·codex`) are the agents known to read that
  location.
- Under a CLAUDE.md, `@path` entries are files it imports. They're nested
  the way Claude Code expands them. `✗ … missing` marks a broken import.
- Under a hook, the entries are the scripts its command runs. You can read
  them; AYCA never runs them.
- `◇` entries (Runtime) describe context with no file behind it. The preview
  explains what it is and where to check it.
- `⇆N` means the same file or folder was found at N other paths, for example
  a skill symlinked into `~/.claude/skills`, `~/.codex/skills` and
  `~/.agents/skills`. It is listed once. The preview shows every path and the
  real target.
- The preview header shows the absolute path, size, a rough token estimate
  (bytes ÷ 4), the category, scope and agents, plus notes such as
  "nested — loaded on demand" or "shadowed by AGENTS.override.md".

## Supported context locations

`<dir>` means every directory from the project root down to the current
directory. *Nested* means any subdirectory below the current directory. Those
files are loaded on demand when an agent works there. The nested scan goes up
to 6 levels deep, skips hidden, `node_modules`, `vendor`, `dist`, `build` and
`target` directories, and is skipped when you run AYCA from `$HOME` or `/`.

### Claude Code

| Category | Project | Ancestors | Global | System |
| --- | --- | --- | --- | --- |
| Instructions | `<dir>/CLAUDE.md`, `<dir>/.claude/CLAUDE.md`, `<dir>/CLAUDE.local.md`, nested `CLAUDE.md` | same files, up to `/` | `~/.claude/CLAUDE.md` | managed `CLAUDE.md` |
| `@imports` | expanded under every CLAUDE.md, up to 5 levels deep, ignoring code blocks | | | |
| Memory | `~/.claude/projects/<project>/memory/` (auto memory; `MEMORY.md` loads every session) | | | |
| Rules | `<dir>/.claude/rules/**/*.md` | | `~/.claude/rules/**/*.md` | |
| Skills | `<dir>/.claude/skills/*/SKILL.md` | | `~/.claude/skills/**/SKILL.md`, plugin skills | |
| Subagents | `<dir>/.claude/agents/` | | `~/.claude/agents/`, plugin `agents/` | |
| Commands | `<dir>/.claude/commands/` | | `~/.claude/commands/`, plugin `commands/` | |
| Output styles | `<dir>/.claude/output-styles/` | | `~/.claude/output-styles/`, plugin `output-styles/` | |
| Hooks | `.claude/settings.json`, `.claude/settings.local.json` | | `~/.claude/settings.json`, plugin `hooks/hooks.json` | `managed-settings.json` |
| MCP | `.mcp.json` (project scope); `~/.claude.json` → `projects[<root>].mcpServers` (local scope) | | `~/.claude.json` → `mcpServers` (user scope), plugin `.mcp.json` | `managed-mcp.json` |

The managed policy directory is `/etc/claude-code` on Linux,
`/Library/Application Support/ClaudeCode` on macOS and
`C:\Program Files\ClaudeCode` on Windows. `$CLAUDE_CONFIG_DIR` is honoured.
Plugins come from `~/.claude/plugins/installed_plugins.json`. Plugins disabled
in `settings.json` are marked as disabled. The active `outputStyle` is noted.

### Codex

| Category | Project | Ancestors | Global |
| --- | --- | --- | --- |
| Instructions | `<dir>/AGENTS.override.md` / `<dir>/AGENTS.md`, nested `AGENTS.md` | `AGENTS.md`, flagged as likely not loaded | `$CODEX_HOME/AGENTS.override.md` / `AGENTS.md` |
| Rules | | | `$CODEX_HOME/rules/*.rules` (execution policy) |
| Skills | `<dir>/.agents/skills`, `<dir>/.codex/skills` | | `~/.agents/skills`, `$CODEX_HOME/skills` |
| Commands | | | `$CODEX_HOME/prompts/*.md` (custom prompts) |
| Hooks | | | `$CODEX_HOME/hooks.json` |
| MCP | `.codex/config.toml` `[mcp_servers.*]` (trusted projects) | | `$CODEX_HOME/config.toml` `[mcp_servers.*]` |

`$CODEX_HOME` defaults to `~/.codex`.

### Cursor

| Category | Project | Global |
| --- | --- | --- |
| Instructions | `AGENTS.md` (root and nested) | |
| Rules | `<dir>/.cursor/rules/**/*.{mdc,md}`, nested `.cursor/rules`, legacy `.cursorrules` | |
| Skills | `<dir>/.cursor/skills` | `~/.cursor/skills` |
| Commands | `.cursor/commands/*.md` | `~/.cursor/commands/*.md` |
| Hooks | `.cursor/hooks.json` | `~/.cursor/hooks.json` |
| MCP | `.cursor/mcp.json` | `~/.cursor/mcp.json` |

Cursor's *User Rules* live inside the app's settings database, not in a file,
so AYCA can't see them.

### Hooks

Each hook command is its own entry, labelled with its event and command.
SessionStart and UserPromptSubmit hooks are flagged because their output goes
straight into the agent's context. AYCA finds the scripts a command
references: absolute paths, `~`, `$HOME`, `${CLAUDE_PLUGIN_ROOT}`,
`$CLAUDE_PROJECT_DIR`, paths relative to the config file, and arguments that
name a file next to such a script. It lists them under the hook so you can
read what would run.

### Runtime context (not on disk)

Some context is present in every session but has no file behind it. AYCA
lists it under **Runtime** with an explanation instead of pretending it
doesn't exist:

- the built-in system prompts of Claude Code, Codex and Cursor
- Claude Code's session environment (working directory, platform, date,
  git status)
- claude.ai account connectors. AYCA lists connectors that `~/.claude.json`
  records as used on this machine, clearly marked as history rather than
  current state.
- launch flags (`--append-system-prompt`, `--mcp-config`, …), environment
  variables, and the output hooks produce when they run
- Cursor User Rules and Memories, which live in the app's settings

### MCP servers, hooks and secrets

Each MCP server and hook appears as its own entry, backed by the config file
it came from. The preview shows only that entry's configuration, never the
whole file: `~/.claude.json`, for example, also holds account data. Values that look
like secrets are masked as `••••••••`. That covers every value in `env` and
`headers` maps, keys such as `token`, `secret`, `password`, `api_key` and
`auth`, arguments after flags like `--api-key`, URL passwords and credential
query parameters, and well-known token formats (`sk-…`, `ghp_…`, `xox…-`,
JWTs and similar), including tokens embedded in a longer command line.
Environment variable references such as `${GITHUB_TOKEN}`
are left visible because they aren't secrets.

## Startup context budget

The header shows roughly how much of each agent's context window the context
on disk fills before you type anything, for example
`startup context on disk: claude 1.9% · codex 0.2%`. Press `c` for the
breakdown by category and item. `--list` prints the same at the end, and
each item's details include a *Startup* line.

What counts:

| Context | Counted as |
| --- | --- |
| Instruction files and their `@imports` | full text |
| Auto memory | the first 200 lines of `MEMORY.md` |
| Claude Code rules without `paths:`, Cursor rules with `alwaysApply: true`, `.cursorrules` | full text |
| Cursor rules with only a `description` | the description (the agent decides whether to read the rest) |
| Skills, Claude Code subagents, Claude Code commands with a `description` | name and description |
| The active Claude Code output style | its body |
| Codex project `AGENTS.md` files | full text, capped at 32 KiB in total (`project_doc_max_bytes`) |

What doesn't count: nested files (they load on demand), shadowed files, files
an agent likely ignores, disabled plugins, conditional rules, Codex `.rules`
(they're enforced by the CLI, not sent to the model), and Codex prompts and
Cursor commands (they load only when typed).

The panel also lists what AYCA knows is there but can't size: the built-in
system prompt and tool definitions, the tools of each MCP server, and the
output of SessionStart and UserPromptSubmit hooks. These are often the
largest part of the real total, so treat the percentage as a lower bound.
Tokens are estimated as bytes ÷ 4. The windows default to 200k for Claude
Code and Cursor and 272k for Codex, but the real size depends on the model;
set yours with `--window`. For exact numbers, run `/context` in Claude Code.

## What AYCA does *not* know

AYCA shows what is **configured on disk**. It can't see what an agent actually
received in a given session. Agents may skip files, load them lazily, or apply
precedence rules that change between versions. Notes like "likely not loaded"
or "nested — loaded on demand" describe documented behaviour; they don't come
from inspecting a running agent.

## Development

```sh
make test     # go test ./...
make lint     # gofmt check + go vet
make build    # ./bin/ayca
```

The layout keeps discovery, presentation and filesystem access separate, so
editing can be added in v0.2 without restructuring:

```
cmd/ayca/            CLI flags, --list output, program start-up
internal/discovery/  Env (paths), agent adapters (claude.go, codex.go, cursor.go),
                     hooks, @imports, runtime info, de-duplication, MCP parsing & masking
internal/budget/     Startup context estimate per agent (tokens, % of window)
internal/tree/       Category → scope → item tree, expansion state, filtering
internal/preview/    Read-only file loading and Glamour rendering (rendered/raw)
internal/tui/        Bubble Tea model: layout, key handling, preview pane
```

To add an agent, write a `discoverX(c *collector)` function in its own file
and register it in `Adapters` in `internal/discovery/adapters.go`. Use
`c.file`, `c.fileDir`, `c.skills`, `c.hooksFile`, `c.mcpJSONFile`,
`c.mcpTOMLFile` and `c.info`. The
collector handles de-duplication, symlink resolution, scope fixes and
warnings.

## License

MIT. See [LICENSE](LICENSE).
