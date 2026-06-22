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

package list

import (
	"testing"

	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/task"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskListJSON(t *testing.T) {
	t.Skip("Skipping task list test - requires a DataRobot project directory with Taskfile")

	// Create a root command with persistent output-format flag
	root := &cobra.Command{Use: "test"}

	var rootOutputFormat outputformat.OutputFormat
	outputformat.AddPersistentFlag(root, &rootOutputFormat)

	// Create and add the list command
	listCmd := Cmd()
	root.AddCommand(listCmd)

	// Parse args to set JSON output format
	root.SetArgs([]string{"list", "--output-format", "json"})

	// Execute the command - should not error
	err := root.Execute()
	require.NoError(t, err)

	// Test passes if command executes successfully with JSON format flag
	// (actual output is tested via manual smoke tests)
}

func TestTaskListText(t *testing.T) {
	t.Skip("Skipping task list test - requires a DataRobot project directory with Taskfile")

	// Create a root command with persistent output-format flag
	root := &cobra.Command{Use: "test"}

	var rootOutputFormat outputformat.OutputFormat
	outputformat.AddPersistentFlag(root, &rootOutputFormat)

	// Create and add the list command
	listCmd := Cmd()
	root.AddCommand(listCmd)

	// Execute with default text format
	root.SetArgs([]string{"list"})

	// Execute the command - should not error
	err := root.Execute()
	require.NoError(t, err)

	// Test passes if command executes successfully with default (text) format
	// (actual output is tested via manual smoke tests)
}

func TestToTaskOutputs(t *testing.T) {
	tasks := []task.Task{
		{
			Name:    "build",
			Aliases: []string{"b", "compile"},
			Desc:    "Build the application\nwith all dependencies",
		},
		{
			Name: "test",
			Desc: "Run tests",
		},
	}

	outputs := toTaskOutputs(tasks)

	require.Len(t, outputs, 2)
	assert.Equal(t, "build", outputs[0].Name)
	assert.Equal(t, []string{"b", "compile"}, outputs[0].Aliases)
	assert.Equal(t, "Build the application with all dependencies", outputs[0].Description)
	assert.Equal(t, "test", outputs[1].Name)
	assert.Empty(t, outputs[1].Aliases)
	assert.Equal(t, "Run tests", outputs[1].Description)
}

func TestToTaskOutputsEmpty(t *testing.T) {
	assert.Empty(t, toTaskOutputs(nil))
	assert.Empty(t, toTaskOutputs([]task.Task{}))
}
