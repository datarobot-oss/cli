// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package setup

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/internal/version"
	"github.com/datarobot/cli/tui"
)

type Model struct{}

func NewModel() Model {
	return Model{}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m Model) View() string {
	var sb strings.Builder

	// Render header with logo
	sb.WriteString(tui.Header())
	sb.WriteString("\n\n")

	// Render welcome content
	welcome := tui.WelcomeStyle.Render("Welcome to " + version.AppName)
	sb.WriteString(welcome)
	sb.WriteString("\n\n")

	sb.WriteString(tui.BaseTextStyle.Render("This wizard will help you set up a new DataRobot application template."))
	sb.WriteString("\n\n")

	// Render footer with quit instructions
	sb.WriteString(tui.Footer())

	return sb.String()
}
