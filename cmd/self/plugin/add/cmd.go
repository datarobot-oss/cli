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

package add

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/plugin"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var pluginName string

	var version string

	var fromFile string

	cmd := &cobra.Command{
		Use:   "add <path-to-index.json>",
		Short: "Add a packaged plugin version to a registry file (index.json)",
		Long: `Add a packaged plugin version entry to a registry file (index.json).

This command helps maintain the plugin registry by adding new version entries
to the specified index.json file.

You can either:
1. Use --from-file to load data from a file created by 'dr self plugin package --index-output'
2. Specify all values manually with individual flags

Example using --from-file:
  dr self plugin package ./my-plugin --index-output /tmp/fragment.json
  dr self plugin add docs/plugins/index.json --from-file /tmp/fragment.json

Example with manual flags:
  dr self plugin add docs/plugins/index.json \
    --name my-plugin \
    --version 1.0.0 \
    --url my-plugin/my-plugin-1.0.0.tar.xz \
    --sha256 abc123... \
    --release-date 2026-01-28`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			indexPath := args[0]

			if fromFile != "" {
				return addFromFile(indexPath, fromFile)
			}

			releaseDate, _ := cmd.Flags().GetString("release-date")
			url, _ := cmd.Flags().GetString("url")
			sha256, _ := cmd.Flags().GetString("sha256")

			if pluginName == "" {
				return errors.New("either --from-file or --name is required")
			}

			if version == "" {
				return errors.New("--version is required")
			}

			if url == "" {
				return errors.New("--url is required")
			}

			if sha256 == "" {
				return errors.New("--sha256 is required")
			}

			if releaseDate == "" {
				return errors.New("--release-date is required")
			}

			return addPluginToIndex(indexPath, pluginName, version, url, sha256, releaseDate)
		},
	}

	cmd.Flags().StringVar(&fromFile, "from-file", "", "Load plugin data from file created by 'dr self plugin package --index-output'")
	cmd.Flags().StringVar(&pluginName, "name", "", "Plugin name (required if not using --from-file)")
	cmd.Flags().StringVar(&version, "version", "", "Plugin version (required if not using --from-file)")
	cmd.Flags().String("url", "", "Archive URL relative to registry base (required if not using --from-file)")
	cmd.Flags().String("sha256", "", "SHA256 checksum of the archive (required if not using --from-file)")
	cmd.Flags().String("release-date", "", "Release date in YYYY-MM-DD format (required if not using --from-file)")

	return cmd
}

type indexFragment struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	URL         string `json:"url"`
	SHA256      string `json:"sha256"`
	ReleaseDate string `json:"releaseDate"`
}

func addFromFile(indexPath, fragmentPath string) error {
	data, err := os.ReadFile(fragmentPath)
	if err != nil {
		return fmt.Errorf("failed to read fragment file: %w", err)
	}

	var fragment indexFragment

	if err := json.Unmarshal(data, &fragment); err != nil {
		return fmt.Errorf("failed to parse fragment file: %w", err)
	}

	if fragment.Name == "" {
		return errors.New("fragment missing required field: name")
	}

	if fragment.Version == "" {
		return errors.New("fragment missing required field: version")
	}

	if fragment.URL == "" {
		return errors.New("fragment missing required field: url")
	}

	log.Info("Loading plugin data from fragment",
		"name", fragment.Name,
		"version", fragment.Version)

	return addPluginToIndex(indexPath, fragment.Name, fragment.Version, fragment.URL, fragment.SHA256, fragment.ReleaseDate)
}

func addPluginToIndex(indexPath, pluginName, version, url, sha256, releaseDate string) error {
	absPath, err := filepath.Abs(indexPath)
	if err != nil {
		return fmt.Errorf("failed to resolve registry path: %w", err)
	}

	registry, err := loadOrCreateRegistry(absPath)
	if err != nil {
		return err
	}

	if registry.Plugins == nil {
		registry.Plugins = make(map[string]plugin.RegistryPlugin)
	}

	newVersion := plugin.RegistryVersion{
		Version:     version,
		URL:         url,
		SHA256:      sha256,
		ReleaseDate: releaseDate,
	}

	pluginEntry, exists := registry.Plugins[pluginName]
	if !exists {
		pluginEntry = plugin.RegistryPlugin{
			Name:     pluginName,
			Versions: []plugin.RegistryVersion{newVersion},
		}

		log.Info("Creating new plugin entry", "name", pluginName)
	} else {
		for _, v := range pluginEntry.Versions {
			if v.Version == version {
				return fmt.Errorf("version %s already exists for plugin %s", version, pluginName)
			}
		}

		pluginEntry.Versions = append(pluginEntry.Versions, newVersion)

		log.Info("Adding version to existing plugin", "name", pluginName, "version", version)
	}

	registry.Plugins[pluginName] = pluginEntry

	if err := saveRegistry(absPath, registry); err != nil {
		return err
	}

	fmt.Printf("✅ Added %s version %s to %s\n", pluginName, version, absPath)

	return nil
}

func loadOrCreateRegistry(path string) (*plugin.PluginRegistry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Info("Creating new registry file", "path", path)

			return &plugin.PluginRegistry{
				Version: "1",
				Plugins: make(map[string]plugin.RegistryPlugin),
			}, nil
		}

		return nil, fmt.Errorf("failed to read registry: %w", err)
	}

	var registry plugin.PluginRegistry

	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, fmt.Errorf("failed to parse registry: %w", err)
	}

	if registry.Version == "" {
		registry.Version = "1"
	}

	if registry.Plugins == nil {
		registry.Plugins = make(map[string]plugin.RegistryPlugin)
	}

	return &registry, nil
}

func saveRegistry(path string, registry *plugin.PluginRegistry) error {
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal registry: %w", err)
	}

	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write registry: %w", err)
	}

	return nil
}
