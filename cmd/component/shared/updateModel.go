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

package shared

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/internal/appframework"
	"github.com/datarobot/cli/tui"
)

type (
	errMsg              struct{ err error }
	screens             int
	componentsLoadedMsg struct {
		list list.Model
	}
	componentInfoRequestMsg struct {
		item ListItem
	}
	updateCompleteMsg struct {
		item ListItem
		err  error
	}
)

const (
	listScreen = screens(iota)
	componentDetailScreen
)

// TODO: Maybe move to tui?
var (
	itemStyle         = lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(tui.DrPurple)
)

type detailKeyMap struct {
	Up       key.Binding
	Down     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Back     key.Binding
}

func (k detailKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.PageUp, k.PageDown, k.Back}
}

func (k detailKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown},
		{k.Back},
	}
}

func newDetailKeys() detailKeyMap {
	return detailKeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("pgup", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown"),
			key.WithHelp("pgdn", "page down"),
		),
		Back: key.NewBinding(
			key.WithKeys("q", "esc"),
			key.WithHelp("q/esc", "back"),
		),
	}
}

// UpdateModel is the Bubble Tea model for the component update TUI.
type UpdateModel struct {
	err         error
	infoMessage string
	screen      screens
	list        list.Model
	width       int
	viewport    viewport.Model
	help        help.Model
	keys        detailKeyMap
	spinner     spinner.Model
	updating    bool
	ready       bool
	ExitMessage string
	dataArgs    []string // forwarded to ExecAnswer before ExecUpdate
	dataFile    string
	frameworkFW string

	ComponentUpdated bool
}

func (m UpdateModel) toggleCurrent() (UpdateModel, tea.Cmd) {
	items := m.list.VisibleItems()
	currentItem := items[m.list.Index()].(ListItem)

	currentItem.checked = !currentItem.checked

	cmd := m.list.SetItem(m.list.GlobalIndex(), currentItem)

	return m, cmd
}

// updateComponent runs the AF update for a single label using tea.ExecProcess so
// Bubble Tea pauses the TUI for the interactive subprocess then resumes.
// Note: --data pre-answering is skipped in TUI mode (the three-way merge handles it).
func updateComponent(item ListItem, fw string, _ string, _ []string) tea.Cmd {
	label := item.instance.Label
	command := appframework.UpdateCmd([]string{label}, fw, ".", false)

	return tea.ExecProcess(command, func(err error) tea.Msg {
		return updateCompleteMsg{item, err}
	})
}

func (m UpdateModel) unselectComponent(itemToUnselect ListItem, err error) (UpdateModel, tea.Cmd) {
	label := itemToUnselect.instance.Label

	if err != nil {
		m.ExitMessage += fmt.Sprintf(
			"Update of %q component finished with error: %s.",
			label, err,
		)
	} else {
		m.ExitMessage += fmt.Sprintf(
			"Update of %q component finished successfully.",
			label,
		)
		m.ComponentUpdated = true
	}

	count := 0

	for _, item := range m.list.VisibleItems() {
		if item.(ListItem).checked {
			count += 1
		}
	}

	if count <= 1 {
		m.updating = false

		return m, tea.Quit
	}

	for i, item := range m.list.VisibleItems() {
		if item.(ListItem).instance.Label == itemToUnselect.instance.Label {
			newItem := item.(ListItem)
			newItem.checked = false

			return m, m.list.SetItem(i, newItem)
		}
	}

	return m, nil
}

func (m UpdateModel) getSelectedComponents() []ListItem {
	items := m.list.VisibleItems()

	values := make([]ListItem, 0, len(items))

	for i := range items {
		if itm := items[i].(ListItem); itm.checked {
			values = append(values, itm)
		}
	}

	return values
}

