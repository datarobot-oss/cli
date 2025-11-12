// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package setup

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/tui"
)

type hostItem struct {
	title       string
	description string
	url         string
	isCustom    bool
}

func (i hostItem) Title() string       { return i.title }
func (i hostItem) Description() string { return i.description }
func (i hostItem) FilterValue() string { return i.title }

type hostItemDelegate struct{}

func (d hostItemDelegate) Height() int                             { return 2 }
func (d hostItemDelegate) Spacing() int                            { return 1 }
func (d hostItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d hostItemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(hostItem)
	if !ok {
		return
	}

	str := i.title
	desc := i.description

	isSelected := index == m.Index()

	if isSelected {
		// Selected item styling
		titleStyle := lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#6124DF", Dark: "#9D7EDF"}).
			Bold(true)

		descStyle := lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#6124DF", Dark: "#9D7EDF"}).
			Faint(true)

		fmt.Fprint(w, titleStyle.Render("▶ "+str))
		fmt.Fprint(w, "\n")
		fmt.Fprint(w, descStyle.Render("  "+desc))
	} else {
		// Normal item styling
		titleStyle := lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#000000", Dark: "#FFFFFF"})

		descStyle := lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#666666", Dark: "#888888"}).
			Faint(true)

		fmt.Fprint(w, titleStyle.Render("  "+str))
		fmt.Fprint(w, "\n")
		fmt.Fprint(w, descStyle.Render("  "+desc))
	}
}

type HostModel struct {
	list        list.Model
	customInput textinput.Model
	showCustom  bool
	width       int
	SuccessCmd  func(string) tea.Cmd
}

func NewHostModel() HostModel {
	items := []list.Item{
		hostItem{
			title:       "🇺🇸 US Cloud",
			description: "https://app.datarobot.com",
			url:         "https://app.datarobot.com",
			isCustom:    false,
		},
		hostItem{
			title:       "🇪🇺 EU Cloud",
			description: "https://app.eu.datarobot.com",
			url:         "https://app.eu.datarobot.com",
			isCustom:    false,
		},
		hostItem{
			title:       "🇯🇵 Japan Cloud",
			description: "https://app.jp.datarobot.com",
			url:         "https://app.jp.datarobot.com",
			isCustom:    false,
		},
		hostItem{
			title:       "🏢 Custom/On-Prem",
			description: "Enter your custom DataRobot URL",
			url:         "",
			isCustom:    true,
		},
	}

	delegate := hostItemDelegate{}
	l := list.New(items, delegate, 0, 0)
	l.Title = "DataRobot Environment"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(true)
	l.Styles.Title = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#6124DF", Dark: "#9D7EDF"}).
		Bold(true).
		MarginLeft(2).
		MarginBottom(1)

	// Set initial size - will be adjusted in Update based on terminal size
	l.SetSize(80, 20)

	customInput := textinput.New()
	customInput.Placeholder = "https://your-datarobot-url.com"
	customInput.CharLimit = 256
	customInput.Width = 50

	return HostModel{
		list:        l,
		customInput: customInput,
		showCustom:  false,
		width:       80,
	}
}

func (m HostModel) Init() tea.Cmd {
	return nil
}

func (m HostModel) handleCustomInput(msg tea.KeyMsg) (HostModel, tea.Cmd) {
	switch msg.String() {
	case "enter":
		url := strings.TrimSpace(m.customInput.Value())
		if url != "" && m.SuccessCmd != nil {
			return m, m.SuccessCmd(url)
		}

		return m, nil
	case "esc":
		m.showCustom = false
		m.customInput.SetValue("")
		m.customInput.Blur()

		return m, nil
	}

	var cmd tea.Cmd

	m.customInput, cmd = m.customInput.Update(msg)

	return m, cmd
}

func (m HostModel) handleListMode(msg tea.KeyMsg) (HostModel, tea.Cmd) {
	if msg.String() != "enter" {
		return m, nil
	}

	selectedItem, ok := m.list.SelectedItem().(hostItem)
	if !ok {
		return m, nil
	}

	if selectedItem.isCustom {
		// Switch to custom input mode
		m.showCustom = true
		m.customInput.Focus()

		return m, textinput.Blink
	}

	// Return selected cloud URL
	if m.SuccessCmd != nil {
		return m, m.SuccessCmd(selectedItem.url)
	}

	return m, nil
}

func (m HostModel) Update(msg tea.Msg) (HostModel, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		// Use most of the terminal width and height
		// Leave some margin for the header and status bar
		listWidth := msg.Width - 4
		listHeight := msg.Height - 8 // Account for header, status bar, and padding

		if listWidth < 60 {
			listWidth = 60
		}

		if listHeight < 15 {
			listHeight = 15
		}

		m.list.SetSize(listWidth, listHeight)

		return m, nil

	case tea.KeyMsg:
		// Handle custom input mode
		if m.showCustom {
			return m.handleCustomInput(msg)
		}

		// Handle list mode
		listResult, listCmd := m.handleListMode(msg)
		if listCmd != nil {
			return listResult, listCmd
		}

		m = listResult
	}

	// Update list
	if !m.showCustom {
		m.list, cmd = m.list.Update(msg)
	}

	return m, cmd
}

func (m HostModel) View() string {
	if m.showCustom {
		// Custom URL input view
		var sb strings.Builder

		title := lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#6124DF", Dark: "#9D7EDF"}).
			Bold(true).
			Render("🏢 Custom DataRobot URL")

		sb.WriteString(title)
		sb.WriteString("\n\n")

		instruction := tui.BaseTextStyle.Render("Enter your DataRobot URL:")
		sb.WriteString(instruction)
		sb.WriteString("\n\n")

		// Styled input frame
		inputFrame := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.AdaptiveColor{Light: "#6124DF", Dark: "#9D7EDF"}).
			Padding(0, 1).
			Width(54)

		sb.WriteString(inputFrame.Render(m.customInput.View()))
		sb.WriteString("\n\n")

		hint := tui.BaseTextStyle.
			Faint(true).
			Render("💡 Press Enter to continue or Esc to go back")

		sb.WriteString(hint)

		return sb.String()
	}

	// List view
	return m.list.View()
}
