// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package dotenv

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/tui"
)

type promptModel struct {
	currentPrompt prompt
	input         textinput.Model
}

func newPromptModel(p prompt) promptModel {
	return promptModel{
		currentPrompt: p,
		input:         textinput.New(),
	}
}

func (pm promptModel) Update(msg tea.Msg) (promptModel, tea.Cmd) {
	var cmd tea.Cmd
	pm.input, cmd = pm.input.Update(msg)

	return pm, cmd
}

func (pm promptModel) View() string {
	var sb strings.Builder

	sb.WriteString("\n\n")
	sb.WriteString(tui.BaseTextStyle.Render(pm.currentPrompt.helpMsg))
	sb.WriteString("\n")
	sb.WriteString(pm.input.View())
	sb.WriteString("\n")

	if pm.currentPrompt.rawPrompt.Default != "" {
		sb.WriteString(tui.BaseTextStyle.Render(fmt.Sprintf("Default: %s", pm.currentPrompt.rawPrompt.Default)))
		sb.WriteString("\n")
	}

	return sb.String()
}
