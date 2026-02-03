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

// PluginRegistryTerminology is the user-facing term for the plugin registry
const PluginRegistryTerminology = "registry"

// PluginRegistryURL is the default URL for the remote plugin registry
const PluginRegistryURL = "https://cli.datarobot.com/plugins/index.json"

// TODO: Consider adding ResetRegistry() for testing, as package-level state makes unit tests harder
var registry = &DiscoveredPluginsRegistry{}

// GetPlugins returns discovered plugins, discovering lazily on first call
// TODO: Consider file-based caching with TTL to avoid manifest fetching on every CLI invocation
func GetPlugins() ([]DiscoveredPlugin, error) {
	registry.once.Do(func() {
		registry.plugins, registry.err = discoverPlugins()
	})

	return registry.plugins, registry.err
}

// TODO: Consider parallel manifest fetching using errgroup for better performance with many PATH directories
func discoverPlugins() ([]DiscoveredPlugin, error) {
	plugins := make([]DiscoveredPlugin, 0)

	seen := make(map[string]bool)

	// 1. Check managed plugins directory first (highest priority)
	managedDir, err := repo.ManagedPluginsDir()
	if err == nil {
		managedPlugins, errs := discoverManagedPlugins(managedDir, seen)
		plugins = append(plugins, managedPlugins...)

		for _, err := range errs {
			log.Debug("Plugin discovery error in managed dir", "dir", managedDir, "error", err)
		}
	}

	// 2. Check project-local directory (higher priority than PATH)
	// TODO: LocalPluginDir shares path with QuickstartScriptPath - consider dedicated plugin directory
	localPlugins, errs := discoverInDir(repo.LocalPluginDir, seen)
	plugins = append(plugins, localPlugins...)

	for _, err := range errs {
		log.Debug("Plugin discovery error in local dir", "dir", repo.LocalPluginDir, "error", err)
	}

	// 3. Check PATH directories
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

// discoverManagedPlugins discovers plugins installed via `dr plugin install`
// These are in subdirectories with a manifest.json and platform-specific scripts
func discoverManagedPlugins(dir string, seen map[string]bool) ([]DiscoveredPlugin, []error) {
	plugins := make([]DiscoveredPlugin, 0)

	var errs []error

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, []error{err}
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		plugin, err := loadManagedPlugin(dir, entry.Name(), seen)
		if err != nil {
			errs = append(errs, err)

			continue
		}

		if plugin != nil {
			plugins = append(plugins, *plugin)
		}
	}

	return plugins, errs
}

func loadManagedPlugin(dir, name string, seen map[string]bool) (*DiscoveredPlugin, error) {
	pluginDir := filepath.Join(dir, name)
	manifestPath := filepath.Join(pluginDir, "manifest.json")

	if _, err := os.Stat(manifestPath); err != nil {
		return nil, nil
	}

	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}

	var manifest PluginManifest

	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, err
	}

	if manifest.Name == "" {
		return nil, errMissingManifestField("name")
	}

	if seen[manifest.Name] {
		log.Warn("Plugin name already registered, skipping",
			"name", manifest.Name,
			"path", pluginDir)

		return nil, nil
	}

	executable, err := resolvePlatformExecutable(pluginDir, &manifest)
	if err != nil {
		return nil, err
	}

	seen[manifest.Name] = true

	return &DiscoveredPlugin{
		Manifest:   manifest,
		Executable: executable,
	}, nil
}

// resolvePlatformExecutable returns the appropriate script path for the current platform
func resolvePlatformExecutable(pluginDir string, manifest *PluginManifest) (string, error) {
	if manifest.Scripts == nil {
		return "", errors.New("managed plugin missing scripts configuration")
	}

	var scriptPath string

	if runtime.GOOS == "windows" {
		scriptPath = manifest.Scripts.Windows
	} else {
		scriptPath = manifest.Scripts.Posix
	}

	if scriptPath == "" {
		return "", errors.New("no script configured for platform: " + runtime.GOOS)
	}

	fullPath := filepath.Join(pluginDir, scriptPath)

	// Verify script exists
	if _, err := os.Stat(fullPath); err != nil {
		return "", err
	}

	return fullPath, nil
}

func errMissingManifestField(field string) error {
	return errors.New("plugin manifest missing required field: " + field)
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

		// Validate plugin is executable by Go runtime
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
			log.Warn("Plugin name already registered, skipping",
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
		// TODO: Wrap error with executable path for better debugging context
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

	// TODO: Validate manifest.Name against a pattern (alphanumeric + hyphens) to prevent confusing command names

	return &manifest, nil
}
