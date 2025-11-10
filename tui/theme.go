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
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

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

func SetAnsiForegroundColor(hexColor lipgloss.Color) string {
	hexString := strings.TrimPrefix(string(hexColor), "#")

	rVal, _ := strconv.ParseUint(hexString[0:2], 16, 8)
	gVal, _ := strconv.ParseUint(hexString[2:4], 16, 8)
	bVal, _ := strconv.ParseUint(hexString[4:6], 16, 8)

	return fmt.Sprintf("\033[38;2;%d;%d;%dm", rVal, gVal, bVal)
}

func ResetForegroundColor() string {
	return "\033[39m"
}

// Common style definitions using DataRobot branding
var (
	BaseTextStyle = lipgloss.NewStyle().Foreground(DrPurple)
	ErrorStyle    = lipgloss.NewStyle().Foreground(DrRed).Bold(true)
	InfoStyle     = lipgloss.NewStyle().Foreground(DrPurpleLight).Bold(true)
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
) // Header renders the common header with DataRobot logo
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
