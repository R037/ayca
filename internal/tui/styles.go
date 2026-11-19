package tui

import (
	"github.com/charmbracelet/lipgloss"

	"ayca/internal/discovery"
)

var (
	colAccent   = lipgloss.AdaptiveColor{Light: "#7D3CCF", Dark: "#B48CFF"}
	colMuted    = lipgloss.AdaptiveColor{Light: "#6B6B6B", Dark: "#8A8A8A"}
	colBorder   = lipgloss.AdaptiveColor{Light: "#C8C8C8", Dark: "#444444"}
	colFocus    = lipgloss.AdaptiveColor{Light: "#7D3CCF", Dark: "#B48CFF"}
	colSelBg    = lipgloss.AdaptiveColor{Light: "#E6DAFA", Dark: "#3A2F52"}
	colProject  = lipgloss.AdaptiveColor{Light: "#1A7F37", Dark: "#7EE787"}
	colAncestor = lipgloss.AdaptiveColor{Light: "#9A6700", Dark: "#E3B341"}
	colGlobal   = lipgloss.AdaptiveColor{Light: "#0969DA", Dark: "#79C0FF"}
	colWarn     = lipgloss.AdaptiveColor{Light: "#BC4C00", Dark: "#FFA657"}

	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	taglineStyle  = lipgloss.NewStyle().Italic(true).Foreground(colMuted)
	mutedStyle    = lipgloss.NewStyle().Foreground(colMuted)
	categoryStyle = lipgloss.NewStyle().Bold(true)
	matchStyle    = lipgloss.NewStyle().Underline(true)
	warnStyle     = lipgloss.NewStyle().Foreground(colWarn)
	keyStyle      = lipgloss.NewStyle().Bold(true)
	labelStyle    = lipgloss.NewStyle().Foreground(colMuted).Width(9)
	selectedStyle = lipgloss.NewStyle().Background(colSelBg).Bold(true)
)

func scopeColor(s discovery.Scope) lipgloss.AdaptiveColor {
	switch s {
	case discovery.ScopeProject:
		return colProject
	case discovery.ScopeAncestor:
		return colAncestor
	default:
		return colGlobal
	}
}

func scopeStyle(s discovery.Scope) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(scopeColor(s))
}

func paneStyle(focused bool) lipgloss.Style {
	c := colBorder
	if focused {
		c = colFocus
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(c)
}
