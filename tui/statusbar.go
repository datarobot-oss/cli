// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package tui

import (
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
)

var (
	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#FFFDF5"}).
			Background(lipgloss.AdaptiveColor{Light: "#E0E0E0", Dark: "#6124DF"}).
			MarginTop(1)

	statusKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFDF5"}).
			Background(lipgloss.AdaptiveColor{Light: "#6124DF", Dark: "#4A1BA8"}).
			Padding(0, 1).
			Bold(true)

	statusMessageStyle = lipgloss.NewStyle().
				Inherit(statusBarStyle).
				Padding(0, 1)
)

// RenderStatusBar creates a status bar with optional spinner and message.
// Based on lipgloss layout example.
func RenderStatusBar(width int, s spinner.Model, message string, isLoading bool) string {
	w := lipgloss.Width

	// Status indicator
	var statusKey string
	if isLoading {
		statusKey = statusKeyStyle.Render(s.View() + " ")
	} else {
		// Idle indicator
		statusKey = statusKeyStyle.Render("✓")
	}

	// Spinner animation (only when loading)
	// Message with optional spinner
	statusMsg := statusMessageStyle.
		Width(width - w(statusKey) - 2).
		Render(message)

	bar := lipgloss.JoinHorizontal(lipgloss.Top,
		statusKey,
		statusMsg,
	)

	return statusBarStyle.Width(width).Render(bar)
}
