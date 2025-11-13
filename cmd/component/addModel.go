// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package component

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/internal/copier"
)

type (
	addScreens             int
	addComponentsLoadedMsg struct {
		list list.Model
	}
)

const (
	addLoadingScreen = addScreens(iota)
	addComponentsScreen
)

type AddModel struct {
	screen   addScreens
	list     list.Model
	RepoURLs []string
}

func NewAddModel() AddModel {
	return AddModel{
		screen: addLoadingScreen,
	}
}

type AddComponentDelegate struct {
	current bool
	checked bool
	details copier.Details
}

func (i AddComponentDelegate) FilterValue() string {
	return i.details.Name
}

func (i AddComponentDelegate) Height() int                             { return 1 }
func (i AddComponentDelegate) Spacing() int                            { return 0 }
func (i AddComponentDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (i AddComponentDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(AddComponentDelegate)
	if !ok {
		return
	}

	checkbox := ""

	if i.checked {
		checkbox = "[x] "
	} else {
		checkbox = "[ ] "
	}

	str := fmt.Sprintf("%s%s", checkbox, i.details.Name)

	fn := itemStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return selectedItemStyle.Render("> " + strings.Join(s, " "))
		}
	}

	fmt.Fprint(w, fn(str))
}

func (am AddModel) toggleCurrent() (AddModel, tea.Cmd) {
	items := am.list.Items()
	currentItem := items[am.list.Index()].(AddComponentDelegate)

	currentItem.checked = !currentItem.checked
	items[am.list.Index()] = currentItem

	cmd := am.list.SetItems(items)

	return am, cmd
}

func (am AddModel) getSelectedRepoURLs() []string {
	items := am.list.Items()

	values := make([]string, 0, len(items))

	for i := range items {
		if itm := items[i].(AddComponentDelegate); itm.checked {
			values = append(values, itm.details.RepoURL)
		}
	}

	return values
}

func (am AddModel) loadComponents() tea.Cmd {
	return func() tea.Msg {
		details := copier.ComponentDetails

		items := make([]list.Item, 0, len(details))

		for i, d := range details {
			if !d.Enabled {
				continue
			}

			items = append(items, AddComponentDelegate{current: i == 0, details: d})
		}

		l := list.New(items, AddComponentDelegate{}, 0, 15)

		return addComponentsLoadedMsg{l}
	}
}

func (am AddModel) Init() tea.Cmd {
	return tea.Batch(am.loadComponents(), tea.WindowSize())
}

func (am AddModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case addComponentsLoadedMsg:
		am.list = msg.list
		am.screen = addComponentsScreen

		return am, nil
	}

	switch am.screen {
	case addLoadingScreen:
		// Empty, updates handled in previous switch
	case addComponentsScreen:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case tea.KeySpace.String():
				return am.toggleCurrent()
			case tea.KeyEnter.String():
				repoURLs := am.getSelectedRepoURLs()
				if len(repoURLs) > 0 {
					am.RepoURLs = repoURLs
					return am, tea.Quit
				}
			case tea.KeyEscape.String(), "q":
				return am, tea.Quit
			}
		}

		var cmd tea.Cmd

		am.list, cmd = am.list.Update(msg)

		return am, cmd
	}

	return am, nil
}

func (am AddModel) View() string {
	var sb strings.Builder

	switch am.screen {
	case addLoadingScreen:
		sb.WriteString(am.addLoadingScreenView())
	case addComponentsScreen:
		sb.WriteString(am.addComponentsScreenView())
	}

	return sb.String()
}

func (am AddModel) addLoadingScreenView() string {
	var sb strings.Builder

	sb.WriteString("Loading components...")

	return sb.String()
}

func (am AddModel) addComponentsScreenView() string {
	var sb strings.Builder

	sb.WriteString(am.list.View())

	return sb.String()
}
