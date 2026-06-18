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

func TestTemplateListJSON(t *testing.T) {
	t.Skip("Skipping template list test - requires authentication and API access")

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

	// Verify the structure has "templates" key
	assert.Contains(t, result, "templates")

	// Verify templates is an array
	templates, ok := result["templates"].([]interface{})
	assert.True(t, ok, "templates should be an array")

	// If templates exist, verify structure
	if len(templates) > 0 {
		template, ok := templates[0].(map[string]interface{})
		assert.True(t, ok, "each template should be an object")
		assert.Contains(t, template, "id")
		assert.Contains(t, template, "name")
	}
}

func TestTemplateListText(t *testing.T) {
	t.Skip("Skipping template list test - requires authentication and API access")

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
	if len(output) > 0 {
		assert.NotContains(t, output, `"templates":`, "text output should not have JSON templates key")
	}
}
