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

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

type screens int

const (
	listScreen = screens(iota)
	editorScreen
)

type variable struct {
	name   string
	value  string
	secret bool
	auto   bool
}

type Model struct {
	screen         screens
	DotenvFile     string
	DotenvTemplate string
	variables      []variable
	err            error
	textarea       textarea.Model
	contents       string
	width          int
	height         int
	SuccessCmd     tea.Cmd
}

type (
	errMsg struct{ err error }

	dotenvFileUpdatedMsg struct {
		variables      []variable
		contents       string
		dotenvTemplate string
	}
)

func (m Model) saveEnvFile() tea.Cmd {
	return func() tea.Msg {
		variables, contents, dotenvTemplate, err := writeDotenvFile(m.DotenvFile)
		if err != nil {
			return errMsg{err}
		}

		return dotenvFileUpdatedMsg{variables, contents, dotenvTemplate}
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.saveEnvFile(), tea.WindowSize())
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		if m.screen == editorScreen {
			m.textarea.SetWidth(m.width - 1)
			m.textarea.SetHeight(m.height - 12)
		}

		return m, nil
	}

	switch m.screen {
	case listScreen:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch keypress := msg.String(); keypress {
			case "enter":
				return m, m.SuccessCmd
			case "e":
				m.screen = editorScreen
				ta := textarea.New()
				ta.SetWidth(m.width - 1)
				ta.SetHeight(m.height - 12)
				ta.SetValue(m.contents)
				ta.CursorStart()
				cmd := ta.Focus()
				m.textarea = ta

				return m, tea.Batch(cmd, func() tea.Msg {
					return tea.KeyMsg{
						Type:  tea.KeyRunes,
						Runes: []rune("ctrl+home"),
					}
				})
			}
		case dotenvFileUpdatedMsg:
			m.variables = msg.variables
			m.contents = msg.contents
			m.DotenvTemplate = msg.dotenvTemplate
			return m, nil
		case errMsg:
			m.err = msg.err
			return m, nil
		}
	case editorScreen:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch keypress := msg.String(); keypress {
			case "esc":
				// m.saving = true
				return m, m.saveEnvFile()
			}
		case dotenvFileUpdatedMsg:
			m.screen = listScreen
			m.variables = msg.variables
			m.contents = msg.contents
			m.DotenvTemplate = msg.dotenvTemplate
			return m, nil
		}
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		m.contents = m.textarea.Value()

		return m, cmd
	}

	return m, nil
}

func (m Model) View() string {
	var sb strings.Builder

	switch m.screen {
	case listScreen:
		fmt.Fprintf(&sb, "Variables found in %s:\n\n", m.DotenvFile)

		for _, v := range m.variables {
			if v.secret {
				fmt.Fprintf(&sb, "%s: ***\n", v.name)
			} else {
				fmt.Fprintf(&sb, "%s: %s\n", v.name, v.value)
			}
		}
	case editorScreen:
		sb.WriteString(m.textarea.View())
	}

	return sb.String()
}
