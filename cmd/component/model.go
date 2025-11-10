// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package component

import (
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/datarobot/cli/internal/copier"
	"github.com/datarobot/cli/tui"
)

type (
	errMsg              struct{ err error }
	screens             int
	componentsLoadedMsg struct {
		components []copier.Component
	}
)

const (
	// TODO: Add more keys as we start to use them in upcoming work
	keyExit  = "esc"
	keyEnter = "enter"
)

const (
	listScreen = screens(iota)
	updateScreen
)

type Model struct {
	err            error
	infoMessage    string
	screen         screens
	initialScreen  screens
	components     []copier.Component
	updateFileName string
}

func NewComponentModel(initialScreen screens) Model {
	return Model{
		screen:        initialScreen,
		initialScreen: initialScreen,
	}
}

func NewUpdateComponentModel(currentScreen screens, updateFileName string) Model {
	return Model{
		screen:         currentScreen,
		initialScreen:  updateScreen, // Always set initial screen to update
		updateFileName: updateFileName,
	}
}

func (m Model) Init() tea.Cmd {
	switch m.initialScreen {
	case listScreen:
		return tea.Batch(m.loadComponents(), tea.WindowSize())
	case updateScreen:
		if m.updateFileName == "" {
			return tea.Batch(m.loadComponents(), tea.WindowSize())
		}

		return tea.WindowSize()
	}

	return tea.WindowSize()
}

func (m Model) loadComponents() tea.Cmd {
	return func() tea.Msg {
		answers, err := copier.AnswersFromPath(".")
		if err != nil {
			return errMsg{err}
		}

		components, err := copier.ComponentsFromAnswers(answers)
		if err != nil {
			return errMsg{err}
		}

		// If we've found zero components return error message that is handled by UI
		if len(components) == 0 {
			return errMsg{errors.New("No components were found.")} //nolint:revive,staticcheck
		}

		return componentsLoadedMsg{components}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:cyclop
	switch msg := msg.(type) {
	case componentsLoadedMsg:
		m.components = msg.components
		if m.initialScreen == updateScreen && m.updateFileName == "" {
			m.infoMessage = "Please use an 'answer file' value with 'dr component update <answer_file>'."
		}
	}

	switch m.screen {
	case listScreen:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch keypress := msg.String(); keypress {
			// For now exit on any keypress
			default:
				return m, tea.Quit
			}
		case errMsg:
			m.err = msg.err
			return m, nil
		}
	case updateScreen:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch keypress := msg.String(); keypress {
			case keyExit:
				return m, tea.Quit
			case keyEnter:
				if m.updateFileName != "" {
					// TODO: run copier.ExecUpdate using tea.ExecProcess here
					err := runUpdate(m.updateFileName)
					if err != nil {
						m.err = err
					}

					return m, nil
				}
			}
		case errMsg:
			m.err = msg.err
			return m, nil
		}
	}

	return m, nil
}

func (m Model) View() string {
	var sb strings.Builder

	switch m.screen {
	case listScreen:
		sb.WriteString(m.viewListScreen())
	case updateScreen:
		sb.WriteString(m.viewUpdateScreen())
	}

	return sb.String()
}

func (m Model) viewListScreen() string {
	var sb strings.Builder

	// Display error message
	if m.err != nil {
		sb.WriteString(fmt.Sprintf("%s %s\n", tui.ErrorStyle.Render("Error:"), m.err.Error()))
		sb.WriteString("\n")
		sb.WriteString(tui.DimStyle.Render("Press any key to exit"))
		sb.WriteString("\n")

		return sb.String()
	}

	sb.WriteString(tui.WelcomeStyle.Render("Available Components for Recipe Agent Template:"))
	sb.WriteString("\n\n")

	// Display status message
	if m.infoMessage != "" {
		sb.WriteString(fmt.Sprintf("%s %s\n", tui.InfoStyle.Render("Info:"), m.infoMessage))
		sb.WriteString("\n")
	}

	// TODO: Add this line once it makes sense we can select etc.
	// sb.WriteString(tui.BaseTextStyle.Render("Core Components (Select at least 1):"))
	// sb.WriteString("\n\n")

	// TODO: Remove this table view once we make updates here to be able to select/interact with components
	t := table.New()
	t.Headers(tui.BaseTextStyle.Render("Answers file"), tui.BaseTextStyle.Render("Repository"))

	for _, c := range m.components {
		t.Row(c.FileName, c.SrcPath)
		// TODO: Figure out if these are the right values to use in display:
		// c.TemplateName and c.TemplateDescription
	}

	// Write table as string (for now)
	sb.WriteString(t.String())
	sb.WriteString("\n")

	// TODO: Add actual other keys that a user may interact with
	sb.WriteString("\n")
	sb.WriteString(tui.BaseTextStyle.Render("Press esc to exit."))

	return sb.String()
}

func (m Model) viewUpdateScreen() string {
	var sb strings.Builder

	// Display error message
	if m.err != nil {
		sb.WriteString(fmt.Sprintf("%s %s\n", tui.ErrorStyle.Render("Error:"), m.err.Error()))
		sb.WriteString("\n")
		sb.WriteString(tui.DimStyle.Render("Press any key to exit"))
		sb.WriteString("\n")

		return sb.String()
	}

	sb.WriteString(tui.WelcomeStyle.Render("Component Update"))
	sb.WriteString("\n\n")
	sb.WriteString(tui.BaseTextStyle.Render("Update component " + m.updateFileName + " ?"))
	sb.WriteString("\n\n")
	sb.WriteString(tui.BaseTextStyle.Render("Press enter to run update."))
	sb.WriteString("\n\n")

	return sb.String()
}