// NewUpdateComponentModel creates the update TUI model.
// dataArgs and dataFile are forwarded to ExecAnswer before each update.
func NewUpdateComponentModel(dataArgs []string, dataFile string) UpdateModel {
	h := help.New()
	h.Styles.ShortKey = lipgloss.NewStyle().Foreground(tui.DrPurple)
	h.Styles.ShortDesc = lipgloss.NewStyle().Foreground(tui.DimStyle.GetForeground())

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = tui.InfoStyle

	return UpdateModel{
		screen:      listScreen,
		help:        h,
		keys:        newDetailKeys(),
		dataArgs:    dataArgs,
		dataFile:    dataFile,
		frameworkFW: GetFrameworkPath(),
		spinner:     s,
	}
}

func (m UpdateModel) Init() tea.Cmd {
	return tea.Batch(m.loadComponents(), m.spinner.Tick, tea.WindowSize())
}

func (m UpdateModel) loadComponents() tea.Cmd {
	fw := m.frameworkFW

	return func() tea.Msg {
		instances, err := appframework.ListInstalled(fw, ".")
		if err != nil {
			return errMsg{err}
		}

		if len(instances) == 0 {
			return errMsg{errors.New("No components were found.")}
		}

		items := make([]list.Item, 0, len(instances))

		for i, c := range instances {
			items = append(items, ListItem{current: i == 0, instance: c})
		}

		delegateKeys := newDelegateKeyMap()
		delegate := newItemDelegate(delegateKeys)
		l := list.New(items, delegate, 0, 15)

		// TODO: The actual filtering works but there's bugs/unresolved issues w/ our custom Render() as well as toggleComponent()
		l.SetFilteringEnabled(false)

		return componentsLoadedMsg{l}
	}
}

func (m UpdateModel) showComponentInfo() tea.Cmd {
	return func() tea.Msg {
		item := m.list.VisibleItems()[m.list.Index()]

		return componentInfoRequestMsg{item.(ListItem)}
	}
}

func (m UpdateModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:cyclop
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width

	case spinner.TickMsg:
		if len(m.list.Items()) == 0 || m.updating {
			var cmd tea.Cmd

			m.spinner, cmd = m.spinner.Update(msg)

			return m, cmd
		}
	case componentsLoadedMsg:
		m.list = msg.list

		return m, nil
	case componentInfoRequestMsg:
		m.screen = componentDetailScreen
		m.ready = false

		return m, tea.WindowSize()
	case updateCompleteMsg:
		if msg.err != nil {
			m.infoMessage = "Failed to update " + msg.item.instance.Label
		} else {
			m.infoMessage = "Updated " + msg.item.instance.Label
		}

		return m.unselectComponent(msg.item, msg.err)
	}

	switch m.screen {
	case listScreen:
		if m.updating {
			switch msg := msg.(type) {
			case tea.WindowSizeMsg:
				if len(m.list.Items()) > 0 {
					newListModel, cmd := m.list.Update(msg)
					m.list = newListModel

					return m, cmd
				}

				return m, nil
			}

			return m, nil
		}

		if m.list.FilterState() == list.Filtering {
			newListModel, cmd := m.list.Update(msg)
			m.list = newListModel

			return m, cmd
		}

		switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			if len(m.list.Items()) > 0 {
				newListModel, cmd := m.list.Update(msg)
				m.list = newListModel

				return m, cmd
			}

			return m, nil
		case tea.KeyMsg:
			switch msg.String() {
			case tea.KeySpace.String():
				return m.toggleCurrent()
			case "k", tea.KeyUp.String():
				if m.list.Cursor() == 0 && m.list.Paginator.OnFirstPage() {
					for range len(m.list.Items()) {
						m.list.CursorDown()
					}
				} else {
					m.list.CursorUp()
				}

				return m, nil
			case "j", tea.KeyDown.String():
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
				return m, m.showComponentInfo()
			case tea.KeyEnter.String():
				if len(m.getSelectedComponents()) > 0 {
					var cmdsToRun []tea.Cmd

					fw := m.frameworkFW

					for _, listItem := range m.getSelectedComponents() {
						cmdsToRun = append(cmdsToRun, updateComponent(listItem, fw, m.dataFile, m.dataArgs))
					}

					m.updating = true
					m.infoMessage = "Updating selected components..."

					cmd := tea.Sequence(cmdsToRun...)

					return m, tea.Batch(m.spinner.Tick, cmd)
				}
			default:
				if m.err != nil {
					return m, tea.Quit
				}
			}

			var cmd tea.Cmd

			m.list, cmd = m.list.Update(msg)

			return m, cmd
		case errMsg:
			m.err = msg.err

			return m, nil
		}
	case componentDetailScreen:
		switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			headerHeight := 4
			footerHeight := 4
			verticalMarginHeight := headerHeight + footerHeight

			if !m.ready {
				m.viewport = viewport.New(msg.Width, msg.Height-verticalMarginHeight)
				m.viewport.YPosition = headerHeight
				m.help.Width = msg.Width
				m.ready = true

				m.viewport.SetContent(m.getComponentDetailContent())
			} else {
				m.viewport.Width = msg.Width
				m.viewport.Height = msg.Height - verticalMarginHeight
				m.help.Width = msg.Width
			}
		case tea.KeyMsg:
			switch msg.String() {
			case "q", tea.KeyEscape.String():
				m.screen = listScreen
				m.ready = false

				return m, nil
			case tea.KeyPgUp.String():
				m.viewport.PageUp()

				return m, nil
			case tea.KeyPgDown.String():
				m.viewport.PageDown()

				return m, nil
			default:
				var cmd tea.Cmd

				m.viewport, cmd = m.viewport.Update(msg)

				return m, cmd
			}
		}

		var cmd tea.Cmd

		m.viewport, cmd = m.viewport.Update(msg)

		return m, cmd
	}

	return m, nil
}

