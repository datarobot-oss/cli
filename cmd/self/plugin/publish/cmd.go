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

package publish

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/plugin"
	"github.com/spf13/cobra"
	"github.com/ulikunitz/xz"
)

func Cmd() *cobra.Command {
	var pluginsDir string

	var indexPath string

	cmd := &cobra.Command{
		Use:   "publish <plugin-dir>",
		Short: "Package and publish a plugin in one step",
		Long: `Package a plugin, copy it to the plugins directory, and update index.json.

This is an all-in-one command that:
  1. Validates the plugin manifest
  2. Creates a .tar.xz archive
  3. Copies it to plugins/<plugin-name>/<plugin-name>-<version>.tar.xz
  4. Updates the index.json with the new version

Example:
  dr self plugin publish ./my-plugin
  dr self plugin publish ./my-plugin --plugins-dir docs/plugins --index docs/plugins/index.json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pluginDir := args[0]

			resolvedIndexPath := indexPath
			if resolvedIndexPath == "" {
				resolvedIndexPath = filepath.Join(pluginsDir, "index.json")
			}

			return publishPlugin(pluginDir, pluginsDir, resolvedIndexPath)
		},
	}

	cmd.Flags().StringVar(&pluginsDir, "plugins-dir", "docs/plugins",
		"Directory where plugin archives are stored")
	cmd.Flags().StringVar(&indexPath, "index", "",
		"Path to the plugin index.json file (defaults to <plugins-dir>/index.json)")

	return cmd
}

func publishPlugin(pluginDir, pluginsDir, indexPath string) error {
	pluginDir = filepath.Clean(pluginDir)

	if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
		return fmt.Errorf("plugin directory does not exist: %s", pluginDir)
	}

	manifest, err := loadManifest(pluginDir)
	if err != nil {
		return err
	}

	if err := validatePluginScript(pluginDir, manifest); err != nil {
		return err
	}

	archiveName := fmt.Sprintf("%s-%s.tar.xz", manifest.Name, manifest.Version)
	pluginOutputDir := filepath.Join(pluginsDir, manifest.Name)
	archivePath := filepath.Join(pluginOutputDir, archiveName)

	if err := os.MkdirAll(pluginOutputDir, 0o755); err != nil {
		return fmt.Errorf("failed to create plugin directory: %w", err)
	}

	log.Info("Publishing plugin",
		"name", manifest.Name,
		"version", manifest.Version,
		"output", archivePath)

	if err := createArchive(pluginDir, archivePath); err != nil {
		return fmt.Errorf("failed to create archive: %w", err)
	}

	sha256sum, err := calculateSHA256(archivePath)
	if err != nil {
		return fmt.Errorf("failed to calculate checksum: %w", err)
	}

	releaseDate := time.Now().Format("2006-01-02")

	url := fmt.Sprintf("%s/%s", manifest.Name, archiveName)

	if err := addToIndex(indexPath, manifest.Name, manifest.Description, manifest.Version, url, sha256sum, releaseDate); err != nil {
		return fmt.Errorf("failed to update index: %w", err)
	}

	fmt.Printf("\n✅ Published %s version %s\n", manifest.Name, manifest.Version)
	fmt.Printf("   Archive: %s\n", archivePath)
	fmt.Printf("   SHA256: %s\n", sha256sum)
	fmt.Printf("   Index: %s\n\n", indexPath)

	return nil
}

func loadManifest(pluginDir string) (*plugin.PluginManifest, error) {
	manifestPath := filepath.Join(pluginDir, "manifest.json")

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest.json: %w", err)
	}

	var manifest plugin.PluginManifest

	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest.json: %w", err)
	}

	if manifest.Name == "" {
		return nil, errors.New("manifest.json missing required field: name")
	}

	if manifest.Version == "" {
		return nil, errors.New("manifest.json missing required field: version")
	}

	if manifest.Description == "" {
		return nil, errors.New("manifest.json missing required field: description")
	}

	return &manifest, nil
}

func createArchive(sourceDir, archivePath string) error {
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	defer archiveFile.Close()

	xzWriter, err := xz.NewWriter(archiveFile)
	if err != nil {
		return fmt.Errorf("failed to create xz writer: %w", err)
	}
	defer xzWriter.Close()

	tarWriter := tar.NewWriter(xzWriter)
	defer tarWriter.Close()

	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		if relPath == "." {
			return nil
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}

		header.Name = relPath

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(tarWriter, file)

		return err
	})
}

func calculateSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()

	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func addToIndex(indexPath, pluginName, description, version, url, sha256sum, releaseDate string) error {
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
		SHA256:      sha256sum,
		ReleaseDate: releaseDate,
	}

	pluginEntry, exists := registry.Plugins[pluginName]
	if !exists {
		pluginEntry = plugin.RegistryPlugin{
			Name:        pluginName,
			Description: description,
			Versions:    []plugin.RegistryVersion{newVersion},
		}

		log.Info("Creating new plugin entry", "name", pluginName)
	} else {
		for _, v := range pluginEntry.Versions {
			if v.Version == version {
				return fmt.Errorf("version %s already exists for plugin %s", version, pluginName)
			}
		}

		pluginEntry.Description = description
		pluginEntry.Versions = append(pluginEntry.Versions, newVersion)

		log.Info("Adding version to existing plugin", "name", pluginName, "version", version)
	}

	registry.Plugins[pluginName] = pluginEntry

	if err := saveRegistry(absPath, registry); err != nil {
		return err
	}

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

func validatePluginScript(pluginDir string, manifest *plugin.PluginManifest) error {
	log.Info("Validating plugin script output", "plugin", manifest.Name)

	return plugin.ValidatePluginScript(pluginDir, *manifest)
}
