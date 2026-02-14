// Copyright 2025 DataRobot, Inc. and its affiliates.
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

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

type itemDelegate struct {
	list.DefaultDelegate
}

func (d itemDelegate) Height() int                             { return 1 }
func (d itemDelegate) Spacing() int                            { return 0 }
func (d itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

// TODO: We could try to move this abstraction (since we're using `.Title()`) to a shared, DRY internal package
// A challenge to that may be that in this file we're setting the "i" key with a specific UI string ("details/info")
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	// TO NOTE: This doesn't work as expected with filtering.
	// It seems that there's a duplicated/"shadow" list of filtered items that aren't updated with our toggleComponent.
	// An incomplete approach looked like this:
	// if m.IsFiltered() {
	// 	items := m.VisibleItems()
	// 	item = items[index].(ListItem)
	item, ok := listItem.(ListItem)
	if !ok {
		// TODO: This was taken from an official example but seems like maybe we should log something here?
		return
	}

	checkbox := ""

	if item.checked {
		checkbox = "[x] "
	} else {
		checkbox = "[ ] "
	}

	str := fmt.Sprintf("%s%s", checkbox, item.Title())

	fn := itemStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return selectedItemStyle.Render("> " + strings.Join(s, " "))
		}
	}

	fmt.Fprint(w, fn(str))
}

func newItemDelegate(keys *delegateKeyMap) itemDelegate {
	d := itemDelegate{}

	help := []key.Binding{keys.info}

	d.ShortHelpFunc = func() []key.Binding {
		return help
	}

	d.FullHelpFunc = func() [][]key.Binding {
		return [][]key.Binding{help}
	}

	return d
}

type delegateKeyMap struct {
	info key.Binding
}

// Additional (to the default) short help entries
func (d delegateKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		d.info,
	}
}

// Additional (to the default) full help entries
func (d delegateKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{
			d.info,
		},
	}
}

func newDelegateKeyMap() *delegateKeyMap {
	return &delegateKeyMap{
		info: key.NewBinding(
			key.WithKeys("i"),
			key.WithHelp("i", "details/info"),
		),
	}
}
