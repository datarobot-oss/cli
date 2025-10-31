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
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/log"
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

	// Editor fallback if env vars are not set
	defaultEditor = "vi"
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
	DotenvTemplate     string
	variables          []variable
	err                error
	textarea           textarea.Model
	contents           string
	width              int
	height             int
	SuccessCmd         tea.Cmd
	prompts            []envbuilder.UserPrompt
	requires           map[string]bool
	envResponses       map[string]string
	currentPromptIndex int
	currentPrompt      promptModel
}

type (
	errMsg struct{ err error }

	dotenvFileUpdatedMsg struct {
		variables      []variable
		contents       string
		dotenvTemplate string
		promptUser     bool
	}

	promptFinishedMsg struct{}

	promptsLoadedMsg struct {
		prompts  []envbuilder.UserPrompt
		requires map[string]bool
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
		variables, contents, dotenvTemplate, err := writeUsingTemplateFile(m.DotenvFile)
		if err != nil {
			return errMsg{err}
		}
		// Don't prompt user, just return to list screen
		return dotenvFileUpdatedMsg{variables, contents, dotenvTemplate, false}
	})
}

func (m Model) externalEditorCmd() *exec.Cmd {
	// Determine the editor to use
	editor := viper.GetString("external_editor")
	if editor == "" {
		editor = defaultEditor // fallback to vi
	}

	return exec.Command(editor, m.DotenvFile)
}

func (m Model) saveEnvFile() tea.Cmd {
	return func() tea.Msg {
		variables, contents, dotenvTemplate, err := writeUsingTemplateFile(m.DotenvFile)
		if err != nil {
			return errMsg{err}
		}

		return dotenvFileUpdatedMsg{variables, contents, dotenvTemplate, true}
	}
}

func (m Model) saveEditedFile() tea.Cmd {
	return func() tea.Msg {
		lines := slices.Collect(strings.Lines(m.contents))
		variables := parseVariablesOnly(lines)

		err := writeContents(m.contents, m.DotenvFile, m.DotenvTemplate)
		if err != nil {
			return errMsg{err}
		}

		return dotenvFileUpdatedMsg{variables, m.contents, m.DotenvTemplate, false}
	}
}

func (m Model) loadPrompts() tea.Cmd {
	return func() tea.Msg {
		currentDir := filepath.Dir(m.DotenvFile)

		userPrompts, roots, err := envbuilder.GatherUserPrompts(currentDir)
		if err != nil {
			return errMsg{err}
		}

		requires := make(map[string]bool, len(roots))

		for _, root := range roots {
			requires[root] = true
		}

		return promptsLoadedMsg{userPrompts, requires}
	}
}

func (m Model) updateCurrentPrompt() (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	prompt := m.prompts[m.currentPromptIndex]

	envKey := prompt.Env
	if envKey == "" {
		envKey = "# " + prompt.Key
	}

	m.currentPrompt, cmd = newPromptModel(prompt, m.envResponses[envKey], promptFinishedCmd)

	return m, cmd
}

// updatedContents merges environment variable responses into the file contents.
// It updates existing variables in-place and appends new variables after DATAROBOT_ENDPOINT.
// If DATAROBOT_ENDPOINT is not found, new variables are prepended to the file.
func (m Model) updatedContents() string {
	// There has to be a better way to write out an .env file. This is messy.
	// Like parsing the entire dotenv into a structure, modifying the structure,
	// then serializing it.
	additions := ""

	for name, value := range m.envResponses {
		// Find existing variable using a regex checking for the variable name at the start of a line
		// to avoid matching comments
		varRegex := regexp.MustCompile(fmt.Sprintf(`(?m)^%s *= *[^\n]*$`, name))
		varBeginEnd := varRegex.FindStringIndex(m.contents)

		varString := fmt.Sprintf("%s=%v", name, value)

		if varBeginEnd == nil {
			if value != "" {
				additions = additions + varString + "\n"
			}
		} else {
			// Replace existing value
			varBegin, varEnd := varBeginEnd[0], varBeginEnd[1]

			m.contents = m.contents[:varBegin] + varString + m.contents[varEnd:]
		}
	}

	if len(additions) == 0 {
		return m.contents
	}

	// If the variables aren't in file - append them below DATAROBOT_ENDPOINT
	// If DATAROBOT_ENDPOINT is not found, prepend to the file
	endpointLineRegex := regexp.MustCompile(fmt.Sprintf(`(?m)^%s *= *[^\n]*\n`, datarobotEndpointVar))
	endpointLineMatch := endpointLineRegex.FindStringIndex(m.contents)

	if endpointLineMatch == nil {
		log.Debug("DATAROBOT_ENDPOINT not found, prepending new variables to the beginning of the file")
		// Insert the new variables at the beginning
		return additions + m.contents
	}

	insertPos := endpointLineMatch[1]

	// Insert the new variables after DATAROBOT_ENDPOINT line
	updatedContents := m.contents[:insertPos] + additions + m.contents[insertPos:]

	return updatedContents
}

