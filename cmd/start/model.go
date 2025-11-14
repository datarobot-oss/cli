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
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/internal/repo"
	"github.com/datarobot/cli/internal/state"
	"github.com/datarobot/cli/internal/tools"
	"github.com/datarobot/cli/tui"
)

// step represents a single step in the quickstart process
type step struct {
	// description is a brief summary of the step
	description string
	// fn is the function that performs the step's Update action
	fn func(*Model) tea.Msg
}

type Model struct {
	opts                 Options
	steps                []step
	current              int
	done                 bool
	quitting             bool
	err                  error
	stepCompleteMessage  string // Optional message from the completed step
	quickstartScriptPath string // Path to the quickstart script to execute
	waitingToExecute     bool   // Whether to wait for user input before proceeding
}

type stepCompleteMsg struct {
	message              string // Optional message to display to the user
	waiting              bool   // Whether to wait for user input before proceeding
	done                 bool   // Whether the quickstart process is complete
	quickstartScriptPath string // Path to quickstart script found (if any)
	executeScript        bool   // Whether to execute the script immediately
}

type scriptCompleteMsg struct{}

type stepErrorMsg struct {
	err error // Error encountered during step execution
}

// err messages used in the start command.
const (
	errNotInRepo          = "Not inside a DataRobot repository. Run `dr templates setup` to create one or navigate to an existing repository"
	errScriptSearchFailed = "Failed to search for quickstart script: %w"
)

var (
	checkMark = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).SetString("✓")
	arrow     = lipgloss.NewStyle().Foreground(tui.DrPurple).SetString("→")
)

func NewStartModel(opts Options) Model {
	return Model{
		steps: []step{
			{description: "Starting application quickstart process...", fn: startQuickstart},
			{description: "Checking template prerequisites...", fn: checkPrerequisites},
			// TODO Implement validateEnvironment
			// {description: "Validating environment...", fn: validateEnvironment},
			{description: "Locating quickstart script...", fn: findQuickstart},
			{description: "Executing quickstart script...", fn: executeQuickstart},
		},
		opts:                 opts,
		current:              0,
		done:                 false,
		quitting:             false,
		err:                  nil,
		stepCompleteMessage:  "",
		quickstartScriptPath: "",
		waitingToExecute:     false,
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
		return currentStep.fn(&m)
	}
}

func (m Model) executeNextStep() (Model, tea.Cmd) {
	// Check if there are more steps
	if m.current >= len(m.steps)-1 {
		// No more steps, we're done
		m.done = true
		return m, tea.Quit
	}

	// Move to next step and execute it
	m.current++

	return m, m.executeCurrentStep()
}

func (m Model) currentStep() step {
	return m.steps[m.current]
}

