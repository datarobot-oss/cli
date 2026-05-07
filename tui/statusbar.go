// Copyright 2026 DataRobot, Inc. and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

	bar := lipgloss.JoinHorizontal(
		lipgloss.Top,
		statusKey,
		statusMsg,
	)

	return statusBarStyle.Width(width).Render(bar)
}
