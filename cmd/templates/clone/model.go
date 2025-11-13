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

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/tui"
)

type keyMap struct {
	Enter key.Binding
	Back  key.Binding
	Quit  key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Enter, k.Back, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Enter, k.Back, k.Quit},
	}
}

type Model struct {
	template       drapi.Template
	directoryInput textinput.Model
	spinner        spinner.Model
	help           help.Model
	keys           keyMap
	debounceID     int
	cloning        bool
	exists         string
	repoURL        string
	cloneError     bool
	finished       bool
	out            string
	Dir            string
	width          int
	SuccessCmd     tea.Cmd
	BackCmd        tea.Cmd
}

type (
	focusInputMsg    struct{}
	validateInputMsg struct{ id int }
	validMsg         struct{}
	backMsg          struct{}
	dirStatusMsg     struct {
		dir     string
		exists  bool
		repoURL string
	}
	cloneSuccessMsg struct{ out string }
	cloneErrorMsg   struct{ out string }
)

func focusInput() tea.Msg { return focusInputMsg{} }
func back() tea.Msg       { return backMsg{} }

func dirExists(dir string) bool {
	_, err := os.Stat(dir)
	return !os.IsNotExist(err)
}

func dirIsAbsolute(dir string) bool {
	return filepath.IsAbs(dir)
}

func cleanDirPath(dir string) string {
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

func dirStatus(dir string) dirStatusMsg {
	updatedDir := cleanDirPath(dir)

	if dirExists(updatedDir) {
		return dirStatusMsg{updatedDir, true, gitOrigin(updatedDir, dirIsAbsolute(updatedDir))}
	}

	return dirStatusMsg{updatedDir, false, ""}
}

func (m Model) pullRepository() tea.Cmd {
	return func() tea.Msg {
		dir := m.directoryInput.Value()
		status := dirStatus(dir) // Dir should be independently validated here

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

		return cloneErrorMsg{out: fmt.Sprintf("directory '%s' already exists with a different repository", status.dir)}
	}
}

func (m Model) validateDir() tea.Cmd {
	return func() tea.Msg {
		dir := m.directoryInput.Value()

		if status := dirStatus(dir); status.exists {
			return status
		}

		return validMsg{}
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, focusInput, m.validateDir())
}

const debounceDuration = 350 * time.Millisecond

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) { //nolint: cyclop
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.help.Width = msg.Width
	case spinner.TickMsg:
		var cmd tea.Cmd

		m.spinner, cmd = m.spinner.Update(msg)

		return m, cmd
	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "enter":
			if m.directoryInput.Value() == "" {
				return m, nil
			}

			m.directoryInput.Blur()
			m.cloning = true
			m.Dir = cleanDirPath(m.directoryInput.Value())

			return m, tea.Batch(m.validateDir(), m.pullRepository())
		case "esc":
			if !m.cloning && !m.finished {
				return m, back
			}
		}
	case backMsg:
		return m, m.BackCmd
	case focusInputMsg:
		focusCmd := m.directoryInput.Focus()
		return m, focusCmd
	case validateInputMsg:
		if m.debounceID == msg.id {
			return m, m.validateDir()
		}

		return m, nil
	case validMsg:
		m.exists = ""
		return m, focusInput
	case dirStatusMsg:
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

	prevValue := m.directoryInput.Value()

	var cmd tea.Cmd

	m.directoryInput, cmd = m.directoryInput.Update(msg)

	if prevValue != m.directoryInput.Value() {
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

	// Title
	title := tui.BaseTextStyle.
		Bold(true).
		Render("📦 Clone Template: " + m.template.Name)

	sb.WriteString(title)
	sb.WriteString("\n\n")

	if m.cloning {
		// Show cloning progress
		message := lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#6124DF", Dark: "#9D7EDF"}).
			Render(fmt.Sprintf("Cloning into %s...", m.directoryInput.Value()))

		sb.WriteString(message)

		return sb.String()
	} else if m.finished {
		sb.WriteString(m.out)
		sb.WriteString("\n")
		sb.WriteString(tui.SubTitleStyle.Render(fmt.Sprintf("🎉 Template %s cloned.", m.template.Name)))
		sb.WriteString("\n")
		sb.WriteString(tui.BaseTextStyle.Render("To navigate to the project directory, use the following command:"))
		sb.WriteString("\n\n")
		sb.WriteString(tui.BaseTextStyle.Render("cd " + m.Dir))
		sb.WriteString("\n\n")
		sb.WriteString(tui.BaseTextStyle.Render("afterward get started with: "))
		sb.WriteString(tui.InfoStyle.Render("dr start"))
		sb.WriteString("\n")

		return sb.String()
	}

	// Instruction
	instruction := tui.BaseTextStyle.
		Render("Enter the destination directory for your project:")

	sb.WriteString(instruction)
	sb.WriteString("\n\n")

	// Input field with styled frame
	inputStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.AdaptiveColor{Light: "#6124DF", Dark: "#9D7EDF"}).
		Padding(0, 1).
		Width(60)

	styledInput := inputStyle.Render(m.directoryInput.View())
	sb.WriteString(styledInput)
	sb.WriteString("\n")

	// Status messages
	if m.exists != "" {
		sb.WriteString("\n")

		var statusMsg string
		if m.repoURL == m.template.Repository.URL {
			statusMsg = tui.InfoStyle.Render(fmt.Sprintf("💡 Directory '%s' exists and will be updated from origin", m.exists))
		} else {
			statusMsg = tui.ErrorStyle.Render(fmt.Sprintf("⚠️ Directory '%s' contains a different repository: '%s'", m.exists, m.repoURL))
		}

		sb.WriteString(statusMsg)
		sb.WriteString("\n")
	}

	if m.cloneError {
		sb.WriteString("\n")

		errorMsg := lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#D32F2F", Dark: "#EF5350"}).
			Render("❌ Error: " + m.out)

		sb.WriteString(errorMsg)
		sb.WriteString("\n")
		sb.WriteString(tui.BaseTextStyle.Faint(true).Render("Please choose a different directory name."))
		sb.WriteString("\n")
	}

	// Help section
	sb.WriteString("\n")

	helpView := m.help.View(m.keys)
	sb.WriteString(helpView)
	sb.WriteString("\n")

	// Status bar
	sb.WriteString("\n")
	sb.WriteString(tui.RenderStatusBar(m.width, m.spinner, "Enter directory name and press Enter to clone", false))

	return sb.String()
}

// IsCloning returns whether the repository is currently being cloned
func (m Model) IsCloning() bool {
	return m.cloning
}

func (m *Model) SetTemplate(template drapi.Template) {
	m.directoryInput = textinput.New()
	m.directoryInput.SetValue(template.DefaultDir())
	m.directoryInput.Placeholder = "e.g., ~/projects/my-ai-app"
	m.directoryInput.CharLimit = 256
	m.template = template

	m.spinner = spinner.New()
	m.spinner.Spinner = spinner.Dot
	m.spinner.Style = tui.InfoStyle

	m.help = help.New()
	m.help.ShowAll = false

	m.keys = keyMap{
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "clone"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back to templates"),
		),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),
	}

	m.width = 80
}
