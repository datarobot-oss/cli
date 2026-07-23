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

package config

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/internal/workload"
)

// createNewID is the sentinel id carried by the "create a new workload" row so
// the picker can return it through the same list-item channel as real
// workloads without a second out-of-band flag.
const createNewID = "\x00create-new"

// workloadItem is one row in the picker: either an existing workload or the
// synthetic "create new" row pinned to the top of the list.
type workloadItem struct {
	id     string
	name   string
	status string
}

func (i workloadItem) Title() string {
	if i.id == createNewID {
		return "＋ Create a new workload"
	}

	return i.name
}

func (i workloadItem) Description() string {
	if i.id == createNewID {
		return "Name it now; it is created on the first `dr workload up`"
	}

	return i.status + " · " + i.id
}

func (i workloadItem) FilterValue() string {
	// A non-empty value keeps the pinned "create new" row from being silently
	// excluded when the user filters; it stays reachable by typing "create"/"new".
	if i.id == createNewID {
		return "create new workload"
	}

	return i.name
}

type workloadItemDelegate struct{}

func (d workloadItemDelegate) Height() int                             { return 2 }
func (d workloadItemDelegate) Spacing() int                            { return 1 }
func (d workloadItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d workloadItemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(workloadItem)
	if !ok {
		return
	}

	accent := lipgloss.AdaptiveColor{Light: "#6124DF", Dark: "#9D7EDF"}

	if index == m.Index() {
		titleStyle := lipgloss.NewStyle().Foreground(accent).Bold(true)
		descStyle := lipgloss.NewStyle().Foreground(accent).Faint(true)

		fmt.Fprint(w, titleStyle.Render("▶ "+i.Title()))
		fmt.Fprint(w, "\n")
		fmt.Fprint(w, descStyle.Render("  "+i.Description()))

		return
	}

	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#000000", Dark: "#FFFFFF"})
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#666666", Dark: "#888888"}).Faint(true)

	fmt.Fprint(w, titleStyle.Render("  "+i.Title()))
	fmt.Fprint(w, "\n")
	fmt.Fprint(w, descStyle.Render("  "+i.Description()))
}

// pickerModel is the TUI model that lets the user pick an existing workload or
// the "create new" row.
type pickerModel struct {
	list     list.Model
	selected *workloadItem
}

// newPickerModel builds the picker from the fetched workloads, pinning the
// "create new" row to the top.
func newPickerModel(workloads []workload.Workload) pickerModel {
	items := make([]list.Item, 0, len(workloads)+1)
	items = append(items, workloadItem{id: createNewID})

	for _, w := range workloads {
		items = append(items, workloadItem{id: w.ID, name: w.Name, status: w.Status})
	}

	delegate := workloadItemDelegate{}
	l := list.New(items, delegate, 0, 0)

	l.Title = "Select a workload"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(true)
	l.Styles.Title = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#6124DF", Dark: "#9D7EDF"}).
		Bold(true).
		MarginLeft(2).
		MarginBottom(1)

	l.SetSize(80, 20)

	return pickerModel{list: l}
}

func (m pickerModel) Init() tea.Cmd { return nil }

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		listWidth := max(msg.Width-4, 60)
		listHeight := max(msg.Height-8, 10)

		m.list.SetSize(listWidth, listHeight)

		return m, nil

	case tea.KeyMsg:
		if m.list.FilterState() == list.Filtering {
			var cmd tea.Cmd

			m.list, cmd = m.list.Update(msg)

			return m, cmd
		}

		if msg.String() == "enter" {
			if item, ok := m.list.SelectedItem().(workloadItem); ok {
				selected := item
				m.selected = &selected

				return m, tea.Quit
			}
		}
	}

	var cmd tea.Cmd

	m.list, cmd = m.list.Update(msg)

	return m, cmd
}

func (m pickerModel) View() string { return m.list.View() }
