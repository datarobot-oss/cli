// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package start

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/tui"
)

// step represents a single step in the quickstart process
type step struct {
	description string
	fn          func() tea.Msg
}

type ModelWithSteps interface {
	CurrentStep() step
	NextStep() step // do i really need this
	PreviousStep() step // do i really need this
}

// StartModel defines the model for the start command's TUI
type StartModel struct {
	steps    []step
	current  int
	done     bool
	quitting bool
	err      error
}

type stepCompleteMsg struct{}

type stepErrorMsg struct {
	err error
}

var (
	checkMark = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).SetString("✓")
	arrow     = lipgloss.NewStyle().Foreground(tui.DrPurple).SetString("→")
	dimStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	boldStyle = lipgloss.NewStyle().Bold(true)
)

func NewStartModel() StartModel {
	return StartModel{
		steps: []step{
			{description: "Starting application quickstart process...", fn: startQuickstart},
			{description: "Checking template prerequisites...", fn: checkPrerequisites},
			{description: "Validating environment...", fn: validateEnvironment},
			{description: "Executing quickstart script...", fn: executeQuickstart},
			{description: "Application quickstart process completed.", fn: completeQuickstart},
		},
		current:  0,
		done:     false,
		quitting: false,
		err:      nil,
	}
}

func (m StartModel) Init() tea.Cmd {
	return m.executeCurrentStep()
}

func (m StartModel) executeCurrentStep() tea.Cmd {
	if m.current >= len(m.steps) {
		return nil
	}

	currentStep := m.CurrentStep()

	return func() tea.Msg {
		return currentStep.fn()
	}
}

func (m StartModel) NextStep() step {
	if m.current+1 < len(m.steps) {
		return m.steps[m.current+1]
	}
}

func (m StartModel) PreviousStep() step {
	if m.current-1 >= 0 {
		return m.steps[m.current-1]
	}
}

func (m StartModel) CurrentStep() step {
	return m.steps[m.current]
}

func (m StartModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}

	case stepCompleteMsg:
		if m.current < len(m.steps)-1 {
			m.current++
			return m, m.executeCurrentStep()
		}

		m.done = true
		return m, tea.Quit

	case stepErrorMsg:
		m.err = msg.err
		m.quitting = true

		return m, tea.Quit
	}

	return m, nil
}

func (m StartModel) View() string {
	if m.quitting {
		if m.err != nil {
			errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
			return fmt.Sprintf("\n%s %s\n\n", errorStyle.Render("Error:"), m.err.Error())
		}

		return ""
	}

	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString(boldStyle.Render("DataRobot Quickstart"))
	sb.WriteString("\n\n")

	for i, step := range m.steps {
		if i < m.current {
			sb.WriteString(fmt.Sprintf("  %s %s\n", checkMark, dimStyle.Render(step.description)))
		} else if i == m.current {
			sb.WriteString(fmt.Sprintf("  %s %s\n", arrow, step.description))
		} else {
			sb.WriteString(fmt.Sprintf("    %s\n", dimStyle.Render(step.description)))
		}
	}

	if !m.done {
		sb.WriteString("\n")
		sb.WriteString(tui.Footer())
	}

	sb.WriteString("\n")

	return sb.String()
}

// Step function stubs

func startQuickstart() tea.Msg {
	// TODO: Implement quickstart initialization logic
	// - Set up initial state
	// - Display welcome message
	// - Prepare for subsequent steps
	time.Sleep(500 * time.Millisecond) // Simulate work

	return stepCompleteMsg{}
}

func checkPrerequisites() tea.Msg {
	// TODO: Implement prerequisites checking logic
	// - Check for required tools (git, docker, etc.)
	// - Verify template configuration
	// - Validate directory structure
	// Return stepErrorMsg{err} if prerequisites are not met
	time.Sleep(500 * time.Millisecond) // Simulate work

	return stepCompleteMsg{}
}

func validateEnvironment() tea.Msg {
	// TODO: Implement environment validation logic
	// - Check environment variables
	// - Verify credentials if needed
	// - Validate system requirements
	// Return stepErrorMsg{err} if validation fails
	time.Sleep(500 * time.Millisecond) // Simulate work

	return stepCompleteMsg{}
}

func executeQuickstart() tea.Msg {
	// TODO: Implement quickstart script execution logic
	// - Look for quickstart.py or quickstart.sh in .datarobot/cli/bin
	// - Execute the script with appropriate parameters
	// - Capture and handle output
	// Return stepErrorMsg{err} if execution fails
	time.Sleep(500 * time.Millisecond) // Simulate work

	return stepCompleteMsg{}
}

func completeQuickstart() tea.Msg {
	// find the quickstart script at `.datarobot/cli/bin/quickstart.py` or `quickstart.sh`
	// and execute it, passing any necessary parameters.
	quickstartScript := filepath.Join(".datarobot", "cli", "bin", "quickstart.py")
	if _, err := os.Stat(quickstartScript); os.IsNotExist(err) {
		quickstartScript = filepath.Join(".datarobot", "cli", "bin", "quickstart.sh")
		

	// TODO: Implement completion logic
	// - Display success message
	// - Show next steps or instructions
	// - Clean up temporary resources
	time.Sleep(500 * time.Millisecond) // Simulate work

	return stepCompleteMsg{}
}
