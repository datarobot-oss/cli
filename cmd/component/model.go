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
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/internal/copier"
	"github.com/datarobot/cli/tui"
)

type (
	errMsg              struct{ err error }
	screens             int
	initiators          int
	componentsLoadedMsg struct {
		list list.Model
	}
	componentInfoRequestMsg struct {
		item ItemDelegate
	}
)

const (
	listScreen = screens(iota)
	updateScreen
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

type ItemDelegate struct {
	current   bool
	checked   bool
	component copier.Component
}

type Model struct {
	err                   error
	infoMessage           string
	screen                screens
	initialScreen         screens // TODO: I don't think we need this.
	initiator             initiators
	list                  list.Model // This list holds the components
	initialUpdateFileName string
}

// TODO: This is required by the `list` interface but not sure what we need to do here - especially since, are we filtering at all?
func (i ItemDelegate) FilterValue() string {
	if i.component.FileName != "" {
		return i.component.FileName
	}

	return i.component.FileName
}

func (i ItemDelegate) Height() int                             { return 1 }
func (i ItemDelegate) Spacing() int                            { return 0 }
func (i ItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (i ItemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(ItemDelegate)
	if !ok {
		return
	}

	checkbox := ""

	if i.checked {
		checkbox = "[x] "
	} else {
		checkbox = "[ ] "
	}

	str := fmt.Sprintf("%s%s", checkbox, i.component.FileName)

	fn := itemStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return selectedItemStyle.Render("> " + strings.Join(s, " "))
		}
	}

	fmt.Fprint(w, fn(str))
}

func (m Model) toggleCurrent() (Model, tea.Cmd) {
	items := m.list.Items()
	currentItem := items[m.list.Index()].(ItemDelegate)

	currentItem.checked = !currentItem.checked
	items[m.list.Index()] = currentItem

	cmd := m.list.SetItems(items)

	return m, cmd
}

func (m Model) getSelectedComponents() []ItemDelegate {
	items := m.list.Items()

	values := make([]ItemDelegate, 0, len(items))

	for i := range items {
		if itm := items[i].(ItemDelegate); itm.checked {
			values = append(values, itm)
		}
	}

	return values
}

func NewComponentModel(initiator initiators, initialScreen screens) Model {
	return Model{
		screen:        initialScreen,
		initialScreen: initialScreen,
		initiator:     initiator,
	}
}

func NewUpdateComponentModel(updateFileName string) Model {
	return Model{
		screen:        listScreen,
		initialScreen: listScreen,
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
			return errMsg{errors.New("No components were found.")} //nolint:revive,staticcheck
		}

		items := make([]list.Item, 0, len(components))

		for i, c := range components {
			checked := false
			if m.initialUpdateFileName != "" && c.FileName == m.initialUpdateFileName {
				checked = true
			}

			items = append(items, ItemDelegate{current: i == 0, checked: checked, component: c})
		}

		l := list.New(items, ItemDelegate{}, 0, 15)

		// Now that we've loaded components we can reset filename property since we no longer need it
		if m.initiator == updateCmd {
			m.initialUpdateFileName = ""
		}

		return componentsLoadedMsg{l}
	}
}

func (m Model) showComponentInfo() tea.Cmd {
	return func() tea.Msg {
		item := m.list.Items()[m.list.Index()]
		return componentInfoRequestMsg{item.(ItemDelegate)}
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
	}

	switch m.screen {
	case listScreen:
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
			case "i":
				// TODO: [CFX-3996] What do we show here?
				return m, m.showComponentInfo()
			case tea.KeyEscape.String(), "q":
				return m, tea.Quit
			default:
				// If we have an error allow any keypress to exit screen/quit
				if m.err != nil {
					return m, tea.Quit
				}
			}
		case errMsg:
			m.err = msg.err
			return m, nil
		}
	case updateScreen:
		// TODO: We're not actually using this update screen/view currently
		// The logic/handling below involving calling `runUpdate()` needs to be hooked up to some UX (maybe on `Enter` keypress in the list view?)
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case tea.KeyEscape.String():
				return m, tea.Quit
			case tea.KeyEnter.String():
				// TODO: We need to actually hook this up to work and to work with one or more selected components
				if len(m.getSelectedComponents()) > 0 {
					// TODO: Be sure to make this able to run for more than one selected component at a time
					for _, listItem := range m.getSelectedComponents() {
						// TODO: run copier.ExecUpdate using tea.ExecProcess here
						err := runUpdate(listItem.component.FileName)
						if err != nil {
							m.err = err
						}
					}

					return m, nil
				}
			}
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
	case updateScreen: // TODO: We're not actually using this update screen/view currently
		sb.WriteString(m.viewUpdateScreen())
	}

	return sb.String()
}

func (m Model) viewListScreen() string {
	var sb strings.Builder

	// Display error message
	if m.err != nil {
		sb.WriteString(fmt.Sprintf("%s %s\n", tui.ErrorStyle.Render("Error:"), m.err.Error()))
		sb.WriteString("\n")
		sb.WriteString(tui.DimStyle.Render("Press any key to exit"))
		sb.WriteString("\n")

		return sb.String()
	}

	sb.WriteString(tui.WelcomeStyle.Render("Available Components for Recipe Agent Template:"))
	sb.WriteString("\n\n")

	// Display status message
	if m.infoMessage != "" {
		sb.WriteString(fmt.Sprintf("%s %s\n", tui.InfoStyle.Render("Info:"), m.infoMessage))
		sb.WriteString("\n")
	}

	sb.WriteString(tui.BaseTextStyle.Render("Core Components (Select at least 1):"))
	sb.WriteString(m.list.View())
	sb.WriteString("\n\n")

	sb.WriteString(tui.BaseTextStyle.Render("Press esc to exit."))

	return sb.String()
}

func (m Model) viewComponentDetailScreen() string {
	var sb strings.Builder

	sb.WriteString(tui.WelcomeStyle.Render("Component Details"))
	sb.WriteString("\n\n")
	// TODO: [CFX-3996] What to display here
	item := m.list.Items()[m.list.Index()].(ItemDelegate)
	selectedComponent := item.component
	selectedComponentDetails := copier.ComponentDetailsMap[selectedComponent.SrcPath]

	sb.WriteString("Component file name: " + selectedComponent.FileName)
	sb.WriteString("\n\n")

	readMe, _ := glamour.Render(selectedComponentDetails.ReadMeContents, "dark")
	sb.WriteString(readMe)
	sb.WriteString("\n\n")
	sb.WriteString(tui.BaseTextStyle.Render("Press any key to return."))
	sb.WriteString("\n\n")

	return sb.String()
}

// TODO: We're not actually using this update screen/view currently
func (m Model) viewUpdateScreen() string {
	var sb strings.Builder

	// Display error message
	if m.err != nil {
		sb.WriteString(fmt.Sprintf("%s %s\n", tui.ErrorStyle.Render("Error:"), m.err.Error()))
		sb.WriteString("\n")
		sb.WriteString(tui.DimStyle.Render("Press any key to exit"))
		sb.WriteString("\n")

		return sb.String()
	}

	sb.WriteString(tui.WelcomeStyle.Render("Component Update"))
	sb.WriteString("\n\n")
	sb.WriteString(tui.BaseTextStyle.Render("Update component " + m.initialUpdateFileName + " ?"))
	sb.WriteString("\n\n")
	sb.WriteString(tui.BaseTextStyle.Render("Press enter to run update."))
	sb.WriteString("\n\n")

	return sb.String()
}
