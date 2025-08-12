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

	tea "github.com/charmbracelet/bubbletea"
)

type variable struct {
	name   string
	value  string
	secret bool
}

type Model struct {
	DotenvFile string
	variables  []variable
	err        error
	SuccessCmd tea.Cmd
}

type errMsg struct{ error } //nolint: errname

type (
	// getVariablesMsg struct{}
	startedMsg struct{ variables []variable }
)

func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		variables := []variable{
			{
				name:   "foo",
				value:  "bar",
				secret: false,
			},
		}

		return startedMsg{variables}
	}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "enter":
			return m, m.SuccessCmd
		}
	case startedMsg:
		m.variables = msg.variables
		return m, nil

	case errMsg:
		m.err = msg
		return m, nil
	}

	return m, nil
}

func (m Model) View() string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "Variables found in %s:\n\n", m.DotenvFile)

	for _, v := range m.variables {
		if v.secret {
			fmt.Fprintf(&sb, "%s: ***\n", v.name)
		} else {
			fmt.Fprintf(&sb, "%s: %s\n", v.name, v.value)
		}
	}

	return sb.String()
}
