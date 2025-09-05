// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package clone

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/internal/drapi"
)

type Model struct {
	template   drapi.Template
	input      textinput.Model
	debounceID int
	cloning    bool
	exists     string
	repoURL    string
	cloneError bool
	finished   bool
	out        string
	Dir        string
	SuccessCmd tea.Cmd
}

type (
	focusInputMsg    struct{}
	validateInputMsg struct{ id int }
	validMsg         struct{}
	DirStatusMsg     struct {
		dir     string
		exists  bool
		repoURL string
	}
	cloneSuccessMsg struct{ out string }
	cloneErrorMsg   struct{ out string }
)

func focusInput() tea.Msg { return focusInputMsg{} }

func dirExists(dir string) bool {
	_, err := os.Stat(dir)
	return !os.IsNotExist(err)
}

func DirIsAbsolute(dir string) bool {
	return filepath.IsAbs(dir)
}

func CleanDirPath(dir string) string {
	currentUser, err := user.Current()
	if err != nil {
		panic(err)
	}

	homeDir := currentUser.HomeDir

	resolvedString := os.ExpandEnv(dir)
	if strings.HasPrefix(resolvedString, "~/") {
		resolvedString = strings.Replace(resolvedString, "~", homeDir, 1)
	}

	updatedDir := filepath.Clean(resolvedString)

	return updatedDir
}

func DirStatus(dir string) DirStatusMsg {
	updatedDir := CleanDirPath(dir)

	if dirExists(updatedDir) {
		return DirStatusMsg{updatedDir, true, GitOrigin(updatedDir, DirIsAbsolute(updatedDir))}
	}

	return DirStatusMsg{updatedDir, false, ""}
}

func (m Model) pullRepository() tea.Cmd {
	return func() tea.Msg {
		dir := m.input.Value()
		status := DirStatus(dir) // Dir should be independently validated here

		if !status.exists {
			out, err := gitClone(m.template.Repository.URL, status.dir)
			if err != nil {
				return cloneErrorMsg{out: err.Error()}
			}

			return cloneSuccessMsg{out}
		}

		if status.repoURL == m.template.Repository.URL {
			out, err := gitPull(status.dir)
			if err != nil {
				return cloneErrorMsg{out: err.Error()}
			}

			return cloneSuccessMsg{out}
		}

		return nil
	}
}

func (m Model) validateDir() tea.Cmd {
	return func() tea.Msg {
		dir := m.input.Value()

		if status := DirStatus(dir); status.exists {
			return status
		}

		return validMsg{}
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(focusInput, m.validateDir())
}

const debounceDuration = 350 * time.Millisecond

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) { //nolint: cyclop
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "enter":
			m.input.Blur()
			m.cloning = true
			m.Dir = CleanDirPath(m.input.Value())

			return m, tea.Batch(m.validateDir(), m.pullRepository())
		}
	case focusInputMsg:
		focusCmd := m.input.Focus()
		return m, focusCmd
	case validateInputMsg:
		if m.debounceID == msg.id {
			return m, m.validateDir()
		}

		return m, nil
	case validMsg:
		m.exists = ""
		return m, focusInput
	case DirStatusMsg:
		m.repoURL = msg.repoURL

		if msg.exists {
			m.exists = msg.dir
		} else {
			m.exists = ""
		}

		return m, focusInput
	case cloneSuccessMsg:
		m.out = msg.out
		m.cloning = false
		m.finished = true

		return m, m.SuccessCmd
	case cloneErrorMsg:
		m.out = msg.out
		m.cloning = false
		m.cloneError = true

		return m, focusInput
	}

	prevValue := m.input.Value()

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)

	if prevValue != m.input.Value() {
		m.debounceID++
		tick := tea.Tick(debounceDuration, func(_ time.Time) tea.Msg {
			return validateInputMsg{m.debounceID}
		})

		return m, tea.Batch(tick, cmd)
	}

	return m, cmd
}

func (m Model) View() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Template %s\n", m.template.Name))

	if m.cloning {
		sb.WriteString("Cloning into " + m.input.Value() + "...")
		return sb.String()
	} else if m.finished {
		sb.WriteString(m.out + "\nFinished cloning into " + m.input.Value() + ".\n")
		return sb.String()
	}

	sb.WriteString("Enter destination directory\n")
	sb.WriteString(m.input.View())
	sb.WriteString("\n")

	if m.exists != "" {
		if m.repoURL == m.template.Repository.URL {
			sb.WriteString("\nDirectory '" + m.exists + "' will be pulled from origin\n")
		} else {
			sb.WriteString("\nDirectory '" + m.exists + "' contains different repository: '" + m.repoURL + "'\n")
		}
	}

	if m.cloneError {
		sb.WriteString("\nError while cloning:\n" + m.out + "\n")
	} else {
		sb.WriteString(m.out + "\n")
	}

	return sb.String()
}

func (m *Model) SetTemplate(template drapi.Template) {
	m.input = textinput.New()
	m.input.SetValue(template.DefaultDir())
	m.template = template
}
