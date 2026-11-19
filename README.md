# AYCA — All Your Context Are Belong To Us

*Know what your agents know.*

AYCA is a local-first terminal explorer for the context that AI coding agents
read: instruction files and their `@imports`, auto memory, rules, skills,
subagents, commands, output styles, hooks, MCP servers and the context that
exists only at runtime. It covers **Claude Code**, **Codex** and **Cursor**,
at project, parent-directory, user-global and system-wide (managed policy)
level.

It is built for learning how your agents work: **explore → read → understand
→ improve.** Open AYCA in a project and you can

- see how the selected agent's context is organised (global, project, nested,
  plugins, hooks, MCP, runtime),
- open a skill like a folder, read its `SKILL.md`, follow its links to
  reference docs and scripts, and come back to where you were,
- ask *what is this, where does it come from, how is it discovered and when
  does it load?* for any item,
- pick a working directory and see which context applies from there, and why,
- see the physical files behind it all, including symlinks shared between
  agents.

```
AYCA — All Your Context Are Belong To Us  Know what your agents know.
Agent: Claude Code (a)  ·  View: Hierarchy (1-4)  ·  Working dir: . (project root) (d)
╭──────────────────────────────────────╮╭──────────────────────────────────────────────────────────╮
│▼ Global context (~/.claude) (6)      ││Global context (~/.claude) › Skills › frontend-design     │
│  ▼ Instructions (1)                  ││[ ] links (6)                                             │
│    └─ ▸ ~/.claude/CLAUDE.md          ││Path     ~/.claude/skills/frontend-design/SKILL.md        │
│  ▼ Rules (1)                         ││File     644 B · ~161 tokens (est.) · rendered view       │
│    └─ ▸ ~/.claude/rules/             ││──────────────────────────────────────────────────────────│
│  ▼ Skills (2)                        ││                                                          │
│    ├─ ▸ frontend-design              ││  Frontmatter                                             │
│    └─ ▸ pdf ⇆1                       ││                                                          │
│  ▼ Subagents (1)                     ││  • name: frontend-design                                 │
│    └─ ▸ ~/.claude/agents/            ││  • description: Build distinctive, production-grade      │
│  ▼ Commands (1)                      ││  frontend interfaces. Use when asked to design or        │
│    └─ ▸ ~/.claude/commands/          ││  restyle web pages and components.                       │
│▼ Project context (4)                 ││  • allowed-tools: Read, Write, Bash                      │
│  ▼ Instructions (2)                  ││                                                          │
│    ├─ ▸ CLAUDE.md                    ││  --------                                                │
│    └─ • CLAUDE.local.md              ││                                                          │
│  ▼ Rules (1)                         ││   Frontend design                                        │
│    └─ ▸ .claude/rules/               ││                                                          │
│  ▼ Skills (1)                        ││  Pick a clear aesthetic direction before writing any     │
│    └─ ▸ deploy                       ││  code.                                                   │
│▼ Parent directories (1)              ││                                                          │
╰──────────────────────────────────────╯╰──────────────────────────────────────────────────────────╯
╭──────────────────────────────────────────────────────────────────────────────────────────────────╮
│What     Skill · global · A skill: a folder with a SKILL.md and optional supporting files that te…│
│From     ~/.claude/skills/frontend-design                                                         │
│Loads    Claude Code: Listed (name + description) — Name and description listed at startup; SKILL…│
│Related  5 supporting files: assets/template.json, references/accessibility.md, references/patter…│
╰──────────────────────────────────────────────────────────────────────────────────────────────────╯
↑↓ move  ←→ fold  enter read  1-4 view  a agent  d dir  e explain  / filter  ? help  q quit
```

AYCA is **read-only**. It never modifies, creates or deletes files, never runs
agents, hooks, scripts or MCP servers, and makes no network calls. Its
explanations come from built-in, per-agent rules, not from an LLM.

## Install

Requires Go 1.24.2 or later.

```sh
go install github.com/R037/ayca/cmd/ayca@latest
```

