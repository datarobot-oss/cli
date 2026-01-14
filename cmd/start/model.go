// Copyright 2025 DataRobot, Inc. and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/repo"
	"github.com/datarobot/cli/internal/state"
	"github.com/datarobot/cli/internal/tools"
	"github.com/datarobot/cli/internal/version"
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
	hideMenu             bool
	quitting             bool
	err                  error
	stepCompleteMessage  string // Optional message from the completed step
	quickstartScriptPath string // Path to the quickstart script to execute
	selfUpdate           bool   // Whether to ask for self update
	waitingToExecute     bool   // Whether to wait for user input before proceeding
	needTemplateSetup    bool   // Whether we need to run template setup after quitting
}

type stepCompleteMsg struct {
	message              string // Optional message to display to the user
	waiting              bool   // Whether to wait for user input before proceeding
	done                 bool   // Whether the quickstart process is complete
	hideMenu             bool   // Do not show menu
	quickstartScriptPath string // Path to quickstart script found (if any)
	selfUpdate           bool   // Whether to ask for self update
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
			{description: "Checking DataRobot CLI version...", fn: checkSelfVersion},
			{description: "Checking template prerequisites...", fn: checkPrerequisites},
			// TODO Implement validateEnvironment
			// {description: "Validating environment...", fn: validateEnvironment},
			{description: "Checking repository setup...", fn: checkRepository},
			{description: "Finding and executing start command...", fn: findAndExecuteStart},
		},
		opts: opts,
	}
}

func (m Model) Init() tea.Cmd {
	log.Info("start: init", "steps", len(m.steps), "answer_yes", m.opts.AnswerYes)

	return m.executeCurrentStep()
}

func (m Model) executeCurrentStep() tea.Cmd {
	if m.current >= len(m.steps) {
		return nil
	}

	currentStep := m.currentStep()
	log.Info("start: execute step", "index", m.current, "description", currentStep.description)

	return func() tea.Msg {
		return currentStep.fn(&m)
	}
}

func (m Model) executeNextStep() (Model, tea.Cmd) {
	// Check if there are more steps
	if m.current >= len(m.steps)-1 {
		// No more steps, we're done
		log.Info("start: all steps complete", "current", m.current, "steps", len(m.steps))

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

func (m Model) execSelfUpdate() tea.Cmd {
	cmd := exec.Command("dr", "self", "update")

	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			return stepErrorMsg{err: err}
		}

		return stepCompleteMsg{
			message:  "Update finished. Please start last command again.",
			hideMenu: true,
			done:     true,
		}
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case stepCompleteMsg:
		return m.handleStepComplete(msg)

	case stepErrorMsg:
		log.Debug("start: step error", "error", msg.err)

		m.err = msg.err

		return m, tea.Quit

	case scriptCompleteMsg:
		log.Debug("start: script complete")

		// Script execution completed successfully, update state and quit
		_ = state.UpdateAfterSuccessfulRun()

		return m, tea.Quit
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// If there's an error, any key press quits
	if m.err != nil {
		log.Debug("start: key ignored due to error", "key", msg.String(), "error", m.err)

		return m, tea.Quit
	}

	// If we're waiting for user confirmation to execute the script
	if m.waitingToExecute {
		log.Debug("start: key while waiting", "key", msg.String(), "self_update", m.selfUpdate, "script", m.quickstartScriptPath)

		switch msg.String() {
		case "y", "Y", "enter":
			// Punch it, Chewie!
			m.waitingToExecute = false
			m.stepCompleteMessage = ""

			if m.selfUpdate {
				return m, m.execSelfUpdate()
			}

			if m.quickstartScriptPath != "" {
				return m, m.execQuickstartScript()
			}

			return m.executeNextStep()
		case "n", "N", "q", "esc":
			// Just hang on. Hang on, Dak.
			if m.selfUpdate {
				m.selfUpdate = false
				return m.handleStepComplete(stepCompleteMsg{})
			}

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
		log.Info("start: quit requested", "key", msg.String())

		m.quitting = true

		return m, tea.Quit
	}

	return m, nil
}

func (m Model) handleStepComplete(msg stepCompleteMsg) (tea.Model, tea.Cmd) {
	log.Debug(
		"start: step complete",
		"message", msg.message,
		"waiting", msg.waiting,
		"done", msg.done,
		"hide_menu", msg.hideMenu,
		"self_update", msg.selfUpdate,
		"execute_script", msg.executeScript,
		"quickstart_script_path", msg.quickstartScriptPath,
		"need_template_setup", msg.needTemplateSetup,
	)

	// Store any message from the completed step
	if msg.message != "" {
		m.stepCompleteMessage = msg.message
	}

	if msg.hideMenu {
		m.hideMenu = msg.hideMenu
	}

	if msg.selfUpdate {
		m.selfUpdate = msg.selfUpdate
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

func (m Model) View() string { //nolint: cyclop
	var sb strings.Builder

	if !m.hideMenu {
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
	}

	// Display error or status message
	if m.err != nil {
		sb.WriteString(fmt.Sprintf("%s %s\n", tui.ErrorStyle.Render("Error: "), m.err.Error()))

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
		} else if !m.selfUpdate {
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

func checkSelfVersion(_ *Model) tea.Msg {
	// Do we have the required self version?
	tool, err := tools.GetSelfRequirement()
	if err != nil {
		return stepErrorMsg{err: err}
	}

	if !tools.SufficientSelfVersion(tool.MinimumVersion) {
		missing := fmt.Sprintf("%s (minimal: v%s, installed: %s)\nDo you want to update it now?",
			tool.Name, tool.MinimumVersion, version.Version)

		return stepCompleteMsg{
			waiting:    true,
			selfUpdate: true,
			message:    missing,
		}
	}

	return stepCompleteMsg{}
}

func checkPrerequisites(_ *Model) tea.Msg {
	// Return stepErrorMsg{err} if prerequisites are not met

	// Do we have the required tools?
	missing := tools.MissingPrerequisites()

	if missing != "" {
		return stepErrorMsg{err: errors.New(missing)}
	}

	// TODO Is template configuration correct?
	// TODO Do we need to validate the directory structure?

	// Are we working hard?
	// time.Sleep(500 * time.Millisecond) // Simulate work

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
		pwd, _ := os.Getwd()
		log.Info("start: pwd " + pwd + " is not a DataRobot repository")
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

	// No start command found - warn user that template may not support DR CLI
	return stepCompleteMsg{
		message: "No start command or quickstart script found.\nThis template may not yet fully support the DataRobot CLI.\nPlease check the template README for more information on how to get started.\n",
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
