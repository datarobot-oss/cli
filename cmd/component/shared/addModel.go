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
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/internal/appframework"
	"github.com/datarobot/cli/tui"
)

type (
	addScreens             int
	addComponentsLoadedMsg struct {
		list list.Model
	}
)

var listStyle = lipgloss.NewStyle().Margin(2, 2, 1)

var errMsgID = 1

const (
	addLoadingScreen = addScreens(iota)
	addComponentsScreen
)

type AddModel struct {
	screen      addScreens
	list        list.Model
	spinner     spinner.Model
	width       int
	height      int
	errorMsg    string
	frameworkFW string // framework path captured at construction time
	ModuleNames []string
}

// NewAddModel creates the add TUI model. fw is the framework path used to load modules.
func NewAddModel(fw string) AddModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = tui.InfoStyle

	return AddModel{
		screen:      addLoadingScreen,
		spinner:     s,
		frameworkFW: fw,
	}
}

// AddComponentDelegate is a list item backed by an appframework.Module.
type AddComponentDelegate struct {
	current bool
	checked bool
	module  appframework.Module
}

func (i AddComponentDelegate) FilterValue() string {
	return strings.ToLower(i.module.DisplayName)
}

func (i AddComponentDelegate) Height() int                             { return 1 }
func (i AddComponentDelegate) Spacing() int                            { return 0 }
func (i AddComponentDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (i AddComponentDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(AddComponentDelegate)
	if !ok {
		return
	}

	checkbox := "[ ] "

	if i.checked {
		checkbox = "[x] "
	}

	str := fmt.Sprintf("%s%s", checkbox, i.module.DisplayName)

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

	if currentItem.checked && am.errorMsg != "" {
		am.errorMsg = ""
	}

	cmd := am.list.SetItems(items)

	return am, cmd
}

func (am AddModel) getSelectedModuleNames() []string {
	items := am.list.Items()

	values := make([]string, 0, len(items))

	for i := range items {
		if itm := items[i].(AddComponentDelegate); itm.checked {
			values = append(values, itm.module.DisambiguatedName)
		}
	}

	return values
}

func (am AddModel) loadComponents() tea.Cmd {
	fw := am.frameworkFW

	return func() tea.Msg {
		modules, err := appframework.DescribeFramework(fw, ".")
		if err != nil {
			// Surface a non-fatal empty list so the TUI still shows (user can cancel and re-run
			// after ensuring the framework is accessible).
			modules = []appframework.Module{}
		}

		items := make([]list.Item, 0, len(modules))
		first := true

		for _, m := range modules {
			items = append(items, AddComponentDelegate{current: first, module: m})
			first = false
		}

		l := list.New(items, AddComponentDelegate{}, 80, 25)
		l.Title = "📚 Available Components"
		l.Styles.Title = l.Styles.Title.Background(tui.DrPurple)

		return addComponentsLoadedMsg{l}
	}
}

func (am AddModel) Init() tea.Cmd {
	return tea.Batch(am.loadComponents(), am.spinner.Tick, tea.WindowSize())
}

func (am AddModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint: cyclop
	switch msg := msg.(type) {
	case tui.ClearStatusMsg:
		if msg.MsgID == errMsgID {
			am.errorMsg = ""

			return am, nil
		}
	case spinner.TickMsg:
		if am.screen == addLoadingScreen {
			var cmd tea.Cmd

			am.spinner, cmd = am.spinner.Update(msg)

			return am, cmd
		}
	case tea.WindowSizeMsg:
		am.width = msg.Width
		am.height = msg.Height

		if am.screen == addComponentsScreen {
			am.list.SetSize(
				am.width-listStyle.GetHorizontalFrameSize(),
				am.height-listStyle.GetVerticalFrameSize()-1,
			)
		}

		return am, nil
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
				moduleNames := am.getSelectedModuleNames()

				if len(moduleNames) == 0 {
					am.errorMsg = "At least one component must be selected. Please select one or more components to continue."

					return am, tui.ClearStatusAfter(3*time.Second, errMsgID)
				}

				am.errorMsg = ""
				am.ModuleNames = moduleNames

				return am, tea.Quit
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

	sb.WriteString(tui.InfoStyle.Render(am.spinner.View()+" ") + "Loading components…")

	return sb.String()
}

func (am AddModel) anySelectedComponents() bool {
	items := am.list.VisibleItems()

	for i := range items {
		if itm := items[i].(AddComponentDelegate); itm.checked {
			return true
		}
	}

	return false
}

func (am AddModel) addComponentsScreenView() string {
	var sb strings.Builder

	sb.WriteString(listStyle.Render(am.list.View()))

	style := tui.DimStyle
	if am.anySelectedComponents() {
		style = tui.BaseTextStyle
	}

	if am.errorMsg != "" {
		sb.WriteString("\n")
		sb.WriteString(tui.ErrorStyle.Render("Error: ") + am.errorMsg)
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(tui.BaseTextStyle.PaddingRight(6).Render("Press space to toggle component."))

	sb.WriteString(style.PaddingRight(6).Render("Press enter to add component."))

	sb.WriteString(tui.BaseTextStyle.Render("Press esc to exit."))

	return sb.String()
}
