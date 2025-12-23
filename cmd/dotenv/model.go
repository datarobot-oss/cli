// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package dotenv

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/internal/envbuilder"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/viper"
)

const (
	// Key bindings
	keyQuit         = "enter"
	keyInteractive  = "w"
	keyEdit         = "e"
	keyOpenExternal = "o"
	keyExit         = "esc"
	keySave         = "ctrl+s"
	keyBack         = "ctrl+p"
)

type screens int

const (
	listScreen = screens(iota)
	editorScreen
	wizardScreen
)

type Model struct {
	screen             screens
	initialScreen      screens
	DotenvFile         string
	variables          []envbuilder.Variable
	err                error
	textarea           textarea.Model
	contents           string
	width              int
	height             int
	SuccessCmd         tea.Cmd
	prompts            []envbuilder.UserPrompt
	currentPromptIndex int
	currentPrompt      promptModel
	hasPrompts         *bool // Cache whether prompts are available
}

type (
	errMsg struct{ err error }

	dotenvFileUpdatedMsg struct {
		variables  []envbuilder.Variable
		contents   string
		promptUser bool
	}

	promptFinishedMsg struct{}

	promptsLoadedMsg struct {
		prompts []envbuilder.UserPrompt
	}

	openEditorMsg struct{}
)

func promptFinishedCmd() tea.Msg {
	return promptFinishedMsg{}
}

func openEditorCmd() tea.Msg {
	return openEditorMsg{}
}

func (m Model) openInExternalEditor() tea.Cmd {
	return tea.ExecProcess(m.externalEditorCmd(), func(err error) tea.Msg {
		if err != nil {
			return errMsg{err}
		}
		// Reload the file after editing
		variables, contents, err := readDotenvFileVariables(m.DotenvFile)
		if err != nil {
			return errMsg{err}
		}
		// Don't prompt user, just return to list screen
		return dotenvFileUpdatedMsg{variables, contents, false}
	})
}

func (m Model) externalEditorCmd() *exec.Cmd {
	// Determine the editor to use
	// TODO we may want to refactor this in the future to
	// use a separate viper instance for better testability
	// rather than the global one.
	editor := viper.GetString("external-editor")

	return exec.Command(editor, m.DotenvFile)
}

func (m Model) loadVariables() tea.Cmd {
	return func() tea.Msg {
		variables, contents, err := readDotenvFileVariables(m.DotenvFile)
		if err != nil {
			return errMsg{err}
		}

		return dotenvFileUpdatedMsg{variables, contents, true}
	}
}

func (m Model) saveEditedFile() tea.Cmd {
	return func() tea.Msg {
		lines := slices.Collect(strings.Lines(m.contents))
		variables := envbuilder.ParseVariablesOnly(lines)

		err := writeContents(m.contents, m.DotenvFile)
		if err != nil {
			return errMsg{err}
		}

		return dotenvFileUpdatedMsg{variables, m.contents, false}
	}
}

func (m Model) checkPromptsAvailable() bool {
	// Use cached result if available
	if m.hasPrompts != nil {
		return *m.hasPrompts
	}

	// Check if prompts exist by attempting to gather them
	currentDir := filepath.Dir(m.DotenvFile)

	userPrompts, err := envbuilder.GatherUserPrompts(currentDir, nil)

	return err == nil && len(userPrompts) > 0
}

func (m Model) loadPrompts() tea.Cmd {
	return func() tea.Msg {
		currentDir := filepath.Dir(m.DotenvFile)

		userPrompts, err := envbuilder.GatherUserPrompts(currentDir, m.variables)
		if err != nil {
			return errMsg{err}
		}

		return promptsLoadedMsg{userPrompts}
	}
}

func (m Model) updateCurrentPrompt() (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	m.currentPrompt, cmd = newPromptModel(m.prompts[m.currentPromptIndex], promptFinishedCmd)

	return m, cmd
}

