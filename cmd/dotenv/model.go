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
	prompts            []prompt
	savedResponses     map[string]interface{}
	envResponses       map[string]interface{}
	currentPromptIndex int
	currentPrompt      promptModel
}

type prompt struct {
	rawPrompt envbuilder.UserPrompt
	key       string
	env       string
	requires  []envbuilder.ParentOption
	helpMsg   string
}

type (
	errMsg struct{ err error }

	dotenvFileUpdatedMsg struct {
		variables      []variable
		contents       string
		dotenvTemplate string
		promptUser     bool
	}

	wizardFinishedMsg struct{}

	promptsLoadedMsg struct {
		prompts []prompt
	}
)

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
		builder := envbuilder.NewEnvBuilder()

		currentDir := filepath.Dir(m.DotenvFile)

		userPrompts, err := builder.GatherUserPrompts(currentDir)
		if err != nil {
			return func() tea.Msg {
				return errMsg{err}
			}
		}

		prompts := make([]prompt, 0, len(userPrompts))

		for _, p := range userPrompts {
			switch p := p.(type) {
			case envbuilder.UserPrompt:
				prompts = append(prompts, prompt{
					rawPrompt: p,
					key:       p.Key,
					env:       p.Env,
					requires:  p.Requires,
					helpMsg:   p.Help,
				})
			case envbuilder.UserPromptCollection:
				for _, up := range p.Prompts {
					prompts = append(prompts, prompt{
						rawPrompt: up,
						key:       up.Key,
						env:       up.Env,
						requires:  p.Requires,
						helpMsg:   up.Help,
					})
				}
			}
		}

		return promptsLoadedMsg{prompts}
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.saveEnvFile(), tea.WindowSize())
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) { //nolint: cyclop
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
		// m.screen = wizardScreen
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
		m.currentPromptIndex = 0
		m.savedResponses = make(map[string]interface{})
		m.envResponses = make(map[string]interface{})

		if len(m.prompts) == 0 {
			return m, func() tea.Msg {
				return wizardFinishedMsg{}
			}
		}

		m.currentPrompt = newPromptModel(m.prompts[0])
		cmd := m.currentPrompt.input.Focus()

		return m, cmd
	case wizardFinishedMsg:
		m.screen = listScreen
		return m, nil
	}

	switch m.screen {
	case listScreen:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch keypress := msg.String(); keypress {
			case "w":
				m.screen = wizardScreen
				m.currentPromptIndex = 0
				m.savedResponses = make(map[string]interface{})
				m.envResponses = make(map[string]interface{})
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
				// m.saving = true
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
			case "enter":
				currentPrompt := m.prompts[m.currentPromptIndex]
				value := m.currentPrompt.input.Value()

				// If a prompt has options, map the selected option name to its value
				// Sometimes options have human readable names, but values we want to store
				if currentPrompt.rawPrompt.Options != nil && len(currentPrompt.rawPrompt.Options) > 0 {
					for _, option := range currentPrompt.rawPrompt.Options {
						if option.Name == value && option.Value != "" {
							value = option.Value
							break
						}
					}
				}
				m.savedResponses[currentPrompt.key] = value

				if currentPrompt.env != "" {
					m.envResponses[currentPrompt.env] = m.currentPrompt.input.Value()
				}

				m.currentPromptIndex++
				// Check if next prompt has requirements
				for m.currentPromptIndex < len(m.prompts) {
					nextPrompt := m.prompts[m.currentPromptIndex]
					meetsRequirements := true

					for _, req := range nextPrompt.requires {
						if val, ok := m.savedResponses[req.Name]; !ok || val != req.Value {
							meetsRequirements = false
							break
						}
					}

					if meetsRequirements {
						break
					}

					m.currentPromptIndex++
				}

				if m.currentPromptIndex >= len(m.prompts) {
					return m, m.SuccessCmd
				}

				m.currentPrompt = newPromptModel(m.prompts[m.currentPromptIndex])
				cmd := m.currentPrompt.input.Focus()

				return m, cmd
			}
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
