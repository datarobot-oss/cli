// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

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
