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

package hostpicker

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// drive feeds messages into the standalone model and returns the final state.
//
// It stands in for the bubbletea runtime: a model communicates by returning a
// tea.Cmd, so any command produced by an Update is executed and its message fed
// back in, otherwise selections (which travel as a chosenMsg) never arrive.
func drive(t *testing.T, msgs ...tea.Msg) standaloneModel {
	t.Helper()

	m := newStandaloneModel()
	m.Init()

	for _, msg := range msgs {
		m = step(t, m, msg)
	}

	return m
}

func step(t *testing.T, m standaloneModel, msg tea.Msg) standaloneModel {
	t.Helper()

	next, cmd := m.Update(msg)

	updated, ok := next.(standaloneModel)
	require.True(t, ok, "Update must return a standaloneModel")

	if cmd == nil {
		return updated
	}

	// One level of command resolution is enough for this model; it never batches.
	if produced := cmd(); produced != nil {
		return step(t, updated, produced)
	}

	return updated
}

func TestStandalone_EnterSelectsHighlightedCloud(t *testing.T) {
	// US Cloud is the first item, so Enter with no navigation picks it. That is the
	// one-keystroke path for the common case.
	m := drive(t, tea.KeyMsg{Type: tea.KeyEnter})

	assert.True(t, m.chosen, "Enter should record a choice")
	assert.Equal(t, "https://app.datarobot.com", m.url)
}

func TestStandalone_NavigateThenEnterSelectsEUCloud(t *testing.T) {
	m := drive(t,
		tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeyEnter},
	)

	assert.True(t, m.chosen)
	assert.Equal(t, "https://app.eu.datarobot.com", m.url)
}

func TestStandalone_EscInListModeCancels(t *testing.T) {
	m := drive(t, tea.KeyMsg{Type: tea.KeyEsc})

	assert.False(t, m.chosen, "Esc in list mode must cancel rather than choose")
	assert.Empty(t, m.url)
	assert.True(t, m.quitting)
}

func TestStandalone_EscInCustomModeGoesBackWithoutCancelling(t *testing.T) {
	// Esc has two meanings: leave the custom-URL field, or cancel the picker. Inside
	// the field it must only back out to the list.
	m := drive(t,
		tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeyEnter}, // opens the custom URL input
		tea.KeyMsg{Type: tea.KeyEsc},   // back to the list
	)

	assert.False(t, m.quitting, "Esc inside the custom field must not quit the picker")
	assert.False(t, m.chosen)

	// A second Esc, now back in list mode, cancels.
	final := step(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	assert.True(t, final.quitting)
	assert.False(t, final.chosen)
}

func TestStandalone_CustomURLIsReturned(t *testing.T) {
	m := drive(t,
		tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeyEnter},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("https://dr.internal.example.com")},
		tea.KeyMsg{Type: tea.KeyEnter},
	)

	assert.True(t, m.chosen)
	assert.Equal(t, "https://dr.internal.example.com", m.url)
}

func TestStandalone_ViewRendersPicker(t *testing.T) {
	// Give it a size so the list has room to render its items.
	sized := drive(t, tea.WindowSizeMsg{Width: 100, Height: 40})

	view := sized.View()

	assert.Contains(t, view, "US Cloud")
	assert.Contains(t, view, "EU Cloud")
}

func TestStandalone_ViewIsEmptyOnceQuitting(t *testing.T) {
	// Leaving the list rendered after selection would duplicate it above whatever
	// the caller prints next.
	m := drive(t, tea.KeyMsg{Type: tea.KeyEnter})

	assert.Empty(t, m.View())
}
