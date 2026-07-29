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

package setup

import (
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regression test for a bug where Model.Update's top-level switch intercepted
// every spinner.TickMsg (matching by type alone) and returned before the
// per-screen dispatch that forwards messages to the active child screen
// (m.clone, m.dotenv). That silently swallowed tick messages meant for a
// child's own spinner, freezing it after a single frame - e.g. the clone
// screen and the dotenv wizard's "Fetching available LLMs..." spinner.
func TestUpdate_SpinnerTickReachesCloneScreenDispatch(t *testing.T) {
	// NewModel reads the on-disk CLI config via a process-global viper
	// instance (internal/config). Isolate HOME/XDG so this test doesn't
	// read the real machine's config, and reset viper afterward so no state
	// leaks into other tests in this package - mirroring the isolation
	// pattern in loginModel_test.go's SetupTest/AfterTest.
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Cleanup(viperx.Reset)

	m := NewModel(false)
	m.screen = cloneScreen
	m.clone.SetTemplate(drapi.Template{
		Repository: drapi.Repository{URL: "https://example.com/repo.git"},
	})

	// Drive the clone child into its "cloning" state via a real Update call,
	// exactly as the wizard would when the user presses enter.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m, ok := updated.(Model)
	require.True(t, ok)
	require.True(t, m.clone.IsCloning())

	// A zero-value spinner.TickMsg is accepted by any spinner.Model (bubbles
	// only rejects ticks with a non-zero, mismatched ID), so both the
	// parent's own spinner AND the clone screen's own spinner (still ticking
	// internally, even though the wizard's status bar is the only place it's
	// rendered - see clone.Model.View) will produce a reschedule cmd for it,
	// if - and only if - the message actually reaches m.clone.Update.
	//
	// tea.Batch collapses to a single cmd when only one of its inputs is
	// non-nil, and only produces a tea.BatchMsg when 2+ are non-nil. So if
	// the child's tick was swallowed by the parent's top-level switch (the
	// regression), only the parent's own spinner cmd would exist and this
	// would NOT be a BatchMsg.
	_, cmd := m.Update(spinner.TickMsg{})
	require.NotNil(t, cmd)

	msg := cmd()
	_, isBatch := msg.(tea.BatchMsg)
	assert.True(t, isBatch,
		"expected both the parent's and the clone screen's own spinner cmds to be batched - "+
			"if not, the child's tick was swallowed before reaching the cloneScreen dispatch")
}
