// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package setup

import (
	"log"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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
)

type Model struct {
	screen screens
	login  LoginModel
	list   list.Model
	clone  clone.Model
}

type successMsg string

func successCmd(apiKey string) tea.Cmd {
	return func() tea.Msg {
		return successMsg(apiKey)
	}
}

type getTemplatesMsg struct{}

func getTemplates() tea.Msg {
	return getTemplatesMsg{}
}

func NewModel() Model {
	return Model{
		screen: welcomeScreen,
		login: LoginModel{
			apiKeyChan: make(chan string, 1),
			successCmd: successCmd,
		},
	}
}

func (m Model) Init() tea.Cmd {
	return getTemplates
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	case getTemplatesMsg:
		templateList, err := drapi.GetTemplates()
		if err != nil {
			m.screen = loginScreen
			if m.login.apiKeyChan != nil {
				cmd := m.login.Init()
				return m, cmd
			}

			return m, nil
		}

		m.list = list.NewModel(templateList.Templates)
		m.screen = listScreen
		return m, m.list.Init()
	case successMsg:
		log.Printf("successMsg\n")
		m.screen = listScreen
		return m, nil
	}

	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch m.screen {
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
		sb.WriteString(m.login.View())
	case listScreen:
		sb.WriteString(m.list.View())
	case cloneScreen:
		sb.WriteString(m.clone.View())
	}

	return sb.String()
}
