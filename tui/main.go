package tui

import (
	_ "embed"
	"fmt"
	"os"
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
	AppName      = "Datarobot CLI"
	QuitHelpText = "Press q or Ctrl+C to quit"
)

// Color scheme
const drPurple = lipgloss.Color("#7770F9")
const drRed = lipgloss.Color("#9A3131")

// Style definitions
var (
	baseTextStyle = lipgloss.NewStyle().Foreground(drPurple)
	welcomeStyle  = baseTextStyle.Bold(true)
	logoStyle     = baseTextStyle
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(drRed)).Bold(true)
)

type model struct {
	currentView        int
	logoDisplayContent string
}

func initialModel() model {
	m := model{
		currentView: ViewWelcome,
	}

	// Process embedded logo with error handling
	if logoContent == "" {
		m.logoDisplayContent = errorStyle.Render("⚠ Logo not available")
	} else {
		logoLines := strings.Split(strings.TrimSpace(logoContent), "\n")
		m.logoDisplayContent = logoStyle.Render(strings.Join(logoLines, "\n"))
	}

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

	welcome := welcomeStyle.Render(fmt.Sprintf("Welcome to %s", AppName))
	sb.WriteString(welcome)
	sb.WriteString("\n\n")

	return sb.String()
}

func (m model) renderFooter() string {
	return baseTextStyle.Render(QuitHelpText)
}

func Start() {
	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Printf("An error occurred: %v", err)
		os.Exit(1)
	}
}