func (m Model) responsesFromVariables() map[string]string {
	if m.envResponses != nil {
		return m.envResponses
	}

	responses := make(map[string]string)

	for _, v := range m.variables {
		if v.name == "" {
			continue
		}

		if v.commented {
			responses["# "+v.name] = v.value
		} else {
			// Capture existing env var values
			existingEnvValue, ok := os.LookupEnv(v.name)
			if ok {
				responses[v.name] = existingEnvValue
			} else {
				responses[v.name] = v.value
			}
		}
	}

	return responses
}

func (m Model) Init() tea.Cmd {
	if m.initialScreen == editorScreen {
		return tea.Batch(openEditorCmd, tea.WindowSize())
	}

	if m.initialScreen == wizardScreen {
		return tea.Batch(m.loadPrompts(), tea.WindowSize())
	}

	return tea.Batch(m.saveEnvFile(), tea.WindowSize())
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
		m.DotenvTemplate = msg.dotenvTemplate

		if msg.promptUser {
			return m, m.loadPrompts()
		}

		return m, nil
	case promptsLoadedMsg:
		// Start in the wizard screen
		m.screen = wizardScreen
		m.prompts = msg.prompts
		m.requires = msg.requires
		m.currentPromptIndex = 0
		m.envResponses = m.responsesFromVariables()

		if len(m.prompts) == 0 {
			m.screen = listScreen
			return m, nil
		}

		return m.updateCurrentPrompt()
	case openEditorMsg:
		m.screen = editorScreen
		m.envResponses = m.responsesFromVariables()

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
			}
		case promptFinishedMsg:
			if m.currentPromptIndex < len(m.prompts) { //nolint: nestif
				currentPrompt := m.prompts[m.currentPromptIndex]
				values := m.currentPrompt.Values

				// Update required sections
				for _, option := range currentPrompt.Options {
					if option.Requires != "" {
						if option.Value != "" && slices.Contains(values, option.Value) {
							m.requires[option.Requires] = true
						} else if option.Value == "" && slices.Contains(values, option.Name) {
							m.requires[option.Requires] = true
						}
					}
				}

				if currentPrompt.Env != "" {
					m.envResponses[currentPrompt.Env] = strings.Join(values, ",")
				}

				m.currentPromptIndex++
				// Advance to next prompt that is required
				for m.currentPromptIndex < len(m.prompts) {
					nextPrompt := m.prompts[m.currentPromptIndex]

					if m.requires[nextPrompt.Section] {
						break
					}

					m.currentPromptIndex++
				}

				if m.currentPromptIndex >= len(m.prompts) {
					// Finished all prompts
					// Update the .env file with the responses
					m.contents = m.updatedContents()

					return m, m.saveEditedFile()
				}

				return m.updateCurrentPrompt()
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

	var content strings.Builder

	switch m.screen {
	case listScreen:
		sb.WriteString(tui.WelcomeStyle.Render("Environment Variables Menu"))
		sb.WriteString("\n\n")
		fmt.Fprintf(&content, "Variables found in %s:\n\n", m.DotenvFile)

		for _, v := range m.variables {
			if v.commented {
				fmt.Fprintf(&content, "# ")
			}

			fmt.Fprintf(&content, "%s: ", v.name)

			if v.secret {
				fmt.Fprintf(&content, "***\n")
			} else {
				fmt.Fprintf(&content, "%s\n", v.value)
			}
		}

		sb.WriteString(tui.BoxStyle.Render(content.String()))
		sb.WriteString("\n\n")

		if len(m.variables) > 0 {
			sb.WriteString(tui.BaseTextStyle.Render("Press w to set up variables interactively."))
			sb.WriteString("\n")
		}

		sb.WriteString(tui.BaseTextStyle.Render("Press e to edit the file directly."))
		sb.WriteString("\n")
		sb.WriteString(tui.BaseTextStyle.Render("Press o to open the file in your EDITOR."))
		sb.WriteString("\n")
		sb.WriteString(tui.BaseTextStyle.Render("Press enter to finish."))
	case editorScreen:
		sb.WriteString(tui.WelcomeStyle.Render("Edit Mode"))
		sb.WriteString("\n\n")
		sb.WriteString(tui.BoxStyle.Width(m.width - 8).Render(m.textarea.View()))
		sb.WriteString("\n\n")
		sb.WriteString(tui.BaseTextStyle.Render("Press ctrl+s to save and go to menu."))
		sb.WriteString("\n")
		sb.WriteString(tui.BaseTextStyle.Render("Press esc to quit without saving."))

	case wizardScreen:
		sb.WriteString(tui.WelcomeStyle.Render("Interactive Setup"))
		sb.WriteString("\n\n")

		if m.currentPromptIndex < len(m.prompts) {
			sb.WriteString(tui.BoxStyle.Render(m.currentPrompt.View()))
		} else {
			sb.WriteString(tui.BoxStyle.Render(tui.BaseTextStyle.Render("No prompts left")))
		}
	}

	// Add status bar showing working directory
	workDir := filepath.Dir(m.DotenvFile)
	if workDir != "" {
		sb.WriteString("\n\n")
		sb.WriteString(tui.StatusBarStyle.Render("📁 Working in: " + workDir))
	}

	return sb.String()
}
