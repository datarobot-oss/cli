// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package tui

import (
	_ "embed"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

//go:embed DR-ASCII.txt
var logoContent string

// View states
const (
	ViewWelcome = iota
	ViewLogin
)

// UI Constants
const (
	AppName      = "DataRobot CLI"
	QuitHelpText = "Press q or Ctrl+C to quit"
)

// Color scheme
const (
	drPurple = lipgloss.Color("#7770F9")
	drRed    = lipgloss.Color("#9A3131")
)

// Style definitions
var (
	baseTextStyle = lipgloss.NewStyle().Foreground(drPurple)
	welcomeStyle  = baseTextStyle.Bold(true)
	logoStyle     = baseTextStyle
	errorStyle    = lipgloss.NewStyle().Foreground(drRed).Bold(true)
)

type model struct {
	currentView        int
	logoDisplayContent string
}

func initialModel() model {
	m := model{
		currentView: ViewWelcome,
	}

	// Process embedded logo
	logoLines := strings.Split(strings.TrimSpace(logoContent), "\n")
	m.logoDisplayContent = logoStyle.Render(strings.Join(logoLines, "\n"))

	return m
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m model) View() string {
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
		sb.WriteString(errorStyle.Render("Unknown view"))
	}

	// Always render footer
	sb.WriteString("\n\n")
	sb.WriteString(m.renderFooter())

	return sb.String()
}

func (m model) renderHeader() string {
	return m.logoDisplayContent
}

func (m model) renderWelcomeView() string {
	var sb strings.Builder

	welcome := welcomeStyle.Render("Welcome to " + AppName)
	sb.WriteString(welcome)
	sb.WriteString("\n\n")

	return sb.String()
}

func (m model) renderFooter() string {
	return baseTextStyle.Render(QuitHelpText)
}

func Start() error {
	p := tea.NewProgram(initialModel())
	_, err := p.Run()

	return err
}
