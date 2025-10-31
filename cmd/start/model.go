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
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/tui"
)

type StartModel struct {
	steps    []string
	current  int
	done     bool
	quitting bool
}

type stepCompleteMsg struct{}

var (
	checkMark = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).SetString("✓")
	arrow     = lipgloss.NewStyle().Foreground(tui.DrPurple).SetString("→")
	dimStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	boldStyle = lipgloss.NewStyle().Bold(true)
)

func NewStartModel() StartModel {
	return StartModel{
		steps: []string{
			"Starting application quickstart process...",
			"This feature is under development and will be available in a future release.",
			"Checking template prerequisites...",
			"Validating environment...",
			"Executing quickstart script...",
			"Application quickstart process completed.",
		},
		current:  0,
		done:     false,
		quitting: false,
	}
}

func (m StartModel) Init() tea.Cmd {
	return tick()
}

func tick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(_ time.Time) tea.Msg {
		return stepCompleteMsg{}
	})
}

func (m StartModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}

	case stepCompleteMsg:
		if m.current < len(m.steps)-1 {
			m.current++
			return m, tick()
		}

		m.done = true
		return m, tea.Quit
	}

	return m, nil
}

func (m StartModel) View() string {
	if m.quitting {
		return ""
	}

	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString(boldStyle.Render("DataRobot Quickstart"))
	sb.WriteString("\n\n")

	for i, step := range m.steps {
		if i < m.current {
			sb.WriteString(fmt.Sprintf("  %s %s\n", checkMark, dimStyle.Render(step)))
		} else if i == m.current {
			sb.WriteString(fmt.Sprintf("  %s %s\n", arrow, step))
		} else {
			sb.WriteString(fmt.Sprintf("    %s\n", dimStyle.Render(step)))
		}
	}

	if !m.done {
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render("  Press q or Ctrl+C to quit"))
	}

	sb.WriteString("\n")

	return sb.String()
}
