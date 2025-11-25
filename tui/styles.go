// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package tui

import "github.com/charmbracelet/lipgloss"

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
	StatusBarStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(DrPurpleLight).
			Foreground(DrPurpleLight).
			Padding(0, 1)
)