func (m Model) moveToNextPrompt() (tea.Model, tea.Cmd) {
	// Update required sections
	m.prompts = envbuilder.DetermineRequiredSections(m.prompts)

	// Advance to next prompt that is required
	for m.currentPromptIndex < len(m.prompts) {
		if m.prompts[m.currentPromptIndex].ShouldAsk() {
			break
		}

		m.currentPromptIndex++
	}

	if m.currentPromptIndex >= len(m.prompts) {
		// Finished all prompts
		// Update the .env file with the responses
		m.contents = envbuilder.DotenvFromPromptsMerged(m.prompts, m.contents)

		return m, m.saveEditedFile()
	}

	return m.updateCurrentPrompt()
}

func (m Model) moveToPreviousPrompt() (tea.Model, tea.Cmd) {
	currentPromptIndex := m.currentPromptIndex

	// Get back to previous prompt that is required
	for {
		currentPromptIndex--
		if currentPromptIndex < 0 {
			return m, nil
		}

		if m.prompts[currentPromptIndex].ShouldAsk() {
			break
		}
	}

	m.currentPromptIndex = currentPromptIndex

	return m.updateCurrentPrompt()
}

func (m Model) Init() tea.Cmd {
	if m.initialScreen == editorScreen {
		return tea.Batch(openEditorCmd, tea.WindowSize())
	}

	if m.initialScreen == wizardScreen {
		return tea.Batch(m.loadPrompts(), tea.WindowSize())
	}

	return tea.Batch(m.loadVariables(), tea.WindowSize())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint: cyclop
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		if m.screen == editorScreen {
			// Width: BoxStyle.Width uses (width-8), then Padding(1,2)=4 chars + borders=2 chars = 14 total
			m.textarea.SetWidth(m.width - 14)
			// Height: header(2) + BoxStyle padding(2) + borders(2) + instructions(4) + status(3) = 13 total
			m.textarea.SetHeight(m.height - 13)
		}

		return m, nil
	case dotenvFileUpdatedMsg:
		m.screen = listScreen
		m.variables = msg.variables
		m.contents = msg.contents

		if msg.promptUser {
			return m, m.loadPrompts()
		}

		return m, nil
	case promptsLoadedMsg:
		// Start in the wizard screen
		m.screen = wizardScreen
		m.prompts = msg.prompts
		m.currentPromptIndex = 0

		// Cache the result
		hasPrompts := len(m.prompts) > 0
		m.hasPrompts = &hasPrompts

		if len(m.prompts) == 0 {
			m.screen = listScreen
			return m, nil
		}

		return m.moveToNextPrompt()
	case openEditorMsg:
		m.screen = editorScreen

		ta := textarea.New()
		// Width: BoxStyle.Width uses (width-8), then Padding(1,2)=4 chars + borders=2 chars = 14 total
		ta.SetWidth(m.width - 14)
		// Height: header(2) + BoxStyle padding(2) + borders(2) + instructions(4) + status(3) = 13 total
		ta.SetHeight(m.height - 13)
		ta.SetValue(m.contents)
		ta.CursorStart()
		cmd := ta.Focus()
		m.textarea = ta

		return m, tea.Batch(cmd, func() tea.Msg {
			return tea.KeyMsg{
				Type:  tea.KeyRunes,
				Runes: []rune("ctrl+home"),
			}
		})
	}

	switch m.screen {
	case listScreen:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch keypress := msg.String(); keypress {
			case keyQuit:
				return m, m.SuccessCmd
			case keyInteractive:
				return m, m.loadPrompts()
			case keyEdit:
				return m, openEditorCmd
			case keyOpenExternal:
				return m, m.openInExternalEditor()
			}
		case errMsg:
			m.err = msg.err
			return m, nil
		}
	case editorScreen:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch keypress := msg.String(); keypress {
			case keySave:
				return m, m.saveEditedFile()
			case keyExit:
				// Quit without saving
				return m, m.SuccessCmd
			}
		}

		var cmd tea.Cmd

		m.textarea, cmd = m.textarea.Update(msg)
		m.contents = m.textarea.Value()

		return m, cmd

	case wizardScreen:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch keypress := msg.String(); keypress {
			case keyExit:
				m.screen = listScreen
				return m, nil
			case keyBack:
				return m.moveToPreviousPrompt()
			}
		case promptFinishedMsg:
			if m.currentPromptIndex < len(m.prompts) {
				values := m.currentPrompt.Values
				m.prompts[m.currentPromptIndex].Value = strings.Join(values, ",")
				m.prompts[m.currentPromptIndex].Commented = false

				m.currentPromptIndex++

				return m.moveToNextPrompt()
			}

			m.screen = listScreen

			return m, nil
		}

		var cmd tea.Cmd

		m.currentPrompt, cmd = m.currentPrompt.Update(msg)

		return m, cmd
	}

	return m, nil
}

