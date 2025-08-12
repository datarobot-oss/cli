// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package clone

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/internal/drapi"
)

type Model struct {
	template   drapi.Template
	input      textinput.Model
	cloning    bool
	finished   bool
	out        string
	Dir        string
	SuccessCmd tea.Cmd
}

type focusInputMsg struct{}

type startCloningMsg struct{}

func startCloning() tea.Msg {
	return startCloningMsg{}
}

func (m Model) Init() tea.Cmd {
	return func() tea.Msg { return focusInputMsg{} }
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "enter":
			m.input.Blur()
			m.cloning = true

			return m, startCloning
		}
	case focusInputMsg:
		focusCmd := m.input.Focus()
		return m, focusCmd
	case startCloningMsg:
		out, err := gitClone(m.template.Repository.URL, m.input.Value())
		if err != nil {
			m.out = err.Error()
			return m, tea.Quit
		}

		m.out = out
		m.cloning = false
		m.finished = true
		m.Dir = m.input.Value()

		return m, m.SuccessCmd
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)

	return m, cmd
}

func (m Model) View() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Template %s\n", m.template.Name))

	if m.cloning {
		sb.WriteString("Cloning into " + m.input.Value() + "...")
	} else if m.finished {
		sb.WriteString(m.out + "\nFinished cloning into " + m.input.Value() + ".\n")
	} else {
		sb.WriteString("Enter destination directory\n" + m.input.View())
	}

	return sb.String()
}

func (m *Model) SetTemplate(template drapi.Template) {
	m.input = textinput.New()
	m.input.SetValue(template.DefaultDir())
	m.template = template
}
