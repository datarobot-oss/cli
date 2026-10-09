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

package tui

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// quietModel is a model that does nothing, so these exercise the wrapper.
type quietModel struct{}

func (quietModel) Init() tea.Cmd                         { return nil }
func (m quietModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func (quietModel) View() string                          { return "" }

func TestWasInterrupted_TrueOnlyAfterCtrlC(t *testing.T) {
	m := NewInterruptibleModel(quietModel{})

	assert.False(t, WasInterrupted(m), "a model that has seen nothing was not interrupted")

	// Any other key leaves it alone; only Ctrl-C counts.
	after, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	assert.False(t, WasInterrupted(after))

	interrupted, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	assert.True(t, WasInterrupted(interrupted),
		"Ctrl-C has to be recorded: Bubble Tea ends the program with a nil error either way")
	require.NotNil(t, cmd, "Ctrl-C still quits")
}

// WasInterrupted reads the outermost wrapper, which is the one tui.Run puts
// on. Unwrapping first would walk past the answer and find a model that never
// saw the keystroke.
func TestWasInterrupted_FalseForAnUnwrappedModel(t *testing.T) {
	assert.False(t, WasInterrupted(quietModel{}))
}

// The error fn returned has to reach the caller through the model. It used to
// travel in a variable the caller closed over, written by the goroutine
// running fn and read once Program.Run returned — fine until Ctrl-C made the
// program return while fn was still running, at which point the read won the
// race and found the nil nobody had written yet.
func TestSpinnerModel_CarriesTheErrorFromTheDoneMessage(t *testing.T) {
	s := spinner.New()
	s.Spinner = spinner.Dot

	want := errors.New("the build failed")

	m := spinnerModel{spinner: Loading{Spinner: s}, label: "Building"}

	updated, cmd := m.Update(spinnerDoneMsg{err: want})
	require.NotNil(t, cmd, "a finished phase quits")

	done, ok := updated.(spinnerModel)
	require.True(t, ok)
	assert.Equal(t, want, done.err)
	assert.True(t, done.done)
}

func TestSpinnerModel_ViewCarriesTheLiveNote(t *testing.T) {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = InfoStyle

	note := &atomic.Pointer[string]{}

	m := spinnerModel{
		spinner: Loading{Spinner: s},
		label:   "Waiting for the new version to serve",
		note:    note,
	}

	assert.NotContains(t, m.View(), "(", "nothing to say yet")

	said := "3m10s so far; the workload is running"
	note.Store(&said)

	view := m.View()
	assert.Contains(t, view, "Waiting for the new version to serve")
	assert.Contains(t, stripANSI(view), "("+said+")")
}

// stripANSI drops the styling so an assertion is about the words rather than
// about which colour lipgloss chose for the terminal running the test.
func stripANSI(s string) string {
	var (
		b  strings.Builder
		in bool
	)

	for _, r := range s {
		switch {
		case r == '\x1b':
			in = true
		case in && (r == 'm'):
			in = false
		case !in:
			b.WriteRune(r)
		}
	}

	return b.String()
}
