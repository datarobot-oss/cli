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
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/internal/envbuilder"
	"github.com/datarobot/cli/tui"
)

type screens int

const (
	listScreen = screens(iota)
	editorScreen
	wizardScreen
)

type Model struct {
	screen             screens
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
)

func promptFinishedCmd() tea.Msg {
	return promptFinishedMsg{}
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
		variables, _, _ := variablesFromTemplate(lines)

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

	var value string
	if prompt.Env != "" {
		value = m.envResponses[prompt.Env]
	} else {
		value = m.envResponses["# "+prompt.Key]
	}

	m.currentPrompt, cmd = newPromptModel(prompt, value, promptFinishedCmd)

	return m, cmd
}

func (m Model) updatedContents() string {
	additions := ""

	for env, value := range m.envResponses {
		// Find existing variable using a regex checking for the variable name at the start of a line
		// to avoid matching comments
		varRegex := regexp.MustCompile(fmt.Sprintf(`\n%s *= *[^\n]*\n`, env))
		varBeginEnd := varRegex.FindStringIndex(m.contents)

		varLine := fmt.Sprintf("%s=%v\n", env, value)

		if varBeginEnd == nil {
			if value != "" {
				additions = additions + varLine
			}
		} else {
			// Replace existing value
			varBegin, varEnd := varBeginEnd[0], varBeginEnd[1]

			m.contents = m.contents[:varBegin] + "\n" + varLine + m.contents[varEnd:]
		}
	}

	if len(additions) == 0 {
		return m.contents
	}

	// If the variables isn't in - append them below DATAROBOT_ENDPOINT
	deRegex := regexp.MustCompile(`\nDATAROBOT_ENDPOINT *= *[^\n]*\n`)
	deBeginEnd := deRegex.FindStringIndex(m.contents)

	if deBeginEnd == nil {
		// Insert the new variables at the beginning
		return additions + m.contents
	}

	_, deEnd := deBeginEnd[0], deBeginEnd[1]

	// Insert the new variables after DATAROBOT_ENDPOINT line
	return m.contents[:deEnd] + additions + m.contents[deEnd:]
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.saveEnvFile(), tea.WindowSize())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint: cyclop
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		if m.screen == editorScreen {
			m.textarea.SetWidth(m.width - 1)
			m.textarea.SetHeight(m.height - 12)
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

		if m.envResponses == nil {
			m.envResponses = make(map[string]string)

			for _, v := range m.variables {
				if v.name != "" {
					if v.commented {
						m.envResponses["# "+v.name] = v.value
					} else {
						m.envResponses[v.name] = v.value
					}
				}
			}
		}

		if len(m.prompts) == 0 {
			m.screen = listScreen
			return m, nil
		}

		return m.updateCurrentPrompt()
	}

	switch m.screen {
	case listScreen:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch keypress := msg.String(); keypress {
			case "w":
				return m, m.loadPrompts()
			case "enter":
				return m, m.SuccessCmd
			case "e":
				m.screen = editorScreen
				ta := textarea.New()
				ta.SetWidth(m.width - 1)
				ta.SetHeight(m.height - 14)
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
		case errMsg:
			m.err = msg.err
			return m, nil
		}
	case editorScreen:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch keypress := msg.String(); keypress {
			case "esc":
				return m, m.saveEditedFile()
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
			case "esc":
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

	switch m.screen {
	case listScreen:
		fmt.Fprintf(&sb, "Variables found in %s:\n\n", m.DotenvFile)

		for _, v := range m.variables {
			if v.commented {
				fmt.Fprintf(&sb, "# ")
			}

			fmt.Fprintf(&sb, "%s: ", v.name)

			if v.secret {
				fmt.Fprintf(&sb, "***\n")
			} else {
				fmt.Fprintf(&sb, "%s\n", v.value)
			}
		}

		sb.WriteString("\n")

		if len(m.prompts) > 0 {
			sb.WriteString(tui.BaseTextStyle.Render("Press w to set up variables interactively."))
			sb.WriteString("\n")
		}

		sb.WriteString(tui.BaseTextStyle.Render("Press e to edit the file directly."))
		sb.WriteString("\n")
		sb.WriteString(tui.BaseTextStyle.Render("Press enter to finish and exit."))
	case editorScreen:
		sb.WriteString(m.textarea.View())
		sb.WriteString("\n\n")
		sb.WriteString(tui.BaseTextStyle.Render("Press esc to save and exit"))

	case wizardScreen:
		if m.currentPromptIndex < len(m.prompts) {
			sb.WriteString(m.currentPrompt.View())
		} else {
			sb.WriteString("\n\n")
			sb.WriteString(tui.BaseTextStyle.Render("No prompts left"))
			sb.WriteString("\n")
		}
	}

	return sb.String()
}
