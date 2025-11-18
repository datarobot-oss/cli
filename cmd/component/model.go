// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package component

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/internal/copier"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/viper"
)

type (
	errMsg              struct{ err error }
	screens             int
	initiators          int
	componentsLoadedMsg struct {
		list list.Model
	}
	componentInfoRequestMsg struct {
		item ListItem
	}
	updateCompleteMsg struct {
		item ListItem
	}
)

const (
	listScreen = screens(iota)
	componentDetailScreen
)

const (
	listCmd = initiators(iota)
	updateCmd
)

var (
	// TODO: Maybe move to tui?
	itemStyle         = lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(tui.DrPurple)
)

type Model struct {
	err                   error
	infoMessage           string
	screen                screens
	initiator             initiators
	list                  list.Model // This list holds the components
	initialUpdateFileName string
}

func (m Model) toggleCurrent() (Model, tea.Cmd) {
	items := m.list.VisibleItems()
	currentItem := items[m.list.Index()].(ListItem)

	currentItem.checked = !currentItem.checked

	// Use GlobalIndex() for what is the canonical, unfiltered list
	cmd := m.list.SetItem(m.list.GlobalIndex(), currentItem)

	return m, cmd
}

func updateComponent(item ListItem) tea.Cmd {
	quiet := false

	debug := viper.GetBool("debug")

	return tea.ExecProcess(copier.Update(item.component.FileName, quiet, debug), func(_ error) tea.Msg {
		return updateCompleteMsg{item}
	})
}

func (m Model) unselectComponent(itemToUnselect ListItem) (Model, tea.Cmd) {
	for i, item := range m.list.VisibleItems() {
		if item.(ListItem).component == itemToUnselect.component {
			newItem := item.(ListItem)
			newItem.checked = false

			return m, m.list.SetItem(i, newItem)
		}
	}

	return m, nil
}

func (m Model) getSelectedComponents() []ListItem {
	items := m.list.VisibleItems()

	values := make([]ListItem, 0, len(items))

	for i := range items {
		if itm := items[i].(ListItem); itm.checked {
			values = append(values, itm)
		}
	}

	return values
}

func NewComponentModel(initiator initiators, initialScreen screens) Model {
	return Model{
		screen:    initialScreen,
		initiator: initiator,
	}
}

func NewUpdateComponentModel(updateFileName string) Model {
	return Model{
		screen: listScreen,
		// Only value here we're actually setting from the function args
		initialUpdateFileName: updateFileName,
	}
}

func (m Model) Init() tea.Cmd {
	switch m.initiator {
	case listCmd, updateCmd:
		return tea.Batch(m.loadComponents(), tea.WindowSize())
	default:
		// TODO: Log here that we didn't handle a certain initiator?
		return tea.WindowSize()
	}
}

func (m Model) loadComponents() tea.Cmd {
	return func() tea.Msg {
		answers, err := copier.AnswersFromPath(".")
		if err != nil {
			return errMsg{err}
		}

		components, err := copier.ComponentsFromAnswers(answers)
		if err != nil {
			return errMsg{err}
		}

		// If we've found zero components return error message that is handled by UI
		if len(components) == 0 {
			return errMsg{errors.New("No components were found.")}
		}

		items := make([]list.Item, 0, len(components))

		for i, c := range components {
			checked := false
			if m.initialUpdateFileName != "" && c.FileName == m.initialUpdateFileName {
				checked = true
			}

			items = append(items, ListItem{current: i == 0, checked: checked, component: c})
		}

		delegateKeys := newDelegateKeyMap()
		delegate := newItemDelegate(delegateKeys)
		l := list.New(items, delegate, 0, 15)

		// TODO: The actual filtering works but there's bugs/unresolved issues w/ our custom Render() as well as toggleComponent()
		l.SetFilteringEnabled(false)

		// Now that we've loaded components we can reset filename property since we no longer need it
		if m.initiator == updateCmd {
			m.initialUpdateFileName = ""
		}

		return componentsLoadedMsg{l}
	}
}

