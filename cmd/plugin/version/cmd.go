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

package version

import (
	"fmt"
	"os"

	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/plugin"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/spf13/cobra"
)

// PluginVersionOutput is the JSON representation of a plugin's version for --output-format json.
type PluginVersionOutput struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func Cmd() *cobra.Command {
	var outputFormat outputformat.OutputFormat

	cmd := &cobra.Command{
		Use:     "version <plugin-name>",
		Short:   "🏷️ Show a plugin's version",
		Long:    "Display the version of a discovered plugin, as reported in its manifest.",
		Example: "  dr plugin version assist\n  dr plugin version assist --output-format json",
		Args:    cobra.ExactArgs(1),
		RunE:    runVersion,
	}

	outputformat.AddFlag(cmd, &outputFormat)

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"plugin_name": telemetry.FirstArg(args),
		}
	})

	return cmd
}

// findPlugin returns the discovered plugin named pluginName, or nil if not found.
func findPlugin(plugins []plugin.DiscoveredPlugin, pluginName string) *plugin.DiscoveredPlugin {
	for _, p := range plugins {
		if p.Manifest.Name == pluginName {
			return &p
		}
	}

	return nil
}

func runVersion(cmd *cobra.Command, args []string) error {
	pluginName := args[0]

	plugins, conflicts := plugin.GetPlugins()

	// Only warn about conflicts for the specific plugin being asked about —
	// unrelated name clashes elsewhere on PATH aren't relevant to this query.
	plugin.LogConflicts(plugin.ConflictsForName(conflicts, pluginName))

	p := findPlugin(plugins, pluginName)
	if p == nil {
		return fmt.Errorf("plugin %q not found", pluginName)
	}

	format := outputformat.GetFormat(cmd)
	if format == outputformat.OutputFormatJSON {
		output := PluginVersionOutput{Name: p.Manifest.Name, Version: p.Manifest.Version}

		return outputformat.PrintJSONEnvelope(os.Stdout, "plugin", output)
	}

	if p.Manifest.Version == "" {
		fmt.Printf("Plugin %q does not report a version.\n", pluginName)

		return nil
	}

	fmt.Println(p.Manifest.Version)

	return nil
}
