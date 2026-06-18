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
	"bytes"
	"encoding/json"
	"testing"

	"github.com/datarobot/cli/internal/outputformat"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskListJSON(t *testing.T) {
	// Create a root command with persistent output-format flag
	root := &cobra.Command{Use: "test"}
	var rootOutputFormat outputformat.OutputFormat
	outputformat.AddPersistentFlag(root, &rootOutputFormat)

	// Create and add the list command
	listCmd := Cmd()
	root.AddCommand(listCmd)

	// Set output to capture JSON
	buf := new(bytes.Buffer)
	root.SetOut(buf)

	// Parse args to set JSON output format
	root.SetArgs([]string{"list", "--output-format", "json"})

	// Execute the command
	err := root.Execute()
	require.NoError(t, err)

	// Parse the JSON output
	var result map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	// Verify the structure has "tasks" key
	assert.Contains(t, result, "tasks")

	// Verify tasks is an array
	tasks, ok := result["tasks"].([]interface{})
	assert.True(t, ok, "tasks should be an array")

	// If tasks exist, verify structure
	if len(tasks) > 0 {
		task, ok := tasks[0].(map[string]interface{})
		assert.True(t, ok, "each task should be an object")
		assert.Contains(t, task, "name")
		assert.Contains(t, task, "desc")
	}
}

func TestTaskListText(t *testing.T) {
	// Create a root command with persistent output-format flag
	root := &cobra.Command{Use: "test"}
	var rootOutputFormat outputformat.OutputFormat
	outputformat.AddPersistentFlag(root, &rootOutputFormat)

	// Create and add the list command
	listCmd := Cmd()
	root.AddCommand(listCmd)

	// Set output to capture
	buf := new(bytes.Buffer)
	root.SetOut(buf)

	// Execute with default text format
	root.SetArgs([]string{"list"})

	// Execute the command
	err := root.Execute()
	require.NoError(t, err)

	// Text output should not have JSON structure
	output := buf.String()
	var result map[string]interface{}
	err = json.Unmarshal([]byte(output), &result)
	// Text output should not be valid JSON with "tasks" key
	if len(output) > 0 {
		assert.NotContains(t, output, `"tasks":`, "text output should not have JSON tasks key")
	}
}
