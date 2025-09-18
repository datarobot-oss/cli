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
	prompt     envbuilder.UserPrompt
	input      textinput.Model
	list       list.Model
	Values     []string
	successCmd tea.Cmd
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

type itemDelegate struct {
	multiple bool
}

func (d itemDelegate) Height() int                             { return 1 }
func (d itemDelegate) Spacing() int                            { return 0 }
func (d itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(item)
	if !ok {
		return
	}

	checkbox := ""

	if d.multiple {
		if i.Checked {
			checkbox = "[x] "
		} else {
			checkbox = "[ ] "
		}
	}

	str := fmt.Sprintf("%s%s", checkbox, i.Name)

	fn := itemStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return selectedItemStyle.Render("> " + strings.Join(s, " "))
		}
	}

	fmt.Fprint(w, fn(str))
}

func newPromptModel(prompt envbuilder.UserPrompt, value string, successCmd tea.Cmd) (promptModel, tea.Cmd) {
	if len(prompt.Options) > 0 {
		items := make([]list.Item, 0, len(prompt.Options)+1)

		if prompt.Optional {
			items = append(items, item{Blank: true, Name: "None (leave blank)"})
		}

		for _, option := range prompt.Options {
			items = append(items, item(option))
		}

		l := list.New(items, itemDelegate{prompt.Multiple}, 0, 15)

		cmd := tea.WindowSize()
		pm := promptModel{
			prompt:     prompt,
			list:       l,
			successCmd: successCmd,
		}

		return pm, cmd
	}

	ti := textinput.New()
	ti.SetValue(value)
	cmd := ti.Focus()
	pm := promptModel{
		prompt:     prompt,
		input:      ti,
		successCmd: successCmd,
	}

	return pm, cmd
}

func (pm promptModel) GetValues() []string {
	if len(pm.prompt.Options) == 0 {
		return []string{strings.TrimSpace(pm.input.Value())}
	}

	items := pm.list.Items()
	current := items[pm.list.Index()].(item)

	if pm.prompt.Multiple {
		values := make([]string, 0, len(items))

		for i := range items {
			if itm := items[i].(item); itm.Checked {
				values = append(values, itm.FilterValue())
			}
		}

		return values
	}

	if current.Blank {
		return nil
	}

	return []string{current.FilterValue()}
}

func (pm promptModel) Update(msg tea.Msg) (promptModel, tea.Cmd) {
	if len(pm.prompt.Options) > 0 {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch keypress := msg.String(); keypress {
			case " ":
				// toggle checkbox, don't submit
				return pm.toggleCurrent()
			case "enter":
				// submit if valid
				return pm.submitList()
			}
		}

		var cmd tea.Cmd
		pm.list, cmd = pm.list.Update(msg)

		return pm, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "enter":
			return pm.submitInput()
		}
	}

	var cmd tea.Cmd
	pm.input, cmd = pm.input.Update(msg)

	return pm, cmd
}

func (pm promptModel) toggleCurrent() (promptModel, tea.Cmd) {
	items := pm.list.Items()
	currentItem := items[pm.list.Index()].(item)

	if !pm.prompt.Multiple {
		return pm, nil
	}

	if currentItem.Blank {
		for i := range items {
			itm := items[i].(item)
			itm.Checked = false
			items[i] = itm
		}
	} else {
		currentItem.Checked = !currentItem.Checked
		items[pm.list.Index()] = currentItem
	}

	cmd := pm.list.SetItems(items)

	return pm, cmd
}

func (pm promptModel) submitList() (promptModel, tea.Cmd) {
	pm.Values = pm.GetValues()

	if pm.prompt.Optional || len(pm.Values) > 0 {
		return pm, pm.successCmd
	}

	return pm, nil
}

func (pm promptModel) submitInput() (promptModel, tea.Cmd) {
	pm.Values = pm.GetValues()

	if pm.prompt.Optional || len(pm.Values[0]) > 0 {
		return pm, pm.successCmd
	}

	return pm, nil
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
