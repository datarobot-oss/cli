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
	"runtime"
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
	steps       []step
	current     int
	done        bool
	quitting    bool
	err         error
	stepCompleteMessage         string // Optional message from the completed step
	quickstartScriptPath string // Path to the quickstart script to execute
	waiting              bool   // Whether to wait for user input before proceeding
}

type stepCompleteMsg struct {
	message string // Optional message to display to the user
	waiting bool   // Whether to wait for user input before proceeding
	done   bool   // Whether the quickstart process is complete
}

type stepErrorMsg struct {
	err error // Error encountered during step execution
}

// err messages used in the start command
const (
	errNotInRepo             = "not inside a DataRobot repository"
	errScriptExecutionFailed = "failed to execute quickstart script: %w"
	errScriptSearchFailed    = "failed to search for quickstart script: %w"
)

var (
	checkMark  = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).SetString("✓")
	arrow      = lipgloss.NewStyle().Foreground(tui.DrPurple).SetString("→")
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

func NewStartModel() Model {
	return Model{
		steps: []step{
			{description: "Starting application quickstart process...", fn: startQuickstart},
			{description: "Checking template prerequisites...", fn: checkPrerequisites},
			// TODO Implement validateEnvironment
			// {description: "Validating environment...", fn: validateEnvironment},
			{description: "Locating quickstart script...", fn: findQuickstart},
			{description: "Executing quickstart script...", fn: executeQuickstart},
		},
		current:  0,
		done:     false,
		quitting: false,
		err:      nil,
		stepCompleteMessage: "",
		quickstartScriptPath: "",
		waiting:  false,
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
		// Store any message from the completed step
		if msg.message != "" {
			m.stepCompleteMessage = msg.message
		}

		// See if there's a next step, and move to it
		if m.current < len(m.steps)-1 {
			m.current++
			return m, m.executeCurrentStep()
		}

		m.waiting = msg.waiting

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

	// Show the DataRobot banner
	sb.WriteString(tui.Header())
	sb.WriteString("\n\n")

	// Show welcome message
	sb.WriteString(tui.WelcomeStyle.Render("DataRobot Quickstart"))
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
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("%s %s\n", tui.ErrorStyle.Render("Error:"), m.err.Error()))
	} else {
		// Display step message if available
		if m.stepCompleteMessage != "" {
			sb.WriteString("\n")
			sb.WriteString(tui.BaseTextStyle.Render(m.stepCompleteMessage))
			sb.WriteString("\n")
		}

		if !m.done && !m.quitting {
			sb.WriteString("\n")
			sb.WriteString(tui.Footer())
		}
	}

	// Wait for user input if required
	if m.waiting {
		sb.WriteString(tui.BaseTextStyle.Render("\nPress 'y' to confirm, 'n' to cancel: "))
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
	// Return stepErrorMsg{err} if prerequisites are not met

	// Are we in a DataRobot repository?
	if !repo.IsInRepo() {
		return stepErrorMsg{err: errors.New(errNotInRepo)}
	}

	// Do we have the required tools?
	if err := tools.CheckPrerequisites(); err != nil {
		return stepErrorMsg{err: err}
	}

	// TODO Is template configuration correct?
	// TODO Do we need to validate the directory structure?

	// Are we working hard?
	time.Sleep(500 * time.Millisecond) // Simulate work

	return stepCompleteMsg{}
}

// func validateEnvironment() tea.Msg {
// 	// TODO: Implement environment validation logic
// 	// - Check environment variables
// 	// - Validate system requirements
// 	// Return stepErrorMsg{err} if validation fails
// 	time.Sleep(100 * time.Millisecond) // Simulate work

// 	// TODO invoke logic in internal.envvalidator

// 	return stepCompleteMsg{}
// }

func findQuickstart() tea.Msg {
	// If we are in a DataRobot repository, look for a quickstart script in the standard location
	// of .datarobot/cli/bin. If we find it, print its path and execute it after user confirmation.
	// If we do not find it, tell the user that we couldn't find one and suggest that they instead
	// run `dr template setup`.
	quickstartScript, err := findQuickstartScript()
	if err != nil {
		return stepErrorMsg{err: err}
	}

	// If we don't find a script, tell the user they can run `dr template setup` instead.
	if quickstartScript == "" {
		log.Println("No quickstart script found")
		return stepCompleteMsg{message: "Could not find a quickstart script. You can run 'dr template setup' to set up your application.\n", done: true}
	}

	log.Println("Found quickstart script at:", quickstartScript)

	fmt.Printf("\nA quickstart script has been found at: %s\n", quickstartScript)
	return stepCompleteMsg{message: "Do you want to execute this script? (y/n): ", waiting: true}
}

func executeQuickstart() tea.Msg {
	// Get the quickstart script path from the model, and execute it
	// If none was found, then tell the user

	quickstartScript, err := findQuickstartScript()
	if err != nil {
		return stepErrorMsg{err: err}
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
		return stepErrorMsg{err: fmt.Errorf(errScriptExecutionFailed, err)}
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
		return "", fmt.Errorf(errScriptSearchFailed, err)
	}

	// Find the first executable file
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			continue
		}

		// Skip directories
		if info.IsDir() {
			continue
		}

		// Check if file is executable
		if isExecutable(match, info) {
			return match, nil
		}
	}

	// No executable script found - this is not an error
	return "", nil
}

// isExecutable determines if a file is executable based on platform-specific rules
func isExecutable(path string, info os.FileInfo) bool {
	// On Windows, check for common executable extensions
	if runtime.GOOS == "windows" {
		ext := strings.ToLower(filepath.Ext(path))
		return ext == ".exe" || ext == ".bat" || ext == ".cmd" || ext == ".ps1"
	}

	// On Unix-like systems, check execute permission bits
	// 0o111 checks if any execute bit is set (user, group, or other)
	return info.Mode()&0o111 != 0
}
