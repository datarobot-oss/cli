package tui

import "github.com/charmbracelet/lipgloss"

// Common style definitions using DataRobot branding
var (
	// Adaptive colors for light/dark terminals
	BorderColor = GetAdaptiveColor(DrPurpleLight, DrPurpleDarkLight)

	BaseTextStyle = lipgloss.NewStyle().Foreground(GetAdaptiveColor(DrPurple, DrPurpleDark))
	ErrorStyle    = lipgloss.NewStyle().Foreground(DrRed).Bold(true)
	InfoStyle     = lipgloss.NewStyle().Foreground(GetAdaptiveColor(DrPurpleLight, DrPurpleDarkLight)).Bold(true)
	DimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	TitleStyle    = BaseTextStyle.Foreground(GetAdaptiveColor(DrGreen, DrGreenDark)).Bold(true).MarginBottom(1)

	// Specific UI styles
	LogoStyle     = BaseTextStyle
	WelcomeStyle  = BaseTextStyle.Bold(true)
	SubTitleStyle = BaseTextStyle.Bold(true).
			Foreground(DrPurpleLight).
			BorderBottom(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(DrGreen)
	BoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(DrPurple).
			Padding(1, 2)
	NoteBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(BorderColor).
			Padding(0, 1)
	TableBorderStyle = lipgloss.NewStyle().Foreground(BorderColor)
	StatusBarStyle   = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(DrPurpleLight).
				Foreground(DrPurpleLight).
				Padding(0, 1)
)
