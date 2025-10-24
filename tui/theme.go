// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package tui

import "github.com/charmbracelet/lipgloss"

// DataRobot brand colors, utilizing the Design System palette
const (
	DrPurple      = lipgloss.Color("#7770F9") // purple-60
	DrPurpleLight = lipgloss.Color("#B4B0FF") // purple-40
	DrIndigo      = lipgloss.Color("#5C41FF") // indigo-70
	DrRed         = lipgloss.Color("#9A3131") // red-80
	DrGreen       = lipgloss.Color("#81FBA5") // green-60
	DrYellow      = lipgloss.Color("#F6EB61") // yellow-60
	DrBlack       = lipgloss.Color("#0B0B0B") // black-90
)

// Common style definitions using DataRobot branding
var (
	BaseTextStyle = lipgloss.NewStyle().Foreground(DrPurple)
	ErrorStyle    = lipgloss.NewStyle().Foreground(DrRed).Bold(true)

	// Specific UI styles
	LogoStyle    = BaseTextStyle
	WelcomeStyle = BaseTextStyle.Bold(true)
)

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
