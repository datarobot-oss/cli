// Copyright 2025 DataRobot, Inc. and its affiliates.
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
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/repo"
	"github.com/spf13/viper"
)

var registry = &PluginRegistry{}

// GetPlugins returns discovered plugins, discovering lazily on first call
func GetPlugins() ([]DiscoveredPlugin, error) {
	registry.once.Do(func() {
		registry.plugins, registry.err = discoverPlugins()
	})

	return registry.plugins, registry.err
}

func discoverPlugins() ([]DiscoveredPlugin, error) {
	plugins := make([]DiscoveredPlugin, 0)

	seen := make(map[string]bool)

	// 1. Check project-local directory first (higher priority)
	localPlugins, errs := discoverInDir(repo.LocalPluginDir, seen)
	plugins = append(plugins, localPlugins...)

	for _, err := range errs {
		log.Debug("Plugin discovery error in local dir", "dir", repo.LocalPluginDir, "error", err)
	}

	// 2. Check PATH directories
	pathEnv := os.Getenv("PATH")

	for _, dir := range filepath.SplitList(pathEnv) {
		dirPlugins, errs := discoverInDir(dir, seen)
		plugins = append(plugins, dirPlugins...)

		for _, err := range errs {
			log.Debug("Plugin discovery error", "dir", dir, "error", err)
		}
	}

	log.Debug("Plugin discovery complete", "count", len(plugins))

	return plugins, nil
}

func discoverInDir(dir string, seen map[string]bool) ([]DiscoveredPlugin, []error) {
	plugins := make([]DiscoveredPlugin, 0)

	var errors []error

	// Check if directory exists
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, nil // Directory doesn't exist, not an error
	}

	// Read directory entries
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, []error{err}
	}

	for _, entry := range entries {
		name := entry.Name()

		// Must match dr-* pattern
		if !strings.HasPrefix(name, "dr-") {
			continue
		}

		fullPath := filepath.Join(dir, name)

		// Must be executable
		if !isExecutable(fullPath) {
			continue
		}

		// Validate plugin can be found by exec.LookPath
		if _, err := exec.LookPath(fullPath); err != nil {
			log.Debug("Plugin not executable by Go runtime", "path", fullPath, "error", err)
			continue
		}

		// Try to get manifest
		manifest, err := getManifest(fullPath)
		if err != nil {
			errors = append(errors, err)

			continue
		}

		// Deduplicate on manifest.Name (the actual command name)
		if seen[manifest.Name] {
			log.Debug("Plugin name already registered, skipping",
				"name", manifest.Name,
				"path", fullPath)

			continue
		}

		seen[manifest.Name] = true

		plugins = append(plugins, DiscoveredPlugin{
			Manifest:   *manifest,
			Executable: fullPath,
		})
	}

	return plugins, errors
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	if runtime.GOOS == "windows" {
		// Check for executable extensions
		ext := strings.ToLower(filepath.Ext(path))

		return ext == ".exe" || ext == ".bat" || ext == ".cmd" || ext == ".ps1"
	}

	// Unix: check execute bit
	return info.Mode()&0o111 != 0
}

func getManifest(executable string) (*PluginManifest, error) {
	// Default timeout if not configured
	timeout := 500 * time.Millisecond
	if viper.IsSet("plugin.manifest_timeout_ms") {
		timeout = time.Duration(viper.GetInt("plugin.manifest_timeout_ms")) * time.Millisecond
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, executable, "--dr-plugin-manifest")

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var manifest PluginManifest

	if err := json.Unmarshal(output, &manifest); err != nil {
		return nil, err
	}

	// Validate required fields
	if manifest.Name == "" {
		return nil, errors.New("plugin manifest missing required field: name")
	}

	return &manifest, nil
}
