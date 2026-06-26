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
	"github.com/datarobot/cli/internal/plugin"
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

	// Parse args to set JSON output format
	root.SetArgs([]string{"list", "--output-format", "json"})

	// Execute the command - should not error
	err := root.Execute()
	require.NoError(t, err)

	// Test passes if command executes successfully with JSON format flag
	// (actual output is tested via manual smoke tests)
}

func TestPluginListText(t *testing.T) {
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

func TestToPluginOutputs(t *testing.T) {
	plugins := []plugin.DiscoveredPlugin{
		{
			Manifest: plugin.PluginManifest{
				BasicPluginManifest: plugin.BasicPluginManifest{
					Name:        "my-plugin",
					Version:     "1.0.0",
					Description: "A plugin",
				},
			},
			Executable: "/path/to/plugin",
		},
		{
			Manifest: plugin.PluginManifest{
				BasicPluginManifest: plugin.BasicPluginManifest{
					Name:        "empty-plugin",
					Version:     "",
					Description: "",
				},
			},
			Executable: "/path/to/empty",
		},
	}

	outputs := toPluginOutputs(plugins)

	require.Len(t, outputs, 2)
	assert.Equal(t, "my-plugin", outputs[0].Name)
	assert.Equal(t, "1.0.0", outputs[0].Version)
	assert.Equal(t, "A plugin", outputs[0].Description)
	assert.Equal(t, "/path/to/plugin", outputs[0].Path)
	assert.Equal(t, "empty-plugin", outputs[1].Name)
	assert.Equal(t, "-", outputs[1].Version)
	assert.Equal(t, "-", outputs[1].Description)
	assert.Equal(t, "/path/to/empty", outputs[1].Path)
}

func TestToPluginOutputsEmpty(t *testing.T) {
	assert.Empty(t, toPluginOutputs(nil))
	assert.Empty(t, toPluginOutputs([]plugin.DiscoveredPlugin{}))
}
