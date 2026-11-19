---
name: prune-context
description: Audit and prune the context an AI coding agent loads (instruction files, memory, rules, skills, subagents, hooks, MCP servers) using the ayca CLI. Use when the user wants to slim down, clean up, deduplicate or optimize their CLAUDE.md / AGENTS.md / Cursor rules, skills or MCP setup, asks why their context is so large, or asks what their agents load at startup.
---

# Prune agent context with ayca

`ayca` lists every piece of context that Claude Code, Codex and Cursor can load
for a project, and estimates how much of it is loaded at session start. You
use its output to find waste and propose cuts. **You never edit or delete
anything until the user approves each change.**

## 1. Collect

Run this from the project the user cares about (or pass `--path <dir>`):

```sh
ayca --json
```

Add `--agent claude` (or `codex`, `cursor`) to look at one agent only. If
`ayca` is not found, tell the user to install it:

```sh
go install github.com/R037/ayca/cmd/ayca@latest   # then make sure $(go env GOPATH)/bin is on PATH
```

Don't install it yourself unless they ask you to.

## 2. Read the output

The JSON has `items`, `budget` and `warnings`.

- `budget[].entries`: what each agent loads **at startup**, largest first.
  `tokens` is an estimate (bytes ÷ 4), and `how` says what counts:
  `full text`, `name + description` (skills: only the frontmatter is
  loaded until the skill is used) or `first 200 lines` (MEMORY.md).
- `budget[].not_counted`: startup context ayca knows about but can't size,
  such as MCP tool definitions and the agent's own system prompt. These are
  often the largest part. In Claude Code, `/context` shows the real totals.
- `items[]`: everything found. `scope` is `project`, `ancestor`, `global` or
  `system`. `deferred_for` lists agents that load the item only on demand
  (nested files, shadowed files, disabled plugins). Those cost nothing until
  they are used. `children` holds `@imports` and hook scripts, and
  `missing: true` marks a broken import. `aliases` are other paths for the
  same file.
- `warnings`: files that couldn't be read or parsed.

Startup entries cost tokens in **every** session, so focus on them first.
Read the files behind the largest entries before you judge them.

## 3. Look for waste

Go through these in order. The first ones cost the user least to fix.

1. **Free fixes**: broken `@imports`, files flagged as shadowed or likely not
   loaded, and `warnings`. Each one is either dead weight or a sign that
   something the user expects to load doesn't.
2. **Unused skills, plugins, subagents and MCP servers.** ayca can't see
   usage, so ask the user which ones they use. Group them by family
   (for example 20 `aws-*` skills) and give the token total per group, so
   they can drop a whole family in one decision.
3. **Duplication across scopes**: the same rules in a global and a project
   file, or near-identical `CLAUDE.md` and `AGENTS.md`. Propose keeping one
   copy and importing it from the other (`@AGENTS.md` in `CLAUDE.md`).
4. **Bloated instruction files.** Cut what the agent can find out by itself
   (directory trees, dependency lists, how to read the code), generic advice
   ("write clean code", "add tests"), stale facts, and long examples.
   Keep commands, non-obvious constraints, conventions and the reasons
   behind them.
5. **Long skill and subagent descriptions.** The description is loaded every
   session. Trim it to what the skill does and when to use it, and move the
   rest into the body.
6. **Memory.** Look for stale, duplicate or contradictory memory files, and
   a `MEMORY.md` longer than the 200 lines that get loaded.
7. **Ancestor files** (`scope: ancestor`) that apply to this project by
   accident.
8. **Move instead of cut**: guidance that matters only in one subdirectory
   can go in a nested file, and a rarely needed procedure can become a
   skill. Both load only when needed.

## 4. Propose

Show a table, largest saving first:

| Item | Path | Scope | Startup tokens | Proposal | Saves | Risk |
| --- | --- | --- | --- | --- | --- | --- |

Proposals are one of: **remove**, **trim**, **merge**, **move** (to on demand)
or **fix**. Give the total saving per agent, both in tokens and as a share of
its current startup estimate. For a trim, show the exact text you'd remove.

Point out the scope of each change. A `global` change affects every project,
and `system` items are managed policy that the user can't usually edit.

## 5. Apply only what is approved

- Make only the changes the user approves, one item at a time if they want.
- Prefer reversible changes. Move removed skills or files to an archive
  folder instead of deleting them, and keep a list of what you moved.
- For plugin skills, disable the plugin rather than deleting files in the
  plugin cache. For MCP servers, use the agent's own command (for example
  `claude mcp remove <name>`). Skills synced from an account may come back
  if you delete them locally, so point the user to where they're managed.
- Never print secrets. ayca already masks them in `config`. Keep it that
  way if you read the underlying config files.

Afterwards, run `ayca --json` again and report the before and after startup
totals per agent.
