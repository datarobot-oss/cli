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

package pluginpackage

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

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/plugin"
	"github.com/spf13/cobra"
	"github.com/ulikunitz/xz"
)

func Cmd() *cobra.Command {
	var outputDir string

	var indexOutput string

	cmd := &cobra.Command{
		Use:   "package <plugin-dir>",
		Short: "Package a plugin directory into a .tar.xz archive",
		Long: `Package a plugin directory into a distributable .tar.xz archive.

The plugin directory must contain a manifest.json file with at minimum:
  - name: plugin name
  - version: version string (e.g., "1.0.0")

The command will:
  1. Validate the manifest
  2. Create a .tar.xz archive
  3. Calculate SHA256 checksum
  4. Output a JSON snippet for index.json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pluginDir := args[0]

			return packagePlugin(pluginDir, outputDir, indexOutput)
		},
	}

	cmd.Flags().StringVarP(&outputDir, "output", "o", ".",
		"Output file path (e.g., my-plugin-1.0.0.tar.xz) or directory (defaults to current directory)")
	cmd.Flags().StringVar(&indexOutput, "index-output", "",
		"Save index JSON fragment to file for use with 'dr self plugin add --from-file'")

	return cmd
}

func packagePlugin(pluginDir, output, indexOutput string) error {
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
	archivePath := determineOutputPath(output, archiveName)

	// Create parent directory if needed
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	log.Info("Packaging plugin",
		"source", pluginDir,
		"output", archivePath,
		"version", manifest.Version)

	if err := createArchive(pluginDir, archivePath); err != nil {
		return fmt.Errorf("failed to create archive: %w", err)
	}

	sha256sum, err := calculateSHA256(archivePath)
	if err != nil {
		return fmt.Errorf("failed to calculate checksum: %w", err)
	}

	releaseDate := time.Now().Format("2006-01-02")

	fmt.Printf("\n✅ Package created: %s\n", archivePath)
	fmt.Printf("   SHA256: %s\n\n", sha256sum)

	if indexOutput != "" {
		if err := saveIndexFragment(indexOutput, manifest, archiveName, sha256sum, releaseDate); err != nil {
			return fmt.Errorf("failed to save index fragment: %w", err)
		}

		fmt.Printf("📝 Index fragment saved to: %s\n\n", indexOutput)
	}

	printIndexJSON(manifest, archiveName, sha256sum, releaseDate)

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

	return &manifest, nil
}

func validatePluginScript(pluginDir string, expectedManifest *plugin.PluginManifest) error {
	log.Info("Validating plugin script output", "plugin", expectedManifest.Name)

	if err := plugin.ValidatePluginScript(pluginDir, *expectedManifest); err != nil {
		return err
	}

	log.Info("✓ Plugin script output matches manifest.json")

	return nil
}

func determineOutputPath(output, archiveName string) string {
	// If output ends with .tar.xz, use it as the exact path
	if filepath.Ext(output) == ".xz" && filepath.Ext(filepath.Base(output[:len(output)-3])) == ".tar" {
		return output
	}

	// Otherwise treat it as a directory
	return filepath.Join(output, archiveName)
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

func printIndexJSON(manifest *plugin.PluginManifest, archiveName, sha256sum, releaseDate string) {
	fmt.Println("Add to index.json:")
	fmt.Println("```json")

	snippet := map[string]interface{}{
		"version":     manifest.Version,
		"url":         fmt.Sprintf("%s/%s", manifest.Name, archiveName),
		"sha256":      sha256sum,
		"releaseDate": releaseDate,
	}

	data, _ := json.MarshalIndent(snippet, "", "  ")
	fmt.Println(string(data))

	fmt.Println("```")
}

type indexFragment struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	URL         string `json:"url"`
	SHA256      string `json:"sha256"`
	ReleaseDate string `json:"releaseDate"`
}

func saveIndexFragment(path string, manifest *plugin.PluginManifest, archiveName, sha256sum, releaseDate string) error {
	fragment := indexFragment{
		Name:        manifest.Name,
		Version:     manifest.Version,
		URL:         fmt.Sprintf("%s/%s", manifest.Name, archiveName),
		SHA256:      sha256sum,
		ReleaseDate: releaseDate,
	}

	data, err := json.MarshalIndent(fragment, "", "  ")
	if err != nil {
		return err
	}

	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}

	return nil
}
