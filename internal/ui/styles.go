package ui

import "github.com/charmbracelet/lipgloss"

var (
	DimStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	ProjectStyle   = lipgloss.NewStyle().Bold(true)
	InfoStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	SafeStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	CautionStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	ProtectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	ActiveStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	RecentStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	StaleStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	DormantStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	TotalStyle     = lipgloss.NewStyle().Bold(true)
	HeaderStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	ErrStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)
