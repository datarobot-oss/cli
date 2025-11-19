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
	needTemplateSetup    bool   // Whether we need to run template setup after quitting
}

type stepCompleteMsg struct {
	message              string // Optional message to display to the user
	waiting              bool   // Whether to wait for user input before proceeding
	done                 bool   // Whether the quickstart process is complete
	quickstartScriptPath string // Path to quickstart script found (if any)
	executeScript        bool   // Whether to execute the script immediately
	needTemplateSetup    bool   // Whether we need to run template setup
}

type scriptCompleteMsg struct{}

type stepErrorMsg struct {
	err error // Error encountered during step execution
}

// err messages used in the start command.
const (
	errScriptSearchFailed = "Failed to search for quickstart script: %w"
	preExecutionDelay     = 200 * time.Millisecond // Brief delay before executing scripts to avoid glitchy screen resets
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
			{description: "Checking repository setup...", fn: checkRepository},
			{description: "Finding and executing start command...", fn: findAndExecuteStart},
		},
		opts:                 opts,
		current:              0,
		done:                 false,
		quitting:             false,
		err:                  nil,
		stepCompleteMessage:  "",
		quickstartScriptPath: "",
		waitingToExecute:     false,
		needTemplateSetup:    false,
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
	// Special case: if the path is "task-start", run 'task start' directly
	if m.quickstartScriptPath == "task-start" {
		// Run 'task start' - use the task binary directly
		taskPath, err := exec.LookPath("task")
		if err != nil {
			// Fallback to just "task" and let the system find it
			taskPath = "task"
		}

		cmd := exec.Command(taskPath, "start")

		return tea.ExecProcess(cmd, func(_ error) tea.Msg {
			return scriptCompleteMsg{}
		})
	}

	// Regular quickstart script execution
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

	// Store whether we need template setup
	if msg.needTemplateSetup {
		m.needTemplateSetup = true
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

		return m, tea.Quit
	}

	// Move to next step
	return m.executeNextStep()
}

func (m Model) View() string {
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString(tui.WelcomeStyle.Render("🚀 DataRobot AI Application Quickstart"))
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
		sb.WriteString(fmt.Sprintf("%s %s\n", tui.ErrorStyle.Render("Error: "), m.err.Error()))
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

func checkRepository(m *Model) tea.Msg {
	// Check if we're in a DataRobot repository
	// If not, we need to run templates setup
	if !repo.IsInRepo() {
		// Not in a repo, signal that we need to run templates setup and quit
		return stepCompleteMsg{
			message:           "Not in a DataRobot repository. Launching template setup...\n",
			done:              true,
			needTemplateSetup: true,
		}
	}

	// We're in a repo, continue to next step
	return stepCompleteMsg{}
}

func findAndExecuteStart(m *Model) tea.Msg {
	// Try to find and execute either 'dr task run start' or a quickstart script
	// Prefer 'dr task run start' if available

	// First, check if 'task start' exists
	hasTask, err := hasTaskStart()
	if err != nil {
		// Explicitly ignore the error - just continue to check for quickstart script
		// This could happen if task isn't installed or other transient issues
		_ = err
	}

	if hasTask {
		// Add a brief delay before executing to avoid glitchy screen resets
		time.Sleep(preExecutionDelay)

		// Run 'task start' as an external command
		return stepCompleteMsg{
			message:              "Running 'task start'...\n",
			quickstartScriptPath: "task-start", // Special marker for task start
			executeScript:        true,
		}
	}

	// If no 'task start', look for quickstart script
	quickstartScript, err := findQuickstartScript()
	if err != nil {
		return stepErrorMsg{err: err}
	}

	if quickstartScript != "" {
		// Add a brief delay before executing to avoid glitchy screen resets
		time.Sleep(preExecutionDelay)

		// Found a quickstart script
		// If '--yes' flag is set, don't wait for confirmation
		waitForConfirmation := !m.opts.AnswerYes

		return stepCompleteMsg{
			message:              fmt.Sprintf("Found quickstart script at: %s\n", quickstartScript),
			waiting:              waitForConfirmation,
			quickstartScriptPath: quickstartScript,
		}
	}

	// No start command found
	return stepCompleteMsg{
		message: "No start command or quickstart script found.\n",
		done:    true,
	}
}

func hasTaskStart() (bool, error) {
	// Check if 'task start' is available by running 'task --list'
	// and checking if 'start' is in the output
	taskPath, err := exec.LookPath("task")
	if err != nil {
		return false, err
	}

	cmd := exec.Command(taskPath, "--list")

	output, err := cmd.Output()
	if err != nil {
		// If the command fails, it could be because we're not in a template directory
		// or task isn't configured - this is not an error, just means no task available
		return false, nil
	}

	// Check if "start" appears in the output
	// Look for either "* start" (list format) or "start:" (detailed format)
	outputStr := string(output)
	hasStart := strings.Contains(outputStr, "* start") || strings.Contains(outputStr, "start:")

	return hasStart, nil
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