func (m Model) View() string {
	var sb strings.Builder

	switch m.screen {
	case listScreen:
		sb.WriteString(m.viewListScreen())
	case editorScreen:
		sb.WriteString(m.viewEditorScreen())
	case wizardScreen:
		sb.WriteString(m.viewWizardScreen())
	}

	// Add status bar showing working directory
	workDir := filepath.Dir(m.DotenvFile)
	if workDir != "" {
		sb.WriteString("\n\n")
		sb.WriteString(tui.StatusBarStyle.Render("📁 Using template found in: " + workDir))
	}

	return sb.String()
}

func (m Model) viewListScreen() string {
	editor := viper.GetString("external-editor")

	var sb strings.Builder

	var content strings.Builder

	sb.WriteString(tui.WelcomeStyle.Render("Environment Variables Menu"))
	sb.WriteString("\n\n")
	fmt.Fprintf(&content, "Variables found in %s:\n\n", m.DotenvFile)

	for _, v := range m.variables {
		content.WriteString(v.StringSecret())
	}

	sb.WriteString(tui.BoxStyle.Render(content.String()))
	sb.WriteString("\n\n")

	if m.checkPromptsAvailable() && len(m.variables) > 0 {
		sb.WriteString(tui.BaseTextStyle.Render("Press w to set up variables interactively."))
		sb.WriteString("\n")
	}

	sb.WriteString(tui.BaseTextStyle.Render("Press e to edit the file directly."))
	sb.WriteString("\n")
	sb.WriteString(tui.BaseTextStyle.Render(fmt.Sprintf("Press o to open the file in your EDITOR (%s).", editor)))
	sb.WriteString("\n")
	sb.WriteString(tui.BaseTextStyle.Render("Press enter to finish."))

	return sb.String()
}

func (m Model) viewEditorScreen() string {
	var sb strings.Builder

	sb.WriteString(tui.WelcomeStyle.Render("Edit Mode"))
	sb.WriteString("\n\n")
	sb.WriteString(tui.BoxStyle.Width(m.width - 8).Render(m.textarea.View()))
	sb.WriteString("\n\n")
	sb.WriteString(tui.BaseTextStyle.Render("Press ctrl+s to save and go to menu."))
	sb.WriteString("\n")
	sb.WriteString(tui.BaseTextStyle.Render("Press esc to quit without saving."))

	return sb.String()
}

func (m Model) viewWizardScreen() string {
	var sb strings.Builder

	sb.WriteString(tui.WelcomeStyle.Render("Interactive Setup"))
	sb.WriteString("\n\n")

	if m.currentPromptIndex < len(m.prompts) {
		sb.WriteString(tui.BoxStyle.Render(m.currentPrompt.View()))
	} else {
		sb.WriteString(tui.BoxStyle.Render(tui.BaseTextStyle.Render("No prompts left")))
	}

	return sb.String()
}
