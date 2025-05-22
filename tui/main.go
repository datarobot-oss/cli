package tui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const drPurple = lipgloss.Color("#7770F9")

var baseTextStyle = lipgloss.NewStyle().Foreground(drPurple)
var welcomeStyle = baseTextStyle.Bold(true)
var logoStyle = baseTextStyle

type model struct {
	logoDisplayContent string
}

func initialModel() model {
	m := model{}
	logoFilePath := "tui/DR-ASCII.txt"

	logoBytes, err := os.ReadFile(logoFilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading logo file %s: %v\n", logoFilePath, err)
		m.logoDisplayContent = logoStyle.Render("Error: Could not load logo.")
	} else {
		logoContent := string(logoBytes)
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
	welcomeMessage := welcomeStyle.Render("Welcome to Datarobot CLI")
	quitMessage := baseTextStyle.Render("Press q to quit.")

	var sb strings.Builder
	sb.WriteString(m.logoDisplayContent)
	sb.WriteString("\n\n")
	sb.WriteString(welcomeMessage)
	sb.WriteString("\n\n")
	sb.WriteString(quitMessage)
	sb.WriteString("\n")
	return sb.String()
}

func Start() {
	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
