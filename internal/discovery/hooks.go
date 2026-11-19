package discovery

import (
	"fmt"
	"path/filepath"
	"strings"
)

// hooksFile adds the hooks configured under "hooks" in a JSON settings file
// (Claude Code settings, plugin hooks.json, Codex hooks.json, Cursor hooks.json).
// pluginRoot expands ${CLAUDE_PLUGIN_ROOT} in commands.
func (c *collector) hooksFile(path string, scope Scope, note, pluginRoot string, agents ...string) {
	doc, ok := c.readJSON(path)
	if !ok {
		return
	}
	hooks, _ := doc["hooks"].(map[string]any)
	c.hooks(path, hooks, scope, note, pluginRoot, agents...)
}

// hooks adds one virtual item per hook command. Two layouts are accepted:
//
//	Claude/Codex: {"Event": [{"matcher": "...", "hooks": [{"type": "command", "command": "..."}]}]}
//	Cursor:       {"event": [{"command": "..."}]}
//
// Hooks are never executed; scripts they reference are listed as children so
// their source can be read.
func (c *collector) hooks(path string, hooks map[string]any, scope Scope, note, pluginRoot string, agents ...string) {
	for _, event := range sortedKeys(hooks) {
		groups, _ := hooks[event].([]any)
		for gi, g := range groups {
			group, _ := g.(map[string]any)
			if group == nil {
				continue
			}
			matcher, _ := group["matcher"].(string)
			inner, nested := group["hooks"].([]any)
			if !nested {
				inner = []any{group}
			}
			for hi, h := range inner {
				hook, _ := h.(map[string]any)
				if hook == nil {
					continue
				}
				cfg := map[string]any{"event": event}
				if matcher != "" {
					cfg["matcher"] = matcher
				}
				for k, v := range hook {
					cfg[k] = v
				}
				command, _ := hook["command"].(string)
				c.add(Item{
					ID:       fmt.Sprintf("%d.%d", gi, hi),
					Name:     event + " · " + c.hookSummary(hook, pluginRoot),
					Path:     path,
					Category: CategoryHook,
					Scope:    scope,
					Agents:   agents,
					Virtual:  true,
					Note:     joinNote(hookEventNote(event), note),
					Config:   MaskSecrets(cfg).(map[string]any),
					Children: c.hookScripts(command, filepath.Dir(path), pluginRoot, scope, agents),
				})
			}
		}
	}
}

func (c *collector) hookSummary(hook map[string]any, pluginRoot string) string {
	if cmd, ok := hook["command"].(string); ok && cmd != "" {
		cmd = maskString(cmd, "command", false)
		if pluginRoot != "" {
			cmd = strings.ReplaceAll(cmd, pluginRoot, "<plugin>")
		}
		cmd = strings.ReplaceAll(cmd, "${CLAUDE_PLUGIN_ROOT}", "<plugin>")
		if c.env.Home != "" {
			cmd = strings.ReplaceAll(cmd, c.env.Home, "~")
		}
		return shorten(cmd, 48)
	}
	if p, ok := hook["prompt"].(string); ok {
		return "prompt: " + shorten(p, 40)
	}
	if t, ok := hook["type"].(string); ok {
		return t
	}
	return "hook"
}

// hookEventNote explains what a hook event means for the agent's context.
func hookEventNote(event string) string {
	switch strings.ToLower(event) {
	case "sessionstart":
		return "runs when a session starts — its output is added to the agent's context"
	case "userpromptsubmit", "beforesubmitprompt":
		return "runs on every prompt — its output can be added to the agent's context"
	case "pretooluse", "posttooluse", "beforeshellexecution", "beforemcpexecution", "afterfileedit", "beforereadfile":
		return "runs around tool use — can block actions or send feedback to the agent"
	case "stop", "subagentstop":
		return "runs when the agent finishes — can make it continue"
	case "precompact":
		return "runs before the conversation is compacted"
	default:
		return "runs on " + event
	}
}

// hookScripts finds files referenced by a hook command: absolute or
// variable-based paths, paths relative to the config file, and arguments
// naming a file next to such a script (e.g. `run-hook.cmd session-start`).
func (c *collector) hookScripts(command, baseDir, pluginRoot string, scope Scope, agents []string) []Item {
	if command == "" {
		return nil
	}
	e := c.env
	expand := func(tok string) string {
		r := strings.NewReplacer(
			"${CLAUDE_PLUGIN_ROOT}", pluginRoot, "$CLAUDE_PLUGIN_ROOT", pluginRoot,
			"${CLAUDE_PROJECT_DIR}", e.ProjectRoot, "$CLAUDE_PROJECT_DIR", e.ProjectRoot,
			"${HOME}", e.Home, "$HOME", e.Home,
		)
		tok = r.Replace(tok)
		if strings.HasPrefix(tok, "~/") {
			tok = filepath.Join(e.Home, tok[2:])
		}
		return tok
	}
	var out []Item
	seen := map[string]bool{}
	addFile := func(p string) bool {
		p = filepath.Clean(p)
		if seen[p] || !fileExists(p) {
			return false
		}
		seen[p] = true
		out = append(out, Item{Name: filepath.Base(p), Path: p, Category: CategoryHook, Scope: scope,
			Agents: agents, Note: "script run by this hook (shown, never executed)"})
		return true
	}
	tokens := shellFields(command)
	var scriptDirs []string
	for _, tok := range tokens {
		t := expand(tok)
		if strings.Contains(tok, "$") && t == tok {
			continue // unknown variable
		}
		var p string
		switch {
		case filepath.IsAbs(t):
			p = t
		case strings.HasPrefix(t, "./") || strings.HasPrefix(t, "../"):
			p = filepath.Join(baseDir, t)
			// Claude Code runs hooks from the project directory.
			if !fileExists(p) && e.ProjectRoot != "" {
				p = filepath.Join(e.ProjectRoot, t)
			}
		case !strings.HasPrefix(t, "-") && strings.Contains(t, "/") && e.ProjectRoot != "" && fileExists(filepath.Join(e.ProjectRoot, t)):
			p = filepath.Join(e.ProjectRoot, t)
		default:
			continue
		}
		if addFile(p) {
			scriptDirs = append(scriptDirs, filepath.Dir(filepath.Clean(p)))
		}
	}
	// Bare arguments that name a file next to a referenced script.
	for _, tok := range tokens {
		if strings.ContainsAny(tok, "/$=") || strings.HasPrefix(tok, "-") {
			continue
		}
		for _, d := range scriptDirs {
			addFile(filepath.Join(d, tok))
		}
	}
	return out
}

// shellFields splits a command line on whitespace, honouring single and
// double quotes. It is deliberately simple: good enough to find paths.
func shellFields(s string) []string {
	var out []string
	var cur strings.Builder
	var quote rune
	inTok := false
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, inTok = r, true
		case r == ' ' || r == '\t' || r == '\n' || r == ';' || r == '&' || r == '|':
			if inTok {
				out = append(out, cur.String())
				cur.Reset()
				inTok = false
			}
		default:
			cur.WriteRune(r)
			inTok = true
		}
	}
	if inTok {
		out = append(out, cur.String())
	}
	return out
}

func shorten(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
