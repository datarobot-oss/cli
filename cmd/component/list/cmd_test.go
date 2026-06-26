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

	"github.com/datarobot/cli/internal/copier"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComponentListJSON(t *testing.T) {
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

func TestComponentListText(t *testing.T) {
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

func TestToComponentOutputs(t *testing.T) {
	answers := []copier.Answers{
		{
			FileName:         "comp1.yaml",
			ComponentDetails: copier.Details{Name: "component-one"},
			Repo:             "https://github.com/example/one",
		},
		{
			FileName:         "comp2.yaml",
			ComponentDetails: copier.Details{Name: "component-two"},
			Repo:             "",
		},
	}

	outputs := toComponentOutputs(answers)

	require.Len(t, outputs, 2)
	assert.Equal(t, "component-one", outputs[0].Name)
	assert.Equal(t, "comp1.yaml", outputs[0].File)
	assert.Equal(t, "https://github.com/example/one", outputs[0].Repo)
	assert.Equal(t, "component-two", outputs[1].Name)
	assert.Equal(t, "comp2.yaml", outputs[1].File)
	assert.Empty(t, outputs[1].Repo)
}

func TestToComponentOutputsEmpty(t *testing.T) {
	assert.Empty(t, toComponentOutputs(nil))
	assert.Empty(t, toComponentOutputs([]copier.Answers{}))
}
