package tui

import (
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/R037/ayca/internal/discovery"
	"github.com/R037/ayca/internal/effective"
)

// picker is the working-directory chooser overlay.
type picker struct {
	dirs []string
	cur  int
}

type atDoneMsg struct {
	dir string
	res discovery.Result
}

type fullDoneMsg discovery.Result

func (m Model) fullOrBase() discovery.Result {
	if m.fullReady {
		return m.full
	}
	return m.base
}

func (m *Model) openPicker() {
	dirs := effective.CandidateDirs(m.base, m.fullOrBase())
	cur := 0
	for i, d := range dirs {
		if d == m.wd {
			cur = i
		}
	}
	m.picker = &picker{dirs: dirs, cur: cur}
}

func (m Model) updatePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := *m.picker
	switch msg.String() {
	case "up", "k":
		p.cur--
	case "down", "j":
		p.cur++
	case "home", "g":
		p.cur = 0
	case "end", "G":
		p.cur = len(p.dirs) - 1
	case "esc", "d", "q":
		m.picker = nil
		return m, nil
	case "enter":
		dir := p.dirs[max(0, min(p.cur, len(p.dirs)-1))]
		m.picker = nil
		return m.selectWd(dir)
	}
	p.cur = max(0, min(p.cur, len(p.dirs)-1))
	m.picker = &p
	return m, nil
}

// selectWd makes dir the working directory. The launch directory reuses the
// startup scan; others are discovered in the background.
func (m Model) selectWd(dir string) (tea.Model, tea.Cmd) {
	if dir == m.wd && !m.atLoading {
		return m, nil
	}
	m.wd = dir
	if dir == m.base.Env.Cwd {
		m.atLoading = false
		m.setAt(m.base)
		return m, nil
	}
	m.atLoading = true
	env, agents := m.env, m.agents
	return m, func() tea.Msg {
		return atDoneMsg{dir: dir, res: discovery.Discover(env.WithCwd(dir), agents)}
	}
}

// pickerView draws the overlay inside the tree pane.
func (m Model) pickerView(width, height int) []string {
	p := m.picker
	root := m.base.Env.ProjectRoot
	lines := []string{
		categoryStyle.Render("Working directory"),
		mutedStyle.Render("Where the agent is started. Relative to the project root."),
		"",
	}
	room := max(height-len(lines), 1)
	start := max(0, min(p.cur-room/2, len(p.dirs)-room))
	for i := start; i < len(p.dirs) && i < start+room; i++ {
		d := p.dirs[i]
		rel, err := filepath.Rel(root, d)
		if err != nil {
			rel = m.env.Tilde(d)
		}
		var tags []string
		if d == m.base.Env.Cwd {
			tags = append(tags, "launch dir")
		}
		if d == m.wd {
			tags = append(tags, "selected")
		}
		tag := ""
		if len(tags) > 0 {
			tag = "  (" + strings.Join(tags, ", ") + ")"
		}
		marker := "  "
		if d == m.wd {
			marker = "● "
		}
		plain := marker + rel + tag
		if i == p.cur {
			lines = append(lines, selectedStyle.Width(width).Render(ansi.Truncate(plain, width, "…")))
		} else {
			lines = append(lines, ansi.Truncate(marker+dirStyle.Render(rel)+mutedStyle.Render(tag), width, "…"))
		}
	}
	return lines
}
