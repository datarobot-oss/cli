package clone

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type Model struct {
	repoUrl  string
	input    textinput.Model
	cloning  bool
	finished bool
	out      string
}

type startCloningMsg struct{}

func StartCloningMsg() tea.Msg {
	return startCloningMsg{}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "ctrl+c":
			return m, tea.Quit

		case "enter":
			m.input.Blur()
			m.cloning = true

			return m, StartCloningMsg
		}

	case startCloningMsg:
		out, err := gitClone(m.repoUrl, m.input.Value())
		if err != nil {
			m.out = err.Error()
			return m, tea.Quit
		}

		m.out = out
		m.cloning = false
		m.finished = true

		return m, tea.Quit
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	if m.cloning {
		return "Cloning " + m.repoUrl + " into " + m.input.Value() + "..."
	} else if m.finished {
		return m.out + "\nFinished cloning into " + m.input.Value() + ".\n"
	}
	return "Enter destination directory\n" + m.input.View()
}

func NewModel(repoUrl, dir string) tea.Model {
	input := textinput.New()
	input.SetValue(dir)
	input.Focus()

	return Model{
		repoUrl: repoUrl,
		input:   input,
	}
}
