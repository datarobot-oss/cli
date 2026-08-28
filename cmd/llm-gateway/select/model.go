// Copyright 2026 DataRobot, Inc. and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package selectcmd

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/internal/drapi"
)

type llmItem struct {
	llmID    string
	name     string
	provider string
	model    string
	kind     string
}

func (i llmItem) Title() string { return i.name }

// Description is the dim second line under each row. Deployed LLMs carry no
// provider and only the litellm sentinel model, so surface the source and the
// deployment id (what the user selects by) instead of an empty "· sentinel".
func (i llmItem) descriptionText() string {
	if i.kind == drapi.LLMKindDeployed {
		return "deployed · " + i.llmID
	}

	return i.provider + " · " + i.model
}

func (i llmItem) Description() string { return i.descriptionText() }
func (i llmItem) FilterValue() string { return i.name }

type llmItemDelegate struct{}

func (d llmItemDelegate) Height() int                             { return 2 }
func (d llmItemDelegate) Spacing() int                            { return 1 }
func (d llmItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d llmItemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(llmItem)
	if !ok {
		return
	}

	isSelected := index == m.Index()

	if isSelected {
		titleStyle := lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#6124DF", Dark: "#9D7EDF"}).
			Bold(true)

		descStyle := lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#6124DF", Dark: "#9D7EDF"}).
			Faint(true)

		fmt.Fprint(w, titleStyle.Render("▶ "+i.name))
		fmt.Fprint(w, "\n")
		fmt.Fprint(w, descStyle.Render("  "+i.descriptionText()))
	} else {
		titleStyle := lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#000000", Dark: "#FFFFFF"})

		descStyle := lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#666666", Dark: "#888888"}).
			Faint(true)

		fmt.Fprint(w, titleStyle.Render("  "+i.name))
		fmt.Fprint(w, "\n")
		fmt.Fprint(w, descStyle.Render("  "+i.descriptionText()))
	}
}

// PickerModel is the TUI model for the LLM picker.
type PickerModel struct {
	list       list.Model
	selectedID string
}

// NewPickerModel constructs the TUI picker from a slice of LLMs.
func NewPickerModel(llms []drapi.LLM) PickerModel {
	items := make([]list.Item, len(llms))

	for i, l := range llms {
		items[i] = llmItem{
			llmID:    l.LlmID,
			name:     l.Name,
			provider: l.Provider,
			model:    l.Model,
			kind:     l.Kind,
		}
	}

	delegate := llmItemDelegate{}
	l := list.New(items, delegate, 0, 0)

	l.Title = "Select Default LLM"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(true)
	l.Styles.Title = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#6124DF", Dark: "#9D7EDF"}).
		Bold(true).
		MarginLeft(2).
		MarginBottom(1)

	l.SetSize(80, 20)

	return PickerModel{list: l}
}

func (m PickerModel) Init() tea.Cmd {
	return nil
}

func (m PickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		listWidth := msg.Width - 4
		listHeight := msg.Height - 8

		if listWidth < 60 {
			listWidth = 60
		}

		if listHeight < 10 {
			listHeight = 10
		}

		m.list.SetSize(listWidth, listHeight)

		return m, nil

	case tea.KeyMsg:
		if m.list.FilterState() == list.Filtering {
			var cmd tea.Cmd

			m.list, cmd = m.list.Update(msg)

			return m, cmd
		}

		if msg.String() == "enter" {
			if item, ok := m.list.SelectedItem().(llmItem); ok {
				m.selectedID = item.llmID

				return m, tea.Quit
			}
		}
	}

	var cmd tea.Cmd

	m.list, cmd = m.list.Update(msg)

	return m, cmd
}

func (m PickerModel) View() string {
	return m.list.View()
}
