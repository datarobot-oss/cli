// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package list

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/tui"
)

var docStyle = lipgloss.NewStyle().Margin(1, 2)

type Model struct {
	list       list.Model
	Template   drapi.Template
	SuccessCmd tea.Cmd
}

func (m Model) Init() tea.Cmd {
	return tea.WindowSize()
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "enter":
			if t, ok := m.list.SelectedItem().(drapi.Template); ok {
				if t.Repository.URL != "" {
					m.Template = t
					return m, m.SuccessCmd
				}
			}

			return m, nil
		}

	case tea.WindowSizeMsg:
		if len(m.list.Items()) > 0 {
			h, v := docStyle.GetFrameSize()
			m.list.SetSize(msg.Width-h, msg.Height-v)
		}

		return m, nil
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)

	return m, cmd
}

func (m Model) View() string {
	return docStyle.Render(m.list.View())
}

func (m *Model) SetTemplates(templates []drapi.Template) {
	items := make([]list.Item, len(templates))
	for i, t := range templates {
		items[i] = t
	}

	m.list = NewList("Application Templates", items)
}

var (
	boldStyle     = lipgloss.NewStyle().Bold(true)
	baseStyle     = lipgloss.NewStyle().Height(6).Margin(1, 0)
	selectedStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, false, true).
			Foreground(tui.DrPurple).BorderForeground(tui.DrPurple)

	itemStyle         = baseStyle.PaddingLeft(2)
	selectedItemStyle = baseStyle.PaddingLeft(1).MarginRight(1).Inherit(selectedStyle)
)

type itemDelegate struct{}

func NewList(title string, items []list.Item) list.Model {
	nl := list.New(items, itemDelegate{}, 0, 0)
	nl.Title = title
	nl.Styles.Title = nl.Styles.Title.Background(tui.DrPurple)

	return nl
}

func (d itemDelegate) Height() int                             { return 8 }
func (d itemDelegate) Spacing() int                            { return 0 }
func (d itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	li, ok := listItem.(drapi.Template)
	if !ok {
		return
	}

	var sb strings.Builder

	url := li.Repository.URL
	if url == "" {
		url = "Template without git repository"
	}

	title := fmt.Sprintf("%-30s  %s", li.Name, url)
	sb.WriteString(boldStyle.Render(title))
	sb.WriteString("\n")

	style := itemStyle
	if index == m.Index() {
		style = selectedItemStyle
	}

	if li.Repository.URL == "" {
		style = style.UnsetForeground()
	}

	sb.WriteString(li.Description)

	fmt.Fprint(w, style.Width(m.Width()-style.GetHorizontalFrameSize()).Render(sb.String()))
}
