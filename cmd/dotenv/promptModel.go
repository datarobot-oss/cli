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
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/internal/envbuilder"
	"github.com/datarobot/cli/tui"
)

type promptModel struct {
	prompt envbuilder.UserPrompt
	input  textinput.Model
	list   list.Model
}

var (
	itemStyle         = lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("170"))
)

type item envbuilder.PromptOption

func (i item) FilterValue() string {
	if i.Value != "" {
		return i.Value
	}

	return i.Name
}

type itemDelegate struct{}

func (d itemDelegate) Height() int                             { return 1 }
func (d itemDelegate) Spacing() int                            { return 0 }
func (d itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(item)
	if !ok {
		return
	}

	str := fmt.Sprintf("%d. %s", index+1, i.Name)

	fn := itemStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return selectedItemStyle.Render("> " + strings.Join(s, " "))
		}
	}

	fmt.Fprint(w, fn(str))
}

func newPromptModel(prompt envbuilder.UserPrompt) promptModel {
	if len(prompt.Options) > 0 {
		items := make([]list.Item, 0, len(prompt.Options)+1)

		if prompt.Optional {
			items = append(items, item{Blank: true, Name: "None (leave blank)"})
		}

		for _, option := range prompt.Options {
			items = append(items, item(option))
		}

		l := list.New(items, itemDelegate{}, 0, 15)

		return promptModel{
			prompt: prompt,
			list:   l,
		}
	}

	ti := textinput.New()

	return promptModel{
		prompt: prompt,
		input:  ti,
	}
}

func (pm promptModel) Value() string {
	if len(pm.prompt.Options) == 0 {
		return strings.TrimSpace(pm.input.Value())
	}

	current := pm.list.Items()[pm.list.Index()].(item)

	if current.Blank {
		return ""
	}

	return current.FilterValue()
}

func (pm promptModel) Update(msg tea.Msg) (promptModel, tea.Cmd) {
	var cmd tea.Cmd

	if len(pm.prompt.Options) > 0 {
		pm.list, cmd = pm.list.Update(msg)
	} else {
		pm.input, cmd = pm.input.Update(msg)
	}

	return pm, cmd
}

func (pm promptModel) View() string {
	var sb strings.Builder

	sb.WriteString("\n\n")
	sb.WriteString(tui.BaseTextStyle.Render(pm.prompt.Help))
	sb.WriteString("\n")

	if len(pm.prompt.Options) > 0 {
		sb.WriteString(pm.list.View())
	} else {
		sb.WriteString(pm.input.View())
	}

	sb.WriteString("\n")

	if pm.prompt.Default != "" && pm.prompt.Default != nil {
		sb.WriteString(tui.BaseTextStyle.Render(fmt.Sprintf("Default: %v", pm.prompt.Default)))
		sb.WriteString("\n")
	}

	return sb.String()
}
