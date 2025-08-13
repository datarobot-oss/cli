// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package setup

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/cmd/dotenv"
	"github.com/datarobot/cli/cmd/templates/clone"
	"github.com/datarobot/cli/cmd/templates/list"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/version"
	"github.com/datarobot/cli/tui"
)

type screens int

const (
	welcomeScreen = screens(iota)
	loginScreen
	listScreen
	cloneScreen
	dotenvScreen
	exitScreen
)

type Model struct {
	screen      screens
	template    drapi.Template
	exitMessage string

	login  LoginModel
	list   list.Model
	clone  clone.Model
	dotenv dotenv.Model
}

type (
	authSuccessMsg      struct{}
	templateSelectedMsg struct{}
	getTemplatesMsg     struct{}
	templateClonedMsg   struct{}
	dotenvUpdatedMsg    struct{}
	exitMsg             struct{}
)

func authSuccess() tea.Msg      { return authSuccessMsg{} }
func getTemplates() tea.Msg     { return getTemplatesMsg{} }
func templateSelected() tea.Msg { return templateSelectedMsg{} }
func templateCloned() tea.Msg   { return templateClonedMsg{} }
func dotenvUpdated() tea.Msg    { return dotenvUpdatedMsg{} }
func exit() tea.Msg             { return exitMsg{} }

func NewModel() Model {
	return Model{
		screen:   welcomeScreen,
		template: drapi.Template{},

		login: LoginModel{
			APIKeyChan: make(chan string, 1),
			SuccessCmd: authSuccess,
		},
		list: list.Model{
			SuccessCmd: templateSelected,
		},
		clone: clone.Model{
			SuccessCmd: templateCloned,
		},
		dotenv: dotenv.Model{
			SuccessCmd: dotenvUpdated,
		},
	}
}

func (m Model) Init() tea.Cmd {
	return getTemplates
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint: cyclop
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			if m.screen != cloneScreen {
				return m, tea.Quit
			}
		}
	case getTemplatesMsg:
		templateList, err := drapi.GetTemplates()
		if err != nil {
			m.screen = loginScreen
			if m.login.APIKeyChan != nil {
				cmd := m.login.Init()
				return m, cmd
			}

			return m, nil
		}

		m.list.SetTemplates(templateList.Templates)
		m.screen = listScreen

		return m, m.list.Init()
	case authSuccessMsg:
		m.screen = listScreen
		return m, getTemplates
	case templateSelectedMsg:
		m.template = m.list.Template
		m.clone.SetTemplate(m.template)
		m.screen = cloneScreen

		return m, m.clone.Init()
	case templateClonedMsg:
		m.screen = dotenvScreen
		m.dotenv.DotenvFile = filepath.Join(m.clone.Dir, ".env")

		return m, m.dotenv.Init()
	case dotenvUpdatedMsg:
		m.screen = exitScreen
		m.exitMessage = fmt.Sprintf("Template '%s' cloned and initialized in '%s' directory.\n\n",
			m.template.Name, m.clone.Dir,
		)

		return m, tea.Sequence(tea.ExitAltScreen, exit)
	case exitMsg:
		return m, tea.Quit
	}

	var cmd tea.Cmd

	var cmds []tea.Cmd

	switch m.screen {
	case welcomeScreen:
	case loginScreen:
		m.login, cmd = m.login.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case listScreen:
		m.list, cmd = m.list.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case cloneScreen:
		m.clone, cmd = m.clone.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case dotenvScreen:
		m.dotenv, cmd = m.dotenv.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case exitScreen:
	}

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	var sb strings.Builder

	// Render header with logo
	sb.WriteString(tui.Header())
	sb.WriteString("\n\n")

	switch m.screen {
	case welcomeScreen:
		// Render welcome content
		welcome := tui.WelcomeStyle.Render("Welcome to " + version.AppName)
		sb.WriteString(welcome)
		sb.WriteString("\n\n")

		sb.WriteString(tui.BaseTextStyle.Render("This wizard will help you set up a new DataRobot application template."))
		sb.WriteString("\n\n")

		// Render footer with quit instructions
		sb.WriteString(tui.Footer())
	case loginScreen:
		sb.WriteString(tui.BaseTextStyle.Render("This wizard will help you set up a new DataRobot application template."))
		sb.WriteString("\n\n")

		sb.WriteString(m.login.View())
	case listScreen:
		sb.WriteString(m.list.View())
	case cloneScreen:
		sb.WriteString(m.clone.View())
	case dotenvScreen:
		sb.WriteString(m.dotenv.View())
	case exitScreen:
		sb.WriteString(m.exitMessage)
	}

	return sb.String()
}
