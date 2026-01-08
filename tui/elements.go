package tui

import "github.com/charmbracelet/lipgloss"

// Header renders the common header with DataRobot logo
func Header() string {
	style := lipgloss.NewStyle().
		Background(DrGreen).
		Foreground(DrBlack).
		Padding(1, 2)

	return style.Render(Banner)
}

// Footer renders the common footer with quit instructions
func Footer() string {
	return BaseTextStyle.Render("Press q or Ctrl+C to quit")
}
