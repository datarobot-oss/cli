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

	"github.com/datarobot/cli/internal/version"
)

// View states
const (
	ViewWelcome = iota
	ViewLogin
)

// UI Constants
const (
	QuitHelpText = "Press q or Ctrl+C to quit"
)

var (
	logoStyle    = BaseTextStyle
	welcomeStyle = BaseTextStyle.Bold(true)
)

type templateInitModel struct {
	currentView        int
	logoDisplayContent string
}

func initialModel() templateInitModel {
	m := templateInitModel{
		currentView: ViewWelcome,
	}

	// Process banner
	logoLines := strings.Split(strings.TrimSpace(Banner), "\n")
	m.logoDisplayContent = logoStyle.Render(strings.Join(logoLines, "\n"))

	return m
}

func (m templateInitModel) Init() tea.Cmd {
	return nil
}

func (m templateInitModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m templateInitModel) View() string {
	var sb strings.Builder

	// Always render header with logo
	sb.WriteString(m.renderHeader())
	sb.WriteString("\n\n")

	// Render current view content
	switch m.currentView {
	case ViewWelcome:
		sb.WriteString(m.renderWelcomeView())
	// Future views:
	// case ViewLogin:
	//     sb.WriteString(m.renderLoginView())
	default:
		sb.WriteString(ErrorStyle.Render("Unknown view"))
	}

	// Always render footer
	sb.WriteString("\n\n")
	sb.WriteString(m.renderFooter())

	return sb.String()
}

func (m templateInitModel) renderHeader() string {
	return m.logoDisplayContent
}

func (m templateInitModel) renderWelcomeView() string {
	var sb strings.Builder

	welcome := welcomeStyle.Render("Welcome to " + version.AppName)
	sb.WriteString(welcome)
	sb.WriteString("\n\n")

	return sb.String()
}

func (m templateInitModel) renderFooter() string {
	return BaseTextStyle.Render(QuitHelpText)
}

func Start() error {
	p := tea.NewProgram(initialModel())
	_, err := p.Run()

	return err
}
