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

package wizard

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/internal/workload"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecEnvPicker_ShowsTheLanguageColumn(t *testing.T) {
	picker := newExecEnvPicker([]workload.ExecutionEnvironment{
		{
			ID: "ee-1", Name: "Py 3.12", ProgrammingLanguage: "python",
			LatestSuccessfulVersion: &workload.EEVersion{ID: "v1"},
		},
		{
			ID: "ee-2", Name: "Node", ProgrammingLanguage: "other",
			LatestSuccessfulVersion: &workload.EEVersion{ID: "v2"},
		},
		{
			ID: "ee-3", Name: "R Lab", ProgrammingLanguage: "R",
			LatestSuccessfulVersion: &workload.EEVersion{ID: "v3"},
		},
	}, "", 120, 40)

	require.Len(t, picker.all, 3)
	assert.Equal(t, []string{"BASE IMAGE", "LANGUAGE", "ID"}, picker.headers)
	assert.Equal(t, []string{"Py 3.12", "python", "ee-1"}, picker.all[0].cells)
	// "other" is where the platform files what it cannot name; as a column
	// value it reads better as absence than as a category.
	assert.Equal(t, []string{"Node", "—", "ee-2"}, picker.all[1].cells)
	// The platform's own casing is kept: only the sort folds case.
	assert.Equal(t, []string{"R Lab", "R", "ee-3"}, picker.all[2].cells)
}

// When bound to a workload, the environment it is built on is tagged "· in
// use" and lifted to the top, so the long list opens on the option most
// likely to be kept.
func TestExecEnvPicker_TagsAndLiftsTheInUseEnvironment(t *testing.T) {
	picker := newExecEnvPicker([]workload.ExecutionEnvironment{
		{
			ID: "ee-1", Name: "Py 3.12", ProgrammingLanguage: "python",
			LatestSuccessfulVersion: &workload.EEVersion{ID: "v1"},
		},
		{
			ID: "ee-2", Name: "Node", ProgrammingLanguage: "javascript",
			LatestSuccessfulVersion: &workload.EEVersion{ID: "v2"},
		},
	}, "ee-2", 120, 40)

	require.Len(t, picker.all, 2)
	// Tagged and first; the tag rides on the last (ID) cell so it trails on the
	// right. It is plain text, not styled, because the cell flows through
	// bubbles/table's ANSI-unaware column sizing and truncation.
	assert.Equal(t, []string{"Node", "javascript", "ee-2" + inUseSuffix}, picker.all[0].cells,
		"the in-use environment is tagged on the right and lifted first")
	assert.Equal(t, []string{"Py 3.12", "python", "ee-1"}, picker.all[1].cells)
}

// The in-use tag rides a table cell, and bubbles/table sizes columns and
// truncates cells with no awareness of ANSI: a styled tag would inflate the
// measured width and, on a narrow terminal, be cut mid-escape, bleeding colour
// across the row. So the cells must stay plain text even under a real colour
// profile. The other picker tests run in the no-colour profile, where a stray
// style renders as plain text and a regression would hide; this one forces
// TrueColor so it cannot.
func TestExecEnvPicker_InUseTagCarriesNoAnsiUnderColour(t *testing.T) {
	prev := lipgloss.ColorProfile()

	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	picker := newExecEnvPicker([]workload.ExecutionEnvironment{
		{
			ID: "ee-1", Name: "Py 3.12", ProgrammingLanguage: "python",
			LatestSuccessfulVersion: &workload.EEVersion{ID: "v1"},
		},
		{
			ID: "ee-2", Name: "Node", ProgrammingLanguage: "javascript",
			LatestSuccessfulVersion: &workload.EEVersion{ID: "v2"},
		},
	}, "ee-2", 80, 40)

	for _, row := range picker.all {
		for _, cell := range row.cells {
			assert.NotContains(t, cell, "\x1b", "picker cells must stay plain text, got %q", cell)
		}
	}

	// The escape check is only meaningful if styling would otherwise emit one:
	// prove the forced profile is live by rendering the tag's own style.
	require.Contains(t, liveStyle.Render(inUseSuffix), "\x1b",
		"the test must run under a colour profile for the assertion above to have teeth")
	require.NotEmpty(t, strings.TrimSpace(inUseSuffix))
}

// A fresh setup has no bound workload, so nothing is tagged and the order is
// left as it came.
func TestExecEnvPicker_TagsNothingWithoutABinding(t *testing.T) {
	picker := newExecEnvPicker([]workload.ExecutionEnvironment{
		{
			ID: "ee-1", Name: "Py 3.12", ProgrammingLanguage: "python",
			LatestSuccessfulVersion: &workload.EEVersion{ID: "v1"},
		},
	}, "", 120, 40)

	require.Len(t, picker.all, 1)
	assert.Equal(t, []string{"Py 3.12", "python", "ee-1"}, picker.all[0].cells)
}