func (m Model) execQuickstartScript() tea.Cmd {
	cmd := exec.Command(m.quickstartScriptPath)

	return tea.ExecProcess(cmd, func(_ error) tea.Msg {
		return scriptCompleteMsg{}
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case stepCompleteMsg:
		return m.handleStepComplete(msg)

	case stepErrorMsg:
		m.err = msg.err
		m.quitting = true
		// Don't quit immediately - wait for user to see the error and press a key
		return m, nil

	case scriptCompleteMsg:
		// Script execution completed successfully, update state and quit
		_ = state.UpdateAfterSuccessfulRun()

		return m, tea.Quit
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// If there's an error, any key press quits
	if m.err != nil {
		return m, tea.Quit
	}

	// If we're waiting for user confirmation to execute the script
	if m.waitingToExecute {
		switch msg.String() {
		case "y", "Y", "enter":
			// Punch it, Chewie!
			m.waitingToExecute = false
			m.stepCompleteMessage = ""

			if m.quickstartScriptPath != "" {
				return m, m.execQuickstartScript()
			}

			return m.executeNextStep()
		case "n", "N", "q", "esc":
			// Just hang on. Hang on, Dak.
			// User chose to not execute script, so update state and quit
			_ = state.UpdateAfterSuccessfulRun()
			m.quitting = true

			return m, tea.Quit
		}
		// Ignore other keys when waiting
		return m, nil
	}

	// Normal key handling when not waiting
	switch msg.String() {
	case "q", "esc":
		m.quitting = true
		return m, tea.Quit
	}

	return m, nil
}

func (m Model) handleStepComplete(msg stepCompleteMsg) (tea.Model, tea.Cmd) {
	// Store any message from the completed step
	if msg.message != "" {
		m.stepCompleteMessage = msg.message
	}

	// Store quickstart script path if provided
	if msg.quickstartScriptPath != "" {
		m.quickstartScriptPath = msg.quickstartScriptPath
	}

	// If this step requires executing a script, do it now
	if msg.executeScript && m.quickstartScriptPath != "" {
		return m, m.execQuickstartScript()
	}

	// If this step requires waiting for user input, set the flag and stop
	if msg.waiting {
		m.waitingToExecute = true
		return m, nil
	}

	// If this step marks completion, we're done
	if msg.done {
		m.done = true
		// Update state and quit
		_ = state.UpdateAfterSuccessfulRun()

		return m, tea.Quit
	}

	// Move to next step
	return m.executeNextStep()
}

func (m Model) View() string {
	var sb strings.Builder

	// Show crazy DR header
	sb.WriteString(tui.Header())
	sb.WriteString("\n\n")

	// Show welcome message
	sb.WriteString(tui.WelcomeStyle.Render("DataRobot Quickstart"))
	sb.WriteString("\n\n")

	for i, step := range m.steps {
		if i < m.current {
			sb.WriteString(fmt.Sprintf("  %s %s\n", checkMark, tui.DimStyle.Render(step.description)))
		} else if i == m.current {
			sb.WriteString(fmt.Sprintf("  %s %s\n", arrow, step.description))
		} else {
			sb.WriteString(fmt.Sprintf("    %s\n", tui.DimStyle.Render(step.description)))
		}
	}

	sb.WriteString("\n")

	// Display error or status message
	if m.err != nil {
		sb.WriteString(fmt.Sprintf("%s %s\n", tui.ErrorStyle.Render("Error:"), m.err.Error()))
		sb.WriteString("\n")
		sb.WriteString(tui.DimStyle.Render("Press any key to exit"))
		sb.WriteString("\n")

		return sb.String()
	}

	// Display step message if available
	if m.stepCompleteMessage != "" {
		sb.WriteString(tui.BaseTextStyle.Render(m.stepCompleteMessage))
		sb.WriteString("\n")
	}

	// Display footer if not done
	if !m.done && !m.quitting {
		sb.WriteString("\n")

		if m.waitingToExecute {
			sb.WriteString(tui.DimStyle.Render("Press 'y' or ENTER to confirm, 'n' to cancel"))
		} else {
			sb.WriteString(tui.Footer())
		}
	}

	sb.WriteString("\n")

	return sb.String()
}

// Step functions

func startQuickstart(_ *Model) tea.Msg {
	// - Set up initial state
	// - Display welcome message
	// - Prepare for subsequent steps
	return stepCompleteMsg{}
}

func checkPrerequisites(_ *Model) tea.Msg {
	// Return stepErrorMsg{err} if prerequisites are not met

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

// func validateEnvironment(m *Model) tea.Msg {
// 	// TODO: Implement environment validation logic
// 	// - Check environment variables
// 	// - Validate system requirements
// 	// Return stepErrorMsg{err} if validation fails
// 	time.Sleep(100 * time.Millisecond) // Simulate work

// 	// TODO invoke logic in internal.envvalidator

// 	return stepCompleteMsg{}
// }

func findQuickstart(m *Model) tea.Msg {
	// If --yes flag is set, don't wait for confirmation
	waitForConfirmation := !m.opts.AnswerYes

	// If we are in a DataRobot repository, look for a quickstart script in the standard location
	// of .datarobot/cli/bin. If we find it, store its path and execute it after user confirmation.
	// If we do not find it, invoke `dr templates setup` to help the user configure their template.
	// If the user has set the '--yes' flag, skip confirmation and execute immediately.
	quickstartScript, err := findQuickstartScript()
	// if the error is due to not being in a repo, we can treat it as no script found
	if err != nil {
		if err.Error() != errNotInRepo {
			return stepErrorMsg{err: err}
		}

		quickstartScript = "" // Ensure no script if not in repo
	}

	// If we don't find a script, we'll proceed to run templates setup in the next step
	if quickstartScript == "" {
		return stepCompleteMsg{
			message:              "Proceed to template setup...\n",
			waiting:              waitForConfirmation,
			quickstartScriptPath: "",
		}
	}

	return stepCompleteMsg{
		message:              fmt.Sprintf("Quickstart found at: %s. Will proceed with execution...\n", quickstartScript),
		waiting:              waitForConfirmation,
		quickstartScriptPath: quickstartScript,
	}
}

func executeQuickstart(m *Model) tea.Msg {
	// Execute the quickstart script that was found and stored in the model
	// If no script path is set, then we should invoke `dr templates setup` after user confirmation
	if m.quickstartScriptPath == "" {
		// We need to quit this program first, then launch setup
		// Store that we need to launch setup and signal completion
		return stepCompleteMsg{message: "Launching template setup...\n", done: true}
	}

	// Signal that we should execute the script
	// The actual execution happens in handleStepComplete to ensure proper tea.ExecProcess handling
	return stepCompleteMsg{executeScript: true}
}

func findQuickstartScript() (string, error) {
	// Are we in a DataRobot repository?
	if !repo.IsInRepo() {
		return "", errors.New(errNotInRepo)
	}

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
