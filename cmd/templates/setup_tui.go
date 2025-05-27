// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package templates

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/datarobot/cli/internal/version"
	"github.com/datarobot/cli/tui"
)

// TemplateSetupModel handles the template setup workflow
type TemplateSetupModel struct {
	tui.BaseModel
}

// NewTemplateSetupModel creates a new template setup model
func NewTemplateSetupModel() TemplateSetupModel {
	return TemplateSetupModel{
		BaseModel: tui.NewBaseModel(),
	}
}

func (m TemplateSetupModel) Init() tea.Cmd {
	return nil
}

func (m TemplateSetupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle common keys (quit, etc.)
		if cmd := tui.HandleCommonKeys(msg); cmd != nil {
			return m, cmd
		}
	}

	return m, nil
}

func (m TemplateSetupModel) View() string {
	var sb strings.Builder

	// Render header with logo
	sb.WriteString(m.RenderHeader())
	sb.WriteString("\n\n")

	// Render welcome content
	welcome := tui.WelcomeStyle.Render("Welcome to " + version.AppName)
	sb.WriteString(welcome)
	sb.WriteString("\n\n")

	sb.WriteString(tui.BaseTextStyle.Render("This wizard will help you set up a new DataRobot application template."))
	sb.WriteString("\n\n")

	// Render footer with quit instructions
	sb.WriteString(m.RenderFooter())

	return sb.String()
}

// StartTemplateSetup starts the template setup TUI
func StartTemplateSetup() error {
	p := tea.NewProgram(NewTemplateSetupModel())
	_, err := p.Run()

	return err
}
