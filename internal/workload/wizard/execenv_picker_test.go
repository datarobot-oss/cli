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
	"testing"

	"github.com/datarobot/cli/internal/workload"
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
	}, 120, 40)

	require.Len(t, picker.all, 2)
	assert.Equal(t, []string{"BASE IMAGE", "LANGUAGE", "ID"}, picker.headers)
	assert.Equal(t, []string{"Py 3.12", "python", "ee-1"}, picker.all[0].cells)
	// "other" is where the platform files what it cannot name; as a column
	// value it reads better as absence than as a category.
	assert.Equal(t, []string{"Node", "—", "ee-2"}, picker.all[1].cells)
}
