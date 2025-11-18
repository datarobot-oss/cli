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

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
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

type Model struct {
	err                   error
	infoMessage           string
	screen                screens
	initiator             initiators
	list                  list.Model // This list holds the components
	initialUpdateFileName string
	viewport              viewport.Model
	help                  help.Model
	keys                  detailKeyMap
	ready                 bool
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
	h := help.New()
	h.Styles.ShortKey = lipgloss.NewStyle().Foreground(tui.DrPurple)
	h.Styles.ShortDesc = lipgloss.NewStyle().Foreground(tui.DimStyle.GetForeground())

	return Model{
		screen:    initialScreen,
		initiator: initiator,
		help:      h,
		keys:      newDetailKeys(),
	}
}

func NewUpdateComponentModel(updateFileName string) Model {
	h := help.New()
	h.Styles.ShortKey = lipgloss.NewStyle().Foreground(tui.DrPurple)
	h.Styles.ShortDesc = lipgloss.NewStyle().Foreground(tui.DimStyle.GetForeground())

	return Model{
		screen: listScreen,
		// Only value here we're actually setting from the function args
		initialUpdateFileName: updateFileName,
		help:                  h,
		keys:                  newDetailKeys(),
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
		m.ready = false

		return m, tea.WindowSize()
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
		case tea.WindowSizeMsg:
			// Only update list size if it's been initialized
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
		case tea.WindowSizeMsg:
			headerHeight := 4
			footerHeight := 4 // help line + status bar + spacing
			verticalMarginHeight := headerHeight + footerHeight

			if !m.ready {
				m.viewport = viewport.New(msg.Width, msg.Height-verticalMarginHeight)
				m.viewport.YPosition = headerHeight
				m.help.Width = msg.Width
				m.ready = true

				// Set the content for the viewport
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
				// Pass other keys to viewport for scrolling
				var cmd tea.Cmd

				m.viewport, cmd = m.viewport.Update(msg)

				return m, cmd
			}
		}

		// Update viewport for mouse wheel scrolling
		var cmd tea.Cmd

		m.viewport, cmd = m.viewport.Update(msg)

		return m, cmd
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

	sb.WriteString(tui.BaseTextStyle.Render("Press space to toggle component."))

	sb.WriteString("\t")
	sb.WriteString(style.Render("Press enter to run update."))

	sb.WriteString("\t")
	sb.WriteString(tui.BaseTextStyle.Render("Press esc to exit."))

	return sb.String()
}

func (m Model) getCurrentComponentFileName() string {
	if len(m.list.VisibleItems()) == 0 {
		return ""
	}

	item := m.list.VisibleItems()[m.list.Index()].(ListItem)

	return item.component.FileName
}

func (m Model) getComponentDetailContent() string {
	var sb strings.Builder

	item := m.list.VisibleItems()[m.list.Index()].(ListItem)
	selectedComponent := item.component
	selectedComponentDetails := copier.ComponentDetailsByURL[selectedComponent.SrcPath]

	style := "light"
	if lipgloss.HasDarkBackground() {
		style = "dark"
	}

	readMe, _ := glamour.Render(selectedComponentDetails.ReadMeContents, style)
	sb.WriteString(readMe)

	return sb.String()
}

func (m Model) viewComponentDetailScreen() string {
	if !m.ready {
		return "\n  Initializing..."
	}

	var sb strings.Builder

	sb.WriteString(tui.WelcomeStyle.Render("Component Details"))
	sb.WriteString("\n\n")
	sb.WriteString(m.viewport.View())
	sb.WriteString("\n\n")

	// Help text with subtle styling
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#666666", Dark: "#999999"}).
		Padding(0, 1)

	sb.WriteString(helpStyle.Render(m.help.View(m.keys)))
	sb.WriteString("\n")

	// Create status bar with three parts: left (label), center (filename), right (percentage)
	fileName := m.getCurrentComponentFileName()

	// Style the label and filename differently - filename with reduced opacity
	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFDF5"}).
		Background(lipgloss.AdaptiveColor{Light: "#6124DF", Dark: "#4A1BA8"})

	fileNameStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFDF5"}).
		Background(lipgloss.AdaptiveColor{Light: "#6124DF", Dark: "#4A1BA8"}).
		Faint(true)

	leftText := labelStyle.Render(" Component file: ") + fileNameStyle.Render(" "+fileName+" ")

	// Calculate scroll percentage - fix the calculation to be more accurate
	scrollPercent := 0

	if m.viewport.TotalLineCount() > m.viewport.Height {
		maxScroll := m.viewport.TotalLineCount() - m.viewport.Height
		if maxScroll > 0 {
			scrollPercent = int(float64(m.viewport.YOffset) / float64(maxScroll) * 100)
		}

		if m.viewport.AtBottom() {
			scrollPercent = 100
		}
	} else {
		scrollPercent = 100 // If content fits in viewport, we're at 100%
	}

	rightText := fmt.Sprintf("%d%%", scrollPercent)

	// Build status bar using the tui styles
	width := m.viewport.Width
	if width <= 0 {
		width = 80 // fallback width
	}

	statusBarStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#FFFDF5"}).
		Background(lipgloss.AdaptiveColor{Light: "#E0E0E0", Dark: "#6124DF"}).
		MarginTop(1)

	statusKeyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFDF5"}).
		Background(lipgloss.AdaptiveColor{Light: "#6124DF", Dark: "#4A1BA8"}).
		Padding(0, 1).
		Bold(true)

	centerStyle := lipgloss.NewStyle().
		Inherit(statusBarStyle).
		Padding(0, 1)

	rightRendered := statusKeyStyle.Render(rightText)

	// Calculate available space for spacing
	leftWidth := lipgloss.Width(leftText)
	rightWidth := lipgloss.Width(rightRendered)
	spacerWidth := width - leftWidth - rightWidth

	if spacerWidth < 0 {
		spacerWidth = 0
	}

	spacer := centerStyle.Width(spacerWidth).Render("")

	bar := lipgloss.JoinHorizontal(lipgloss.Top,
		leftText,
		spacer,
		rightRendered,
	)

	sb.WriteString(statusBarStyle.Width(width).Render(bar))

	return sb.String()
}
