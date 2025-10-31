// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package tui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TUI configuration constants
const (
	// DebugLogFile is the filename for TUI debug logs
	DebugLogFile = "dr-tui-debug.log"
)

// SetupDebugLogging configures debug logging for the TUI if debug mode is enabled.
// It should be called at the start of TUI programs when debug logging is needed.
// Returns a cleanup function that should be deferred to close the log file.
func SetupDebugLogging() (cleanup func(), err error) {
	f, err := tea.LogToFile(DebugLogFile, "debug")
	if err != nil {
		fmt.Println("fatal:", err)
		os.Exit(1)
	}

	return func() { f.Close() }, nil
}

// Common style definitions using DataRobot branding
var (
	BaseTextStyle = lipgloss.NewStyle().Foreground(GetAdaptiveColor(DrPurple, DrPurpleDark))
	ErrorStyle    = lipgloss.NewStyle().Foreground(DrRed).Bold(true)
	InfoStyle     = lipgloss.NewStyle().Foreground(GetAdaptiveColor(DrPurpleLight, DrPurpleDarkLight)).Bold(true)
	DimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

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
			BorderForeground(GetAdaptiveColor(DrPurpleLight, DrPurpleDarkLight)).
			Padding(0, 1)
	StatusBarStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(DrPurpleLight).
			Foreground(DrPurpleLight).
			Padding(0, 1)
)
