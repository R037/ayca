# AYCA — All Your Context Are Belong To Us

*Know what your agents know.*

AYCA is a local-first terminal UI that shows every piece of context that can
influence an AI coding agent in your project: instruction files, rules, skills
and MCP servers, across project, parent-directory and user-global locations,
for **Claude Code**, **Codex** and **Cursor**.

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
agents or MCP servers, and makes no network calls.

## Install

Requires Go 1.24.2 or later.

```sh
git clone <this repo> ayca && cd ayca
make install            # same as: go install ./cmd/ayca
```

This puts `ayca` in `$(go env GOPATH)/bin` (usually `~/go/bin`). Make sure
that directory is on your `PATH`. To build without installing, run `make build`;
the binary ends up at `./bin/ayca`.

## Usage

```sh
ayca                         # inspect the current directory
ayca --path ~/code/my-app    # inspect another project
ayca --agent claude,cursor   # only show context for some agents
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
| `w` | Show discovery warnings (unreadable or malformed files) | |
| `r` | Rescan | Rescan |
| `q` / `ctrl+c` | Quit | Quit |

### Reading the tree

- Items are grouped by **category** (Instructions, Rules, Skills, MCP Servers),
  then by **scope**:
  - **Project** (green): inside the project root. The project root is the
    nearest git root, or the current directory if there isn't one.
  - **Ancestor directories** (yellow): above the project root.
  - **Global** (blue): user-wide configuration.
- The tags after a name (`claude·codex`) are the agents known to read that
  location.
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

| Category | Project | Ancestors | Global |
| --- | --- | --- | --- |
| Instructions | `<dir>/CLAUDE.md`, `<dir>/.claude/CLAUDE.md`, `<dir>/CLAUDE.local.md`, nested `CLAUDE.md` | same files, up to `/` | `~/.claude/CLAUDE.md` |
| Rules | `<dir>/.claude/rules/**/*.md` | | `~/.claude/rules/**/*.md` |
| Skills | `<dir>/.claude/skills/*/SKILL.md` | | `~/.claude/skills/**/SKILL.md`, skills of installed plugins |
| MCP | `.mcp.json` (project scope); `~/.claude.json` → `projects[<root>].mcpServers` (local scope) | | `~/.claude.json` → `mcpServers` (user scope), plugin `.mcp.json` |

`$CLAUDE_CONFIG_DIR` is honoured. Plugins come from
`~/.claude/plugins/installed_plugins.json`. Plugins disabled in
`settings.json` are marked as disabled.

### Codex

| Category | Project | Ancestors | Global |
| --- | --- | --- | --- |
| Instructions | `<dir>/AGENTS.override.md` / `<dir>/AGENTS.md`, nested `AGENTS.md` | `AGENTS.md`, flagged as likely not loaded | `$CODEX_HOME/AGENTS.override.md` / `AGENTS.md` |
| Rules | | | `$CODEX_HOME/rules/*.rules` (execution policy) |
| Skills | `<dir>/.agents/skills`, `<dir>/.codex/skills` | | `~/.agents/skills`, `$CODEX_HOME/skills` |
| MCP | `.codex/config.toml` `[mcp_servers.*]` (trusted projects) | | `$CODEX_HOME/config.toml` `[mcp_servers.*]` |

`$CODEX_HOME` defaults to `~/.codex`.

### Cursor

| Category | Project | Global |
| --- | --- | --- |
| Instructions | `AGENTS.md` (root and nested) | |
| Rules | `<dir>/.cursor/rules/**/*.{mdc,md}`, nested `.cursor/rules`, legacy `.cursorrules` | |
| Skills | `<dir>/.cursor/skills` | `~/.cursor/skills` |
| MCP | `.cursor/mcp.json` | `~/.cursor/mcp.json` |

Cursor's *User Rules* live inside the app's settings database, not in a file,
so AYCA can't see them.

### MCP servers and secrets

Each MCP server appears as its own entry, backed by the config file it came
from. The preview shows only that server's configuration, never the whole
file: `~/.claude.json`, for example, also holds account data. Values that look
like secrets are masked as `••••••••`. That covers every value in `env` and
`headers` maps, keys such as `token`, `secret`, `password`, `api_key` and
`auth`, arguments after flags like `--api-key`, URL passwords and credential
query parameters, and well-known token formats (`sk-…`, `ghp_…`, `xox…-`,
JWTs and similar). Environment variable references such as `${GITHUB_TOKEN}`
are left visible because they aren't secrets.

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
internal/discovery/  Env (paths), agent adapters, de-duplication, MCP parsing & masking
internal/tree/       Category → scope → item tree, expansion state, filtering
internal/preview/    Read-only file loading and Glamour rendering (rendered/raw)
internal/tui/        Bubble Tea model: layout, key handling, preview pane
```

To add an agent, write a `discoverX(c *collector)` function in
`internal/discovery/adapters.go` and register it in `Adapters`. Use
`c.file`, `c.ruleDir`, `c.skills`, `c.mcpJSONFile` and `c.mcpTOMLFile`. The
collector handles de-duplication, symlink resolution, scope fixes and
warnings.
