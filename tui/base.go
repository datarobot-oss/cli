// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// BaseModel provides common functionality for all TUI models
type BaseModel struct {
	LogoDisplayContent string
}

// NewBaseModel creates a new base model with common setup
func NewBaseModel() BaseModel {
	m := BaseModel{}

	// Process banner for logo display
	logoLines := strings.Split(strings.TrimSpace(Banner), "\n")
	m.LogoDisplayContent = LogoStyle.Render(strings.Join(logoLines, "\n"))

	return m
}

// RenderHeader renders the common header with DataRobot logo
func (m BaseModel) RenderHeader() string {
	return m.LogoDisplayContent
}

// RenderFooter renders the common footer with quit instructions
func (m BaseModel) RenderFooter() string {
	return BaseTextStyle.Render("Press q or Ctrl+C to quit")
}

// HandleCommonKeys handles common key presses (quit, etc.)
func HandleCommonKeys(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c", "q":
		return tea.Quit
	}

	return nil
}
