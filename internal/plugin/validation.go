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

package plugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/datarobot/cli/internal/log"
	"github.com/google/go-cmp/cmp"
)

// ValidatePluginScript validates that a plugin script outputs a manifest matching the expected manifest.
// All fields must match exactly, including Scripts and MinCLIVersion for managed plugins.
func ValidatePluginScript(pluginDir string, expectedManifest PluginManifest) error {
	if err := ValidateLicense(pluginDir); err != nil {
		return err
	}

	scriptPath, err := FindPluginScript(pluginDir, expectedManifest.Name)
	if err != nil {
		return err
	}

	log.Debug("Validating plugin script outputs correct manifest", "script", scriptPath)

	scriptManifest, err := ExecPluginManifest(scriptPath)
	if err != nil {
		return fmt.Errorf("failed to execute plugin script: %w", err)
	}

	return validateManifests(expectedManifest, *scriptManifest)
}

// ValidateLicense validates that a plugin has a LICENSE.txt file.
func ValidateLicense(pluginDir string) error {
	licensePath := filepath.Join(pluginDir, "LICENSE.txt")

	if _, err := os.Stat(licensePath); err != nil {
		if os.IsNotExist(err) {
			return errors.New("plugin must contain LICENSE.txt file")
		}

		return fmt.Errorf("failed to check for LICENSE.txt: %w", err)
	}

	log.Debug("Plugin license validation passed", "path", licensePath)

	return nil
}

// validateManifests compares two manifests and returns an error if they differ.
func validateManifests(expected, actual PluginManifest) error {
	opts := cmp.Options{
		// Ignore Scripts and MinCLIVersion - they're optional managed plugin fields
		cmp.FilterPath(func(p cmp.Path) bool {
			return p.String() == "Scripts" || p.String() == "MinCLIVersion"
		}, cmp.Ignore()),
	}

	if diff := cmp.Diff(expected, actual, opts); diff != "" {
		return fmt.Errorf("plugin script output does not match manifest.json:\n%s", diff)
	}

	log.Debug("Plugin script manifest validation passed")

	return nil
}

// FindPluginScript finds the plugin script in the given directory.
func FindPluginScript(pluginDir, pluginName string) (string, error) {
	// First try exact match (no extension)
	scriptPath := filepath.Join(pluginDir, PluginPrefix+pluginName)

	if _, err := os.Stat(scriptPath); err == nil {
		return scriptPath, nil
	}

	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		return "", fmt.Errorf("failed to read plugin directory: %w", err)
	}

	foundFiles := make([]string, 0, len(entries))

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		foundFiles = append(foundFiles, name)

		// Check for exact match or script with extensions
		if isPluginScript(name) {
			return filepath.Join(pluginDir, name), nil
		}
	}

	if len(foundFiles) == 0 {
		return "", fmt.Errorf("plugin script not found: expected '%s%s' in directory %s (directory is empty)", PluginPrefix, pluginName, pluginDir)
	}

	return "", fmt.Errorf("plugin script not found: expected '%s%s' in directory %s (found: %v)", PluginPrefix, pluginName, pluginDir, foundFiles)
}

func isPluginScript(name string) bool {
	prefixLen := len(PluginPrefix)
	if len(name) < prefixLen || name[:prefixLen] != PluginPrefix {
		return false
	}

	ext := filepath.Ext(name)

	// No extension is always valid (Unix executable)
	if ext == "" {
		return true
	}

	// Platform-specific extensions
	if runtime.GOOS == "windows" {
		return ext == ".ps1"
	}

	return ext == ".sh"
}

// ExecPluginManifest executes a plugin script and returns its manifest.
func ExecPluginManifest(scriptPath string) (*PluginManifest, error) {
	if err := os.Chmod(scriptPath, 0o755); err != nil {
		return nil, fmt.Errorf("failed to make script executable: %w", err)
	}

	cmd := exec.Command(scriptPath, PluginManifestFlag)

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("script error: %s", stderr.String())
		}

		return nil, fmt.Errorf("failed to execute: %w", err)
	}

	var manifest PluginManifest

	decoder := json.NewDecoder(&stdout)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("invalid manifest JSON - check field names (use 'authenticated' not 'authentication'): %w\nOutput: %s", err, stdout.String())
	}

	return &manifest, nil
}