`go install` puts the binary in `$(go env GOPATH)/bin`, which is usually
`~/go/bin`. If running `ayca` then gives `command not found`, that directory
isn't on your `PATH` yet. Add it to your shell profile (`~/.bashrc`,
`~/.zshrc`, ...) and open a new terminal:

```sh
export PATH="$PATH:$(go env GOPATH)/bin"
```

If you set `GOBIN`, the binary goes there instead, so put that directory on
your `PATH`.

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
ayca --view effective        # start the TUI in another view (hierarchy, effective, filesystem, inventory)
ayca --list                  # print the discovered context as plain text (no TUI)
ayca --json                  # print it as JSON, for scripts and AI agents
ayca --version
```

## Exploring your context

### Four views of one inventory

AYCA discovers everything once and shows it four ways. Press `1`–`4` to
switch views. Each view remembers its own cursor and folds, so you can switch
back and forth without losing your place.

| Key | View | Shows |
| --- | --- | --- |
| `1` | **Hierarchy** (default) | One agent's context, organised the way that agent sees it: managed policy, global context, project context, parent directories, nested / conditional context, plugins, hooks, MCP servers, runtime context. |
| `2` | **Effective** | What applies to one agent started in one working directory, grouped by when it loads (see [Effective context](#effective-context)). |
| `3` | **Filesystem** | The real directories and files behind the context. A symlink appears as a link (`→ target`) at its own location. The physical folder is marked `⇆ N locations`. Config files such as `settings.json` or `.mcp.json` hold their hooks and servers. |
| `4` | **Inventory** | Everything for all agents, by category and scope. This was AYCA's original view. |

Press `a` to switch the agent (Claude Code → Codex → Cursor). Hierarchy and
Effective follow it, and explanations use it in every view. With `--agent`
only the agents you named are offered.

The same skill can be one folder exposed in several places, or separate
copies. For example, `~/.claude/skills/pdf` may be a symlink to the shared
`~/.agents/skills/pdf`, while `~/.cursor/skills/pdf` is a separate folder.
The Filesystem view shows the first as one physical folder with a link
pointing at it, and the second as a folder of its own.

### Reading a multi-file skill

Skills are folders. AYCA treats them as small documentation sites you can
browse:

1. In the Hierarchy view, move to **Global context → Skills →
   frontend-design** and press `→` to unfold it. You see its files:
   `SKILL.md`, `references/`, `scripts/`, `assets/`.
2. Move to `SKILL.md` and press `enter`. The preview pane takes focus and
   shows a breadcrumb, the frontmatter (name, description, allowed tools)
   separated from the instructions, and then the instructions themselves.
   Press `v` to see the raw file.
3. The status line says `[ ] links (6)`. Press `]` to step through the links
   in the document. Each one is marked:
   - `✓` an existing file
   - `✗ missing` a broken reference
   - `URL — AYCA never fetches` a web link
   - `outside the skill and config folders — not opened` a file AYCA won't open
4. With `references/patterns.md` selected, press `enter`. The tree jumps to
   that file and the preview shows it. Links to files that aren't in the tree
   open as a loose document.
5. Press `esc` (or `backspace`) to go back. You return to `SKILL.md` at the
   same scroll position. `esc` with nothing to go back to returns to the tree.
6. Open `scripts/validate.sh`. It renders as read-only, syntax-highlighted
   source. AYCA never runs it.
7. Press `/` in the preview to find text in a long document. `n` and `N`
   move between matches.

Reading `SKILL.md`, with the first link selected:

```
AYCA — All Your Context Are Belong To Us  Know what your agents know.
Agent: Claude Code (a)  ·  View: Hierarchy (1-4)  ·  Working dir: . (project root) (d)
╭──────────────────────────────────────╮╭──────────────────────────────────────────────────────────╮
│▼ Global context (~/.claude) (6)      ││… › Skills › frontend-design › SKILL.md                   │
│  ▼ Instructions (1)                  ││Link 1/6 → references/patterns.md ✓ (enter opens)         │
│    └─ ▸ ~/.claude/CLAUDE.md          ││Path     ~/.claude/skills/frontend-design/SKILL.md        │
│  ▼ Rules (1)                         ││File     644 B · ~161 tokens (est.) · rendered view       │
│    └─ ▸ ~/.claude/rules/             ││──────────────────────────────────────────────────────────│
│  ▼ Skills (2)                        ││                                                          │
│    ├─ ▾ frontend-design              ││   Frontend design                                        │
│    │  ├─ ▸ assets/                   ││                                                          │
│    │  ├─ ▸ references/               ││  Pick a clear aesthetic direction before writing any     │
│    │  ├─ ▸ scripts/                  ││  code.                                                   │
│    │  └─ • SKILL.md                  ││                                                          │
│    └─ ▸ pdf ⇆1                       ││  • Read the patterns /references/patterns.md for         │
│  ▼ Subagents (1)                     ││  layout and component rules.                             │
│    └─ ▸ ~/.claude/agents/            ││  • Read the a11y /references/accessibility.md            │
│  ▼ Commands (1)                      ││  checklist before shipping.                              │
│    └─ ▸ ~/.claude/commands/          ││  • Run  scripts/validate.sh  to lint the result.         │
│▼ Project context (4)                 ││  • Start from  assets/template.json  for theme tokens.   │
│  ▼ Instructions (2)                  ││  • See the old guide /references/missing.md (removed).   │
│    ├─ ▸ CLAUDE.md                    ││  • Upstream docs: https://example.com/design-guide       │
│    └─ • CLAUDE.local.md              ││                                                          │
│  ▼ Rules (1)                         ││                                                          │
╰──────────────────────────────────────╯╰──────────────────────────────────────────────────────────╯
╭──────────────────────────────────────────────────────────────────────────────────────────────────╮
│What     Skill entry file · global · The entry file of the frontend-design skill: frontmatter wit…│
│From     ~/.claude/skills/frontend-design/SKILL.md                                                │
│Loads    Claude Code: Only when invoked — Full text loads when the skill is invoked               │
│Related  5 supporting files: assets/template.json, references/accessibility.md, references/patter…│
╰──────────────────────────────────────────────────────────────────────────────────────────────────╯
↑↓ scroll  [ ] links  enter open  esc back  / find  n/N next  v raw  e explain  ? help
```

AYCA follows links only to files inside the item's own folder, the project
and the agents' config folders. Links are resolved relative to the document,
and symlink cycles aren't followed.

### Explaining what you see

When the terminal is at least 26 lines tall, a four-line strip under the
panes describes the selected item:

```
What     Skill · global · A skill: a folder with a SKILL.md and optional supporting files…
From     ~/.claude/skills/pdf → ~/.agents/skills/pdf · +1 other location
Loads    Claude Code: Listed (name + description) — Name and description listed at startup…
Related  ~/.claude/skills/pdf is a symlink to ~/.agents/skills/pdf · 2 supporting files…
```

Press `e` for the full explanation, which answers five questions:

- **What is this?** The item's role. For a skill, subagent or command, its
  own description.
- **Where does it come from?** The full path, real path, other locations,
  scope, plugin and parent item.
- **How does the agent discover it?** The discovery rule for the selected
  agent, for example "Claude Code starts in the working directory and walks
  up to the filesystem root, reading CLAUDE.md…".
- **When does it load?** The loading state for each agent that reads it, with
  the reason.
- **How does it relate to other context?** Imports, symlinks, aliases,
  shadowing, the plugin that provides it, supporting files and missing
  references.

#### Loading states

| State | Meaning |
| --- | --- |
| Startup | In context from the start of every session (instruction files, imports, unconditional rules, `MEMORY.md`, the active output style, built-in prompts). |
| Listed | Only the name and description are in context. The full content loads when it is used (skills, subagents, Claude commands with a description, Cursor *Agent Requested* rules). |
| On demand | Loaded when the agent works in that path or follows a pointer to it (nested `CLAUDE.md`, a skill's supporting files). |
| Conditional | Applies when a condition matches (Claude rules with `paths:`, Cursor rules with `globs`). |
| Only when invoked | Loaded only when you call it (`/prompts:<name>` in Codex, manual Cursor rules, skills with `disable-model-invocation`). |
| Runs on events | A hook. It isn't context itself, but its output can be (SessionStart, UserPromptSubmit). |
| Enforced by the program | Applied by the agent program, never sent to the model (Codex `.rules` execution policy). |
| Disabled / Shadowed / Not loaded | Present on disk but not used: a disabled plugin, an `AGENTS.md` overridden by `AGENTS.override.md`, a file above the Codex project root, a broken import. |
| Unknown | Depends on runtime state (MCP servers, claude.ai connectors, launch flags). |

*Listed* and *Startup* describe what takes up context before you type
anything. *Available* is not the same as *consumed*: a listed skill costs its
description until it is used.

### Effective context

Press `2` for the Effective view, and `d` to pick a working directory. The
picker lists the project root, the directory you started in, and every
directory with its own instructions or rules. AYCA then rescans as if the
agent had been started there and groups the result:

Claude Code started in `services/api`. Its own `CLAUDE.md` has moved into *Applicable*:

```
AYCA — All Your Context Are Belong To Us  Know what your agents know.
Agent: Claude Code (a)  ·  View: Effective (1-4)  ·  Working dir: services/api (d)
╭──────────────────────────────────────╮╭──────────────────────────────────────────────────────────╮
│▼ Applicable — loaded at startup (11) ││Applicable — loaded at startup                            │
│  ├─ • ~/.claude/CLAUDE.md startup    ││                                                          │
│  ├─ • @~/.claude/shared/style.md sta…││                                                          │
│  ├─ • ~/.claude/rules/security.md st…││   Applicable — loaded at startup                         │
│  ├─ • CLAUDE.md startup              ││                                                          │
│  ├─ • @docs/architecture.md startup  ││  Read at the start of every session from this working    │
│  ├─ • CLAUDE.local.md startup        ││  directory. These take up context before you type        │
│  ├─ • services/api/CLAUDE.md startup ││  anything.                                               │
│  ├─ • ~/code/CLAUDE.md startup       ││                                                          │
│  ├─ • .claude/rules/style.md startup ││  Shown for Claude Code starting in  services/api/ .      │
│  ├─ ◇ Claude Code system prompt star…││                                                          │
│  └─ ◇ Session environment startup    ││  How things load                                         │
│▼ Discoverable — listed, loaded when …││                                                          │
│  ├─ ▸ deploy listed                  ││  • Startup — Loaded into the agent's context at the      │
│  ├─ ▸ frontend-design listed         ││  start of every session.                                 │
│  ├─ ▸ pdf ⇆1 listed                  ││  • Listed (name + description) — Only the name and       │
│  ├─ • ~/.claude/agents/reviewer.md l…││  description take up context at startup; the full        │
│  └─ • ~/.claude/commands/ship.md lis…││  content loads when it is invoked or used.               │
│▼ Conditional / on demand (3)         ││  • Conditional — Applies only when a condition (file     │
│  ├─ • .claude/rules/backend.md condi…││  patterns or a nested directory) matches.                │
│  ├─ ▸ SessionStart · ~/.claude/hooks…││  • On demand — Not loaded at startup; loaded when the    │
╰──────────────────────────────────────╯╰──────────────────────────────────────────────────────────╯
╭──────────────────────────────────────────────────────────────────────────────────────────────────╮
│What     Applicable — loaded at startup (11) — Read at the start of every session from this worki…│
│                                                                                                  │
│                                                                                                  │
│                                                                                                  │
╰──────────────────────────────────────────────────────────────────────────────────────────────────╯
↑↓ move  ←→ fold  enter read  1-4 view  a agent  d dir  e explain  / filter  ? help  q quit
```

- **Applicable**: loaded at startup from this directory.
- **Discoverable**: listed at startup, loaded when used.
- **Conditional / on demand**: rules with file patterns, nested
  instructions, supporting files and hooks.
- **Not loaded**: disabled, shadowed or ignored, each with the reason.
- **Unknown**: depends on runtime state.
- **Excluded from this working directory**: in the project, but not on the
  path from the root to this directory (for example `web/CLAUDE.md` when you
  pick `services/api`).

Switch agents with `a` to see how the same project looks to Codex or Cursor.
The Hierarchy view follows the selected working directory too. The Inventory
and Filesystem views always show the full inventory from where you started
AYCA.

This is a projection based on each agent's documented discovery rules, not a
recording of a live session (see [How AYCA models loading](#how-ayca-models-loading)).

## Keyboard shortcuts

Press `?` in AYCA for the same list.

**Everywhere**

| Key | Action |
| --- | --- |
| `1` `2` `3` `4` | Hierarchy, Effective, Filesystem, Inventory view |
| `a` | Next agent |
| `d` | Choose the working directory (`↑` `↓`, `enter`, `esc` in the picker) |
| `e` | Full explanation of the selected item (press again to close) |
| `tab` | Switch between tree and preview |
| `v` | Rendered or raw view |
| `c` | Startup context budget per agent |
| `w` | Discovery warnings (unreadable or malformed files) |
| `r` | Rescan |
| `?` | Keyboard help |
| `q` / `ctrl+c` | Quit |

**Context tree**

| Key | Action |
| --- | --- |
| `↑` `↓` / `k` `j` | Move |
| `←` / `h` | Collapse, or jump to parent |
| `→` / `l` | Expand, or step into the first child |
| `space` | Toggle expand/collapse |
| `enter` | Read the item in the preview; on a group, toggle it |
| `pgup` `pgdn` / `ctrl+u` `ctrl+d` | Page |
| `g` `G` / `home` `end` | First / last |
| `/` | Filter by name or path (`enter` keeps the filter, `esc` clears it) |
| `esc` | Clear the filter or close a panel |

**Preview (reading)**

| Key | Action |
| --- | --- |
| `↑` `↓` / `k` `j`, `pgup` `pgdn` | Scroll |
| `g` `G` / `home` `end` | Top / bottom |
| `]` `[` | Next / previous link in the document |
| `enter` | Open the selected link |
| `esc` / `backspace` | Go back to the previous document, or to the tree |
| `←` / `h` | Back to the tree |
| `/` | Find in the document; `n` `N` next / previous match, `esc` clears |

Below 72 columns AYCA shows one pane at a time (`tab`, `enter` and `esc`
switch). Below 26 lines the explanation strip is hidden, but `e` still opens
the full explanation.

## Reading the tree

- **Colours mark scope.** Project (green) is inside the project root, which
  is the nearest git root or else the current directory. Ancestor (yellow) is
  above the project root. Global (blue) is user-wide configuration. System
  (red) is machine-wide managed policy set by an administrator, plus context
  built into the agents themselves.
- In the Hierarchy view, groups follow the agent's own structure. In the
  Inventory view, items are grouped by **category** (Instructions, Memory,
  Rules, Skills, Subagents, Commands, Output styles, Hooks, MCP Servers,
  Runtime), then by scope. Selecting a group shows what it means for the
  selected agent.
- In the Inventory and Filesystem views, the tags after a name
  (`claude·codex`) are the agents known to read that location. Badges such as
  `startup`, `listed`, `conditional`, `disabled` and `overridden` give the
  loading state (Effective view) or a special status.
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
- In the Filesystem view, `→ ~/…` marks a symlink or alias location, and
  `⇆ N locations` marks the physical folder or file behind it.
- The preview header shows a breadcrumb, the absolute path, size and a rough
  token estimate (bytes ÷ 4). The explanation strip below the panes gives the
  kind, origin, loading state and related context.

## Let an agent prune your context

`ayca --json` gives agents the same data as the TUI. It lists every item with
its absolute path, scope, the agents that read it, size, estimated tokens and
whether it loads at session start, followed by the startup budget per agent.
The format is versioned (`schema_version`, still `1`). Two optional fields
were added without breaking it:

- `found`: how each agent finds the item. Each entry has `agent`, `path`,
  `mechanism` (for example `user-skills` or `nested-instructions`), and
  `provider`, `disabled` and `shadowed_by` when they apply.
- `load`: the loading state per agent. Each entry has `agent`, `state` (one
  of the [loading states](#loading-states) as `startup`, `listed`,
  `on-demand`, `conditional`, `invocation`, `event`, `enforced`, `disabled`,
  `shadowed`, `not-loaded`, `unknown`), `reason` and `confidence`
  (`documented`, `version-dependent`, `runtime-dependent`).

The repo includes a skill that uses it to audit your setup and suggest cuts:
[`skills/prune-context`](skills/prune-context/SKILL.md).

### Install the skill

You need `ayca` v0.2.0 or later on your `PATH` (`ayca --version`), because
the skill runs `ayca --json`. Then put the skill folder where your agent looks
for skills:

| Agent | User-wide (all projects) | One project only |
| --- | --- | --- |
| Claude Code | `~/.claude/skills/prune-context/` | `<project>/.claude/skills/prune-context/` |
| Codex | `~/.agents/skills/prune-context/` | |

To keep the skill up to date, link it to a clone of this repo. A `git pull`
then updates it:

```sh
git clone https://github.com/R037/ayca ~/src/ayca
mkdir -p ~/.claude/skills
ln -s ~/src/ayca/skills/prune-context ~/.claude/skills/prune-context
```

Or take a one-off copy instead:

```sh
cp -r ~/src/ayca/skills/prune-context ~/.claude/skills/
```

Agents load skills when a session starts, so open a new session after you
install it. Other agents that can run shell commands can use the skill too:
tell them to read `skills/prune-context/SKILL.md` and follow it.

### Use it

Start a session in the project you want to check and ask, for example:

- "Prune my context."
- "Why is my startup context so big?"
- "Find duplicate or stale instructions in my CLAUDE.md and AGENTS.md."
- "Which of my skills and MCP servers could I drop?"

In Claude Code you can also run it directly with `/prune-context`.

The agent then:

1. Runs `ayca --json` in the project (add "for Codex only" or similar to
   limit it to one agent).
2. Reads the largest items that load at startup.
3. Looks for broken imports, unused skills and MCP servers, instructions
   duplicated across scopes, bloated files, long skill descriptions and stale
   memory. It asks you which skills and servers you actually use, because
   ayca can't see usage.
4. Proposes cuts in a table: what to remove, trim, merge, move or fix, the
   tokens each saves, and the risk.
5. Makes only the changes you approve, preferring reversible ones (archiving
   instead of deleting), then runs ayca again to show the before and after.

Token counts are estimates (bytes ÷ 4) of files on disk only. The agent's
built-in prompt and MCP tool definitions aren't included. In Claude Code,
`/context` shows the real totals. AYCA itself stays read-only.

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

## How AYCA models loading

Loading states and explanations come from explicit, per-agent rules in
`internal/effective` and `internal/explain`. They are based on Claude Code
2.1.328 and Codex CLI 0.167.1 (the versions installed when the rules were
written) and on Cursor's documentation (Cursor itself wasn't installed). The
main assumptions:

- **Claude Code** walks up from the working directory to `/`, loading every
  `CLAUDE.md`, `.claude/CLAUDE.md` and `CLAUDE.local.md` on the way, plus
  managed policy, `~/.claude/CLAUDE.md`, unconditional rules and the first 200
  lines of `MEMORY.md`. Files below the working directory load when Claude
  reads files there. Skills, subagents and described commands are listed by
  name and description. A skill with `disable-model-invocation: true` is only
  available as `/name`.
- **Codex** reads `$CODEX_HOME/AGENTS.md` and the `AGENTS.md` files from the
  git root down to the working directory, preferring `AGENTS.override.md` in
  each directory, capped at 32 KiB. It doesn't read above the git root.
  Whether it reads nested `AGENTS.md` files below the working directory is up
  to the model, following Codex's built-in instructions, so AYCA marks this
  *version-dependent*. Execution-policy `.rules` are enforced by the CLI and
  never sent to the model.
- **Cursor** reads the root `AGENTS.md` and `.cursor/rules`. The rule type
  comes from its frontmatter: `alwaysApply` makes it Always, `globs` makes it
  Auto Attached, a description alone makes it Agent Requested, and anything
  else is Manual. Nested `AGENTS.md` and `.cursor/rules` apply when the agent
  works in that directory.
- **MCP servers** are marked *Unknown*. Their tools appear only once a server
  starts, project servers may need approval, and agents can defer tool
  definitions. AYCA never starts servers.

When a rule depends on the agent version or on runtime state, the
explanation says so (*version-dependent* or *runtime-dependent*). Agents
change. If you see a mismatch with your version, the rules live in one place
and are covered by tests.

## What AYCA does *not* know

AYCA shows what is **configured on disk**. It can't see what an agent actually
received in a given session. Agents may skip files, load them lazily, or apply
precedence rules that change between versions. The loading states, the
Effective view and notes like "likely not loaded" describe documented
behaviour; they don't come from inspecting a running agent. Other known limits:

- The Effective view rescans as if the agent had started in the chosen
  directory. It can't know about `--add-dir`, launch flags, settings passed on
  the command line, or which project MCP servers you approved.
- Conditional rules are shown as conditional. AYCA doesn't evaluate whether
  their globs match the files you will touch.
- Cursor's User Rules, Memories and built-in prompt live inside the app and
  can't be read.
- Duplicate detection is by physical file (symlinks and shared folders). Two
  files with similar or identical text at different paths aren't flagged yet.
- Links are found in Markdown links, path-like text and `@imports`. A link
  that wraps across rendered lines scrolls to its first line.

## Development

```sh
make test     # go test ./...
make lint     # gofmt check + go vet
make build    # ./bin/ayca
```

One discovery inventory powers every view. Discovery records *how* each item
was found. Separate packages classify, explain and present it:

```
cmd/ayca/              CLI flags, --list output, program start-up
internal/discovery/    Env (paths), agent adapters (claude.go, codex.go, cursor.go), hooks,
                       @imports, runtime info, de-duplication, MCP parsing & masking.
                       Every item records Findings: agent, path, mechanism, plugin,
                       disabled, shadowed-by
internal/effective/    Per-agent loading rules (Classify) and working-directory
                       projection (Project, CandidateDirs)
internal/explain/      Rule-based explanations; knowledge.go maps (agent, mechanism)
                       to how discovery works
internal/frontmatter/  Small YAML frontmatter parser shared by budget, preview, effective
internal/budget/       Startup context estimate per agent (tokens, % of window)
internal/tree/         Hierarchy, Filesystem and Inventory trees, expansion state, filtering
internal/preview/      Read-only file loading, Glamour rendering, frontmatter,
                       document links, in-document search
internal/tui/          Bubble Tea model: views, picker, reader history, explain strip
internal/testfixture/  A realistic fake home + project used by the tests
```

To try AYCA against the test fixture without touching your own config:

```sh
go run ./internal/testfixture/demo /tmp/ayca-demo
HOME=/tmp/ayca-demo/home ayca --path /tmp/ayca-demo/ws/proj
```

To add an agent, write a `discoverX(c *collector)` function in its own file
and register it in `Adapters` in `internal/discovery/adapters.go`. Use
`c.file`, `c.fileDir`, `c.skills`, `c.hooksFile`, `c.mcpJSONFile`,
`c.mcpTOMLFile` and `c.info`, each wrapped in `c.via(<mechanism>, …)`. The
collector handles de-duplication, symlink resolution, scope fixes and
warnings. Then add the agent's loading rules to `internal/effective`, its
discovery texts to `internal/explain/knowledge.go`, and its group help to
`internal/tree/hierarchy.go`. The completeness tests tell you what's missing.

## License

MIT. See [LICENSE](LICENSE).