func (m Model) showComponentInfo() tea.Cmd {
	return func() tea.Msg {
		item := m.list.VisibleItems()[m.list.Index()]
		return componentInfoRequestMsg{item.(ListItem)}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:cyclop
	switch msg := msg.(type) {
	case componentsLoadedMsg:
		m.list = msg.list

		if m.initiator == updateCmd && m.initialUpdateFileName == "" {
			m.infoMessage = "Please use an 'answer file' value with 'dr component update <answer_file>'."
		}
	case componentInfoRequestMsg:
		m.screen = componentDetailScreen
	case updateCompleteMsg:
		return m.unselectComponent(msg.item)
	}

	switch m.screen {
	case listScreen:
		// IMPT: Since we're using a custom item & respective delegate
		// we need to account for filtering here and allow list to handle updating
		if m.list.FilterState() == list.Filtering {
			newListModel, cmd := m.list.Update(msg)
			m.list = newListModel

			return m, cmd
		}

		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case tea.KeySpace.String():
				return m.toggleCurrent()
			case "k", tea.KeyUp.String():
				// If we're at the top of list go to the bottom (accounting for pagination as well)
				if m.list.Cursor() == 0 && m.list.Paginator.OnFirstPage() {
					for range len(m.list.Items()) {
						m.list.CursorDown()
					}
				} else {
					m.list.CursorUp()
				}

				return m, nil
			case "j", tea.KeyDown.String():
				// If we're already at end of list go back to the beginning (accounting for pagination)
				itemsLength := len(m.list.Items())
				if m.list.Cursor() == m.list.Paginator.ItemsOnPage(itemsLength)-1 && m.list.Paginator.OnLastPage() {
					for range itemsLength {
						m.list.CursorUp()
					}
				} else {
					m.list.CursorDown()
				}

				return m, nil
			case "i":
				// TODO: [CFX-3996] What do we show here?
				return m, m.showComponentInfo()
			case tea.KeyEnter.String():
				if len(m.getSelectedComponents()) > 0 {
					var cmdsToRun []tea.Cmd

					for _, listItem := range m.getSelectedComponents() {
						cmdsToRun = append(cmdsToRun, updateComponent(listItem))
					}

					cmd := tea.Sequence(cmdsToRun...)

					return m, cmd
				}
			default:
				// If we have an error allow any keypress to exit screen/quit
				if m.err != nil {
					return m, tea.Quit
				}
			}

			var cmd tea.Cmd

			// Be sure to call list's Update method - to note, we're overriding the up/down keys
			m.list, cmd = m.list.Update(msg)

			return m, cmd
		case errMsg:
			m.err = msg.err
			return m, nil
		}
	case componentDetailScreen:
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			default:
				m.screen = listScreen
			}
		}
	}

	return m, nil
}

func (m Model) View() string {
	var sb strings.Builder

	switch m.screen {
	case listScreen:
		sb.WriteString(m.viewListScreen())
	case componentDetailScreen:
		sb.WriteString(m.viewComponentDetailScreen())
	}

	return sb.String()
}

func (m Model) viewListScreen() string {
	var sb strings.Builder

	// Display error message
	if m.err != nil {
		sb.WriteString(fmt.Sprintf("%s %s\n", tui.ErrorStyle.Render("Error: "), m.err.Error()))
		sb.WriteString("\n")
		sb.WriteString(tui.DimStyle.Render("Press any key to exit"))
		sb.WriteString("\n")

		return sb.String()
	}

	sb.WriteString(tui.WelcomeStyle.Render("Available Components for Recipe Agent Template:"))
	sb.WriteString("\n\n")

	// Display status message
	if m.infoMessage != "" {
		sb.WriteString(fmt.Sprintf("%s %s\n", tui.InfoStyle.Render("Info: "), m.infoMessage))
		sb.WriteString("\n")
	}

	sb.WriteString(tui.BaseTextStyle.Render("Core Components (Select at least 1):"))
	sb.WriteString(m.list.View())
	sb.WriteString("\n\n")

	// If we don't have any components selected then grey out the message
	style := tui.DimStyle
	if len(m.getSelectedComponents()) > 0 {
		style = tui.BaseTextStyle
	}

	sb.WriteString(style.Render("Press enter to run update."))

	sb.WriteString("\t")
	sb.WriteString(tui.BaseTextStyle.Render("Press esc to exit."))

	return sb.String()
}

func (m Model) viewComponentDetailScreen() string {
	var sb strings.Builder

	sb.WriteString(tui.WelcomeStyle.Render("Component Details"))
	sb.WriteString("\n\n")
	// TODO: [CFX-3996] What to display here
	item := m.list.VisibleItems()[m.list.Index()].(ListItem)
	selectedComponent := item.component
	selectedComponentDetails := copier.ComponentDetailsByURL[selectedComponent.SrcPath]

	sb.WriteString("Component file name: " + selectedComponent.FileName)
	sb.WriteString("\n\n")

	style := "light"
	if lipgloss.HasDarkBackground() {
		style = "dark"
	}

	readMe, _ := glamour.Render(selectedComponentDetails.ReadMeContents, style)
	sb.WriteString(readMe)
	sb.WriteString("\n\n")
	sb.WriteString(tui.BaseTextStyle.Render("Press any key to return."))
	sb.WriteString("\n\n")

	return sb.String()
}
