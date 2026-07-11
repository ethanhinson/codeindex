package tree

import "github.com/charmbracelet/lipgloss"

// Adaptive colors keep the view readable on both light and dark terminals.
var (
	accent    = lipgloss.AdaptiveColor{Light: "63", Dark: "111"}
	muted     = lipgloss.AdaptiveColor{Light: "245", Dark: "243"}
	borderCol = lipgloss.AdaptiveColor{Light: "250", Dark: "238"}
	cursorBg  = lipgloss.AdaptiveColor{Light: "254", Dark: "236"}

	headerStyle = lipgloss.NewStyle().Bold(true).Padding(0, 1)
	countStyle  = lipgloss.NewStyle().Foreground(muted)
	footerStyle = lipgloss.NewStyle().Foreground(muted).Padding(0, 1)
	paneStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
			BorderForeground(borderCol).Padding(0, 1)
	cursorStyle = lipgloss.NewStyle().Background(cursorBg).Bold(true)
	dirStyle    = lipgloss.NewStyle().Bold(true)
	badgeStyle  = lipgloss.NewStyle().Foreground(muted)
	matchStyle  = lipgloss.NewStyle().Foreground(accent)
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(accent)
)
