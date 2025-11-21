// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package component

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/internal/copier"
	"github.com/datarobot/cli/tui"
)

// getScrollPercent calculates the scroll percentage for the viewport
func (m Model) getScrollPercent() int {
	if m.viewport.TotalLineCount() <= m.viewport.Height {
		return 100 // If content fits in viewport, we're at 100%
	}

	maxScroll := m.viewport.TotalLineCount() - m.viewport.Height
	if maxScroll <= 0 {
		return 100
	}

	if m.viewport.AtBottom() {
		return 100
	}

	return int(float64(m.viewport.YOffset) / float64(maxScroll) * 100)
}

// getCurrentComponentFileName returns the filename of the currently selected component
func (m Model) getCurrentComponentFileName() string {
	if len(m.list.VisibleItems()) == 0 {
		return ""
	}

	item := m.list.VisibleItems()[m.list.Index()].(ListItem)

	return item.component.FileName
}

// getComponentDetailContent generates the markdown content for component details
func (m Model) getComponentDetailContent() string {
	var sb strings.Builder

	item := m.list.VisibleItems()[m.list.Index()].(ListItem)
	selectedComponent := item.component
	selectedComponentDetails := copier.ComponentDetailsByURL[selectedComponent.Repo]

	style := "light"
	if lipgloss.HasDarkBackground() {
		style = "dark"
	}

	readMe, _ := glamour.Render(selectedComponentDetails.ReadMeContents, style)
	sb.WriteString(readMe)

	return sb.String()
}

// renderStatusBar creates the status bar for the detail view
func (m Model) renderStatusBar() string {
	fileName := m.getCurrentComponentFileName()
	scrollPercent := m.getScrollPercent()

	// Style definitions
	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFDF5"}).
		Background(lipgloss.AdaptiveColor{Light: "#6124DF", Dark: "#4A1BA8"})

	fileNameStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFDF5"}).
		Background(lipgloss.AdaptiveColor{Light: "#6124DF", Dark: "#4A1BA8"}).
		Faint(true)

	statusBarStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#FFFDF5"}).
		Background(lipgloss.AdaptiveColor{Light: "#E0E0E0", Dark: "#6124DF"}).
		MarginTop(1)

	statusKeyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFDF5"}).
		Background(lipgloss.AdaptiveColor{Light: "#6124DF", Dark: "#4A1BA8"}).
		Padding(0, 1).
		Bold(true)

	centerStyle := lipgloss.NewStyle().
		Inherit(statusBarStyle).
		Padding(0, 1)

	// Build left, center, and right parts
	leftText := labelStyle.Render(" Component file: ") + fileNameStyle.Render(" "+fileName+" ")
	rightText := statusKeyStyle.Render(fmt.Sprintf("%d%%", scrollPercent))

	// Calculate available space for spacing
	width := m.viewport.Width
	if width <= 0 {
		width = 80 // fallback width
	}

	leftWidth := lipgloss.Width(leftText)
	rightWidth := lipgloss.Width(rightText)
	spacerWidth := width - leftWidth - rightWidth

	if spacerWidth < 0 {
		spacerWidth = 0
	}

	spacer := centerStyle.Width(spacerWidth).Render("")

	bar := lipgloss.JoinHorizontal(lipgloss.Top, leftText, spacer, rightText)

	return statusBarStyle.Width(width).Render(bar)
}

// viewComponentDetailScreen renders the component detail screen
func (m Model) viewComponentDetailScreen() string {
	if !m.ready {
		return "\n  Initializing..."
	}

	var sb strings.Builder

	sb.WriteString(tui.WelcomeStyle.Render("Component Details"))
	sb.WriteString("\n\n")
	sb.WriteString(m.viewport.View())
	sb.WriteString("\n\n")

	// Help text with subtle styling
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#666666", Dark: "#999999"}).
		Padding(0, 1)

	sb.WriteString(helpStyle.Render(m.help.View(m.keys)))
	sb.WriteString("\n")
	sb.WriteString(m.renderStatusBar())

	return sb.String()
}
