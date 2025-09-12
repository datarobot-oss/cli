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
	prompts            []promptMsg
	savedResponses     map[string]interface{}
	envResponses       map[string]interface{}
	currentPromptIndex int
}

type (
	errMsg struct{ err error }

	dotenvFileUpdatedMsg struct {
		variables      []variable
		contents       string
		dotenvTemplate string
	}

	wizardFinishedMsg struct{}

	promptMsg struct {
		rawPrompt envbuilder.UserPrompt
		key       string
		env       string
		requires  []envbuilder.ParentOption
		helpMsg   string
	}
)

func (m Model) saveEnvFile() tea.Cmd {
	return func() tea.Msg {
		variables, contents, dotenvTemplate, err := writeUsingTemplateFile(m.DotenvFile)
		if err != nil {
			return errMsg{err}
		}

		return dotenvFileUpdatedMsg{variables, contents, dotenvTemplate}
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

		return dotenvFileUpdatedMsg{variables, m.contents, m.DotenvTemplate}
	}
}

func (m Model) Init() tea.Cmd {
	builder := envbuilder.NewEnvBuilder()

	currentDir := filepath.Dir(m.DotenvFile)

	prompts, err := builder.GatherUserPrompts(currentDir)
	if err != nil {
		envbuilder.PrintToStdOut(fmt.Sprintf("Error gathering user prompts: %v", err))
		return func() tea.Msg {
			return errMsg{err}
		}
	}
	promptMsgs := make([]promptMsg, 0, len(prompts))
	for _, p := range prompts {
		switch p := p.(type) {
		case envbuilder.UserPrompt:
			promptMsgs = append(promptMsgs, promptMsg{
				rawPrompt: p,
				key:       p.Key,
				env:       p.Env,
				requires:  p.Requires,
				helpMsg:   p.Help,
			})
		case envbuilder.UserPromptCollection:
			for _, up := range p.Prompts {
				promptMsgs = append(promptMsgs, promptMsg{
					rawPrompt: up,
					key:       up.Key,
					env:       up.Env,
					requires:  p.Requires,
					helpMsg:   up.Help,
				})
			}
		}
	}
	m.prompts = promptMsgs
	m.currentPromptIndex = 0
	m.savedResponses = make(map[string]interface{})
	m.envResponses = make(map[string]interface{})
	if len(m.prompts) == 0 {
		return func() tea.Msg {
			return wizardFinishedMsg{}
		}
	}

	// Start in the wizard screen
	m.screen = wizardScreen
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
		m.screen = wizardScreen
		m.variables = msg.variables
		m.contents = msg.contents
		m.DotenvTemplate = msg.dotenvTemplate

		return m, nil

	case wizardFinishedMsg:
		m.screen = listScreen
		return m, nil
	}

	switch m.screen {
	case listScreen:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch keypress := msg.String(); keypress {
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
			default:
				currentPrompt := m.prompts[m.currentPromptIndex]
				m.savedResponses[currentPrompt.key] = keypress
				if currentPrompt.env != "" {
					m.envResponses[currentPrompt.env] = keypress
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
			}
		}
	}

	return m, nil
}

func (m Model) View() string {
	var sb strings.Builder

	switch m.screen {
	case listScreen:
		//fmt.Fprintf(&sb, "Variables found in %s:\n\n", m.DotenvFile)
		//
		//for _, v := range m.variables {
		//	if v.commented {
		//		fmt.Fprintf(&sb, "# ")
		//	}
		//
		//	fmt.Fprintf(&sb, "%s: ", v.name)
		//
		//	if v.secret {
		//		fmt.Fprintf(&sb, "***\n")
		//	} else {
		//		fmt.Fprintf(&sb, "%s\n", v.value)
		//	}
		//}
		//
		//sb.WriteString("\n")
		//sb.WriteString(tui.BaseTextStyle.Render("Press e to edit variables, enter to finish"))
		currentPrompt := m.prompts[m.currentPromptIndex]
		sb.WriteString("\n\n")
		sb.WriteString(tui.BaseTextStyle.Render(currentPrompt.helpMsg))
		sb.WriteString("\n")
		if currentPrompt.rawPrompt.Default != "" {
			sb.WriteString(tui.BaseTextStyle.Render(fmt.Sprintf("Default: %s", currentPrompt.rawPrompt.Default)))
			sb.WriteString("\n")
		}
	case editorScreen:
		sb.WriteString(m.textarea.View())
		sb.WriteString("\n\n")
		sb.WriteString(tui.BaseTextStyle.Render("Press esc to save and exit"))

	case wizardScreen:
		currentPrompt := m.prompts[m.currentPromptIndex]
		sb.WriteString("\n\n")
		sb.WriteString(tui.BaseTextStyle.Render(currentPrompt.helpMsg))
		sb.WriteString("\n")
		if currentPrompt.rawPrompt.Default != "" {
			sb.WriteString(tui.BaseTextStyle.Render(fmt.Sprintf("Default: %s", currentPrompt.rawPrompt.Default)))
			sb.WriteString("\n")
		}
	}

	return sb.String()
}
