// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package start

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/internal/repo"
	"github.com/datarobot/cli/internal/tools"
	"github.com/datarobot/cli/tui"
)

// step represents a single step in the quickstart process
type step struct {
	// description is a brief summary of the step
	description string
	// fn is the function that performs the step's Update action
	fn func() tea.Msg
}

type ModelWithSteps interface {
	currentStep() step
	nextStep() step     // do i really need this
	previousStep() step // do i really need this
}

type Model struct {
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

func NewStartModel() Model {
	return Model{
		steps: []step{
			{description: "Starting application quickstart process...", fn: startQuickstart},
			{description: "Checking template prerequisites...", fn: checkPrerequisites},
			{description: "Validating environment...", fn: validateEnvironment},
			{description: "Executing quickstart script...", fn: executeQuickstart},
		},
		current:  0,
		done:     false,
		quitting: false,
		err:      nil,
	}
}

func (m Model) Init() tea.Cmd {
	return m.executeCurrentStep()
}

func (m Model) executeCurrentStep() tea.Cmd {
	if m.current >= len(m.steps) {
		return nil
	}

	currentStep := m.currentStep()

	return func() tea.Msg {
		return currentStep.fn()
	}
}

func (m Model) currentStep() step {
	return m.steps[m.current]
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}

	case stepCompleteMsg:
		// See if there's a next step, and move to it
		if m.current < len(m.steps)-1 {
			m.current++
			return m, m.executeCurrentStep()
		}

		// No more steps, we're done
		m.done = true

		return m, tea.Quit

	case stepErrorMsg:
		m.err = msg.err
		m.quitting = true

		return m, tea.Quit
	}

	return m, nil
}

func (m Model) View() string {
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

	// If there's an error, display it at the end after showing the steps
	if m.err != nil {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)

		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("%s %s\n", errorStyle.Render("Error:"), m.err.Error()))
	} else if !m.done && !m.quitting {
		sb.WriteString("\n")
		sb.WriteString(tui.Footer())
	}

	sb.WriteString("\n")

	return sb.String()
}

// Step function stubs

func startQuickstart() tea.Msg {
	// - Set up initial state
	// - Display welcome message
	// - Prepare for subsequent steps
	return stepCompleteMsg{}
}

func checkPrerequisites() tea.Msg {
	// TODO: Implement prerequisites checking logic
	// - Check we are in a DR repository
	// - Check for required tools (git, docker, etc.)
	// - Verify template configuration
	// - Validate directory structure
	// Return stepErrorMsg{err} if prerequisites are not met
	if !repo.IsInRepo() {
		return stepErrorMsg{err: errors.New("not inside a DataRobot repository")}
	}

	if err := tools.CheckPrerequisites(); err != nil {
		return stepErrorMsg{err: err}
	}

	return stepCompleteMsg{}
}

func validateEnvironment() tea.Msg {
	// TODO: Implement environment validation logic
	// - Check environment variables
	// - Validate system requirements
	// Return stepErrorMsg{err} if validation fails
	time.Sleep(100 * time.Millisecond) // Simulate work

	// TODO invoke logic in internal.envvalidator

	return stepCompleteMsg{}
}

func executeQuickstart() tea.Msg {
	// If we are in a DataRobot repository, look for a quickstart script in the standard location
	// of .datarobot/cli/bin. If we find it, print its path and execute it after user confirmation.
	// If we do not find it, tell the user that we couldn't find one and suggest that they instead
	// run `dr template setup`.
	quickstartScript, err := findQuickstartScript()
	if err != nil {
		return stepErrorMsg{err: err}
	}

	log.Println("Found quickstart script at:", quickstartScript)

	// TODO Can we make this a reusable step?
	fmt.Print("Do you want to execute this script? (y/n): ")
	var response string
	fmt.Scanln(&response)
	if response != "y" {
		log.Println("Aborting quickstart script execution.")
		return stepCompleteMsg{}
	}

	// Execute the script directly (it should have a shebang or be executable)
	cmd := exec.Command(quickstartScript)

	// Set up command to inherit stdin/stdout/stderr for interactive execution
	// TODO This needs to interrupt the TUI properly so that users
	// can actually see the output and interact with the script
	// and only after this has completed do we return to the TUI
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Execute the script
	// TODO After user confirmation
	log.Println("Executing quickstart script...")

	if err := cmd.Run(); err != nil {
		return stepErrorMsg{err: fmt.Errorf("failed to execute quickstart script: %w", err)}
	}

	log.Println("Quickstart script completed successfully")

	return stepCompleteMsg{}
}

func findQuickstartScript() (string, error) {
	// Look for any executable file named quickstart* in the configured path relative to CWD
	executablePath := repo.QuickstartScriptPath

	// Find files matching quickstart*
	matches, err := filepath.Glob(filepath.Join(executablePath, "quickstart*"))
	if err != nil {
		return "", fmt.Errorf("failed to search for quickstart script: %w", err)
	}

	// Find the first executable file
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			continue
		}

		// Check if it's a regular file and executable
		if !info.IsDir() && info.Mode()&0o111 != 0 {
			return match, nil
		}
	}

	absExecutablePath, _ := filepath.Abs(executablePath)

	return "", fmt.Errorf("no executable quickstart script found in %s", absExecutablePath)
}