func (m UpdateModel) View() string {
	var sb strings.Builder

	switch m.screen {
	case listScreen:
		sb.WriteString(m.viewListScreen())
	case componentDetailScreen:
		sb.WriteString(m.viewComponentDetailScreen())
	}

	return sb.String()
}

func (m UpdateModel) viewListScreen() string {
	var sb strings.Builder

	if m.err != nil {
		fmt.Fprintf(&sb, "%s %s\n", tui.ErrorStyle.Render("Error: "), m.err.Error())
		sb.WriteString("\n")
		sb.WriteString(tui.DimStyle.Render("Press any key to exit"))
		sb.WriteString("\n")

		return sb.String()
	}

	if len(m.list.Items()) == 0 {
		sb.WriteString(tui.InfoStyle.Render(m.spinner.View()+" ") + "Loading components…")

		return sb.String()
	}

	if m.updating {
		sb.WriteString(tui.WelcomeStyle.Render("Available Components for Recipe Agent Template:"))
		sb.WriteString("\n\n")
		sb.WriteString(tui.RenderStatusBar(m.width, m.spinner, "Updating selected components...", true))

		return sb.String()
	}

	sb.WriteString(tui.WelcomeStyle.Render("Available Components for Recipe Agent Template:"))
	sb.WriteString("\n\n")

	if m.infoMessage != "" {
		fmt.Fprintf(&sb, "%s %s\n", tui.InfoStyle.Render("Info: "), m.infoMessage)
		sb.WriteString("\n")
	}

	sb.WriteString(tui.BaseTextStyle.Render("Core Components (Select at least 1):"))
	sb.WriteString(m.list.View())
	sb.WriteString("\n\n")

	style := tui.DimStyle
	if len(m.getSelectedComponents()) > 0 {
		style = tui.BaseTextStyle
	}

	sb.WriteString(tui.BaseTextStyle.PaddingRight(6).Render("Press space to toggle component."))

	sb.WriteString(style.PaddingRight(6).Render("Press enter to run update."))

	sb.WriteString(tui.BaseTextStyle.Render("Press esc to exit."))

	return sb.String()
}
