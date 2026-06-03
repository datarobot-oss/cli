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
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpinnerModel_ViewShowsLabelWhileRunning(t *testing.T) {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = InfoStyle

	m := spinnerModel{
		spinner: s,
		label:   "Loading…",
	}

	view := m.View()

	assert.Contains(t, view, "Loading…")
}

func TestSpinnerModel_ViewEmptyWhenDone(t *testing.T) {
	s := spinner.New()
	s.Spinner = spinner.Dot

	m := spinnerModel{
		spinner: s,
		label:   "Loading…",
		done:    true,
	}

	assert.Empty(t, m.View())
}

func TestSpinnerModel_UpdateOnDoneMsg(t *testing.T) {
	s := spinner.New()
	s.Spinner = spinner.Dot

	m := spinnerModel{
		spinner: s,
		label:   "Loading…",
	}

	updated, cmd := m.Update(spinnerDoneMsg{err: nil})

	assert.True(t, updated.(spinnerModel).done)
	assert.NotNil(t, cmd) // tea.Quit
}

func TestSpinnerModel_UpdateOnDoneMsgWithError(t *testing.T) {
	s := spinner.New()
	s.Spinner = spinner.Dot

	m := spinnerModel{
		spinner: s,
		label:   "Loading…",
	}

	sentErr := errors.New("something failed")

	updated, _ := m.Update(spinnerDoneMsg{err: sentErr})

	assert.True(t, updated.(spinnerModel).done)
}

func TestRunWithSpinner_SuccessPath(t *testing.T) {
	called := false

	err := RunWithSpinner("test label", func() error {
		called = true

		return nil
	})

	require.NoError(t, err)
	assert.True(t, called)
}

func TestRunWithSpinner_ErrorPath(t *testing.T) {
	want := errors.New("fn error")

	err := RunWithSpinner("test label", func() error {
		return want
	})

	assert.ErrorIs(t, err, want)
}

func TestRunWithSpinner_NonInteractiveEnv_SkipsTUI(t *testing.T) {
	t.Setenv("DATAROBOT_CLI_NON_INTERACTIVE", "true")

	called := false

	err := RunWithSpinner("test label", func() error {
		called = true

		return nil
	})

	require.NoError(t, err)
	assert.True(t, called)
}

func TestIsNonInteractiveEnv(t *testing.T) {
	t.Setenv("DATAROBOT_CLI_NON_INTERACTIVE", "")
	assert.False(t, isNonInteractiveEnv())

	t.Setenv("DATAROBOT_CLI_NON_INTERACTIVE", "true")
	assert.True(t, isNonInteractiveEnv())

	t.Setenv("DATAROBOT_CLI_NON_INTERACTIVE", "1")
	assert.True(t, isNonInteractiveEnv())

	t.Setenv("DATAROBOT_CLI_NON_INTERACTIVE", "false")
	assert.False(t, isNonInteractiveEnv())
}

func TestRunWithSpinner_LabelRendered(t *testing.T) {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = InfoStyle

	m := spinnerModel{
		spinner: s,
		label:   "my special label",
	}

	view := m.View()

	assert.Contains(t, view, "my special label")
}

func TestSpinnerModel_InitReturnsBatch(t *testing.T) {
	s := spinner.New()
	s.Spinner = spinner.Dot

	m := spinnerModel{
		spinner: s,
		label:   "Loading…",
		fn:      func() error { return nil },
	}

	cmd := m.Init()

	assert.NotNil(t, cmd)

	// Execute the batch to drain msgs (just verifies it doesn't panic)
	msgs := cmd()
	_ = msgs
}

func TestSpinnerModel_UpdateTickMsg(t *testing.T) {
	s := spinner.New()
	s.Spinner = spinner.Dot

	m := spinnerModel{
		spinner: s,
		label:   "Loading…",
	}

	tickMsg := spinner.TickMsg{}

	updated, cmd := m.Update(tickMsg)

	assert.NotNil(t, updated)
	assert.NotNil(t, cmd) // next tick
}

func TestSpinnerModel_UpdateUnknownMsg(t *testing.T) {
	s := spinner.New()
	s.Spinner = spinner.Dot

	type unknownMsg struct{}

	m := spinnerModel{
		spinner: s,
		label:   "Loading…",
	}

	updated, cmd := m.Update(unknownMsg{})

	assert.NotNil(t, updated)
	assert.Nil(t, cmd)
}

func TestSpinnerModel_UpdateReturnsSelf(t *testing.T) {
	s := spinner.New()
	s.Spinner = spinner.Dot

	m := spinnerModel{
		spinner: s,
		label:   "Loading…",
	}

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	_, ok := updated.(spinnerModel)

	assert.True(t, ok)
}
