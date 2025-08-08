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
	DrPurple = lipgloss.Color("#7770F9") // purple-60
	DrRed    = lipgloss.Color("#9A3131") // red-80
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
	return LogoStyle.Render(Banner)
}

// Footer renders the common footer with quit instructions
func Footer() string {
	return BaseTextStyle.Render("Press q or Ctrl+C to quit")
}
