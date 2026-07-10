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
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sendKey calls Update with a KeyMsg and returns the resulting model and cmd.
func sendKey(m PickerModel, key tea.KeyType) (PickerModel, tea.Cmd) {
	next, cmd := m.Update(tea.KeyMsg{Type: key})
	return next.(PickerModel), cmd
}

// sendRune calls Update with a rune KeyMsg (e.g. '/' to start filtering).
func sendRune(m PickerModel, r rune) (PickerModel, tea.Cmd) {
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	return next.(PickerModel), cmd
}

// isQuit invokes cmd and reports whether it produced a tea.QuitMsg.
func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}

	_, ok := cmd().(tea.QuitMsg)

	return ok
}

// --- PickerModel.Update ---

func TestPickerModel_EnterSelectsItem(t *testing.T) {
	m := NewPickerModel(testLLMs)

	next, cmd := sendKey(m, tea.KeyEnter)

	assert.Equal(t, testLLMs[0].LlmID, next.selectedID)
	assert.True(t, isQuit(cmd), "expected tea.Quit after Enter")
}

func TestPickerModel_EnterWhileFiltering_DoesNotSelect(t *testing.T) {
	m := NewPickerModel(testLLMs)

	// '/' activates filtering in bubbles/list
	m, _ = sendRune(m, '/')
	require.Equal(t, list.Filtering, m.list.FilterState(), "list should be in filtering state")

	// Enter while filtering should confirm the filter, not select an item
	next, cmd := sendKey(m, tea.KeyEnter)

	assert.Empty(t, next.selectedID, "selectedID must remain empty when Enter confirms a filter")
	assert.False(t, isQuit(cmd), "Enter during filtering must not quit")
}

func TestPickerModel_WindowSizeMsg(t *testing.T) {
	m := NewPickerModel(testLLMs)

	next, cmd := m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	picker := next.(PickerModel)

	assert.Nil(t, cmd)
	// List width = 200-4 = 196, height = 50-8 = 42
	w, h := picker.list.Width(), picker.list.Height()
	assert.Equal(t, 196, w)
	assert.Equal(t, 42, h)
}

func TestPickerModel_WindowSizeMsg_MinimumClamp(t *testing.T) {
	m := NewPickerModel(testLLMs)

	next, _ := m.Update(tea.WindowSizeMsg{Width: 10, Height: 5})
	picker := next.(PickerModel)

	w, h := picker.list.Width(), picker.list.Height()
	assert.Equal(t, 60, w, "width should clamp to 60")
	assert.Equal(t, 10, h, "height should clamp to 10")
}
