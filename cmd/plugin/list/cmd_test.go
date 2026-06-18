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

func TestPluginListJSON(t *testing.T) {
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

	// Verify the structure has "plugins" key
	assert.Contains(t, result, "plugins")

	// Verify plugins is an array
	plugins, ok := result["plugins"].([]interface{})
	assert.True(t, ok, "plugins should be an array")

	// If plugins exist, verify structure
	if len(plugins) > 0 {
		plugin, ok := plugins[0].(map[string]interface{})
		assert.True(t, ok, "each plugin should be an object")
		assert.Contains(t, plugin, "name")
		assert.Contains(t, plugin, "version")
		assert.Contains(t, plugin, "description")
		assert.Contains(t, plugin, "path")
	}
}

func TestPluginListText(t *testing.T) {
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

	// Text output should not be valid JSON (it contains the table)
	output := buf.String()
	var result map[string]interface{}
	err = json.Unmarshal([]byte(output), &result)
	// Should fail to unmarshal as JSON if it's text output
	if len(output) > 0 {
		// Output exists; if it's JSON it will parse, if it's text it won't
		// We just verify that text mode doesn't output the JSON wrapper
		assert.NotContains(t, output, `"plugins":`, "text output should not have JSON plugins key")
	}
}
