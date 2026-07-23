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
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/repo"
)

// PluginRegistryTerminology is the user-facing term for the plugin registry.
const PluginRegistryTerminology = "registry"

// PluginRegistryURL is the default URL for the remote plugin registry.
const PluginRegistryURL = "https://cli.datarobot.com/plugins/index.json"

// TODO: Consider adding ResetRegistry() for testing, as package-level state makes unit tests harder.
var registry = &DiscoveredPluginsRegistry{}

// GetPlugins returns discovered plugins and any name conflicts found along
// the way, discovering lazily on first call. If PrimeCache already populated
// the registry (e.g. RegisterPluginCommands ran during startup), that result
// is reused instead of discovering again.
// TODO: Consider file-based caching with TTL to avoid manifest fetching on every CLI invocation.
func GetPlugins() ([]DiscoveredPlugin, []PluginConflict) {
	registry.once.Do(func() {
		registry.plugins, registry.conflicts = DiscoverPluginsWithContext(context.Background())
	})

	return registry.plugins, registry.conflicts
}

// PrimeCache seeds the discovery cache with an already-computed plugin list
// and its conflicts, so a later GetPlugins() call reuses them instead of
// discovering again. This is a no-op if GetPlugins() already ran. It exists
// because command registration (RegisterPluginCommands) discovers plugins
// under a user-configurable timeout before any command runs; without priming
// the cache, GetPlugins() would redo that same PATH scan from scratch.
func PrimeCache(plugins []DiscoveredPlugin, conflicts []PluginConflict) {
	registry.once.Do(func() {
		registry.plugins = plugins
		registry.conflicts = conflicts
	})
}

// LogConflicts logs a WARN for each conflict, in the same format previously
// emitted directly by discovery internals. Callers choose which conflicts to
// pass in — e.g. all of them for a full listing, or only those returned by
// ConflictsForName when only one specific plugin was requested.
func LogConflicts(conflicts []PluginConflict) {
	for _, c := range conflicts {
		log.Warn("Plugin name already registered, skipping", "name", c.Name, "path", c.Path)
	}
}

// ConflictsForName filters conflicts down to those matching a single plugin
// name, so callers that only care about one plugin (e.g. `dr plugin version
// <name>`) don't surface warnings about unrelated plugins.
func ConflictsForName(conflicts []PluginConflict, name string) []PluginConflict {
	var matched []PluginConflict

	for _, c := range conflicts {
		if c.Name == name {
			matched = append(matched, c)
		}
	}

	return matched
}

// DiscoverPluginsWithContext discovers all plugins under the given context deadline,
// along with any name conflicts encountered (a plugin skipped because another
// plugin already claimed its manifest name from a higher-priority location).
// Managed and local plugins (file I/O only) always complete before PATH scanning starts,
// so they are always returned even when ctx is cancelled mid-discovery. PATH plugins
// return whatever finished before ctx is done.
func DiscoverPluginsWithContext(ctx context.Context) ([]DiscoveredPlugin, []PluginConflict) {
	plugins := make([]DiscoveredPlugin, 0)

	var conflicts []PluginConflict

	seen := make(map[string]bool)

	// 1. Check managed plugins directories first (highest priority).
	// Includes primary dir (XDG_CONFIG_HOME) and XDG_CONFIG_DIRS if set.
	managedDirs, err := ManagedPluginsDirs()
	if err == nil {
		for _, managedDir := range managedDirs {
			managedPlugins, managedConflicts, errs := discoverManagedPlugins(managedDir, seen)
			plugins = append(plugins, managedPlugins...)
			conflicts = append(conflicts, managedConflicts...)

			for _, err := range errs {
				log.Debug("Plugin discovery error in managed dir", "dir", managedDir, "error", err)
			}
		}
	}

	// 2. Check project-local directory (higher priority than PATH)
	// TODO: LocalPluginDir shares path with QuickstartScriptPath - consider dedicated plugin directory
	localPlugins, localConflicts, errs := discoverInDir(ctx, repo.LocalPluginDir, seen)
	plugins = append(plugins, localPlugins...)
	conflicts = append(conflicts, localConflicts...)

	for _, err := range errs {
		log.Debug("Plugin discovery error in local dir", "dir", repo.LocalPluginDir, "error", err)
	}

	// 3. Check PATH directories in parallel.
	// Snapshot seen after managed+local so each goroutine can filter conflicts
	// without sharing mutable state. Cross-dir dedup is handled inside the helper.
	baseSeen := maps.Clone(seen)

	pathPlugins, pathConflicts := discoverPathDirsParallel(ctx, uniqueDirs(filepath.SplitList(os.Getenv("PATH"))), baseSeen)
	plugins = append(plugins, pathPlugins...)
	conflicts = append(conflicts, pathConflicts...)

	if ctx.Err() != nil {
		log.Warn("Plugin discovery timed out; some PATH plugins may be missing — use --plugin-discovery-timeout to adjust", "discovered", len(plugins))
	}

	log.Debug("Plugin discovery complete", "count", len(plugins))

	return plugins, conflicts
}

// uniqueDirs removes duplicate directory entries from PATH, preserving order.
// PATH commonly lists the same directory more than once (e.g. sourced twice
// by shell rc files); scanning it twice would make discoverPathDirsParallel
// re-report every plugin in that directory as an "already registered"
// conflict with itself.
func uniqueDirs(dirs []string) []string {
	seen := make(map[string]bool, len(dirs))
	unique := make([]string, 0, len(dirs))

	for _, dir := range dirs {
		if seen[dir] {
			continue
		}

		seen[dir] = true

		unique = append(unique, dir)
	}

	return unique
}

// discoverPathDirsParallel runs discoverInDir for each PATH directory concurrently.
// Each goroutine receives its own copy of baseSeen so managed/local plugins are filtered
// without cross-goroutine map races. Goroutines not yet started are skipped when ctx is done.
// Results are merged in directory order and cross-dir duplicates are recorded as conflicts.
func discoverPathDirsParallel(ctx context.Context, pathDirs []string, baseSeen map[string]bool) ([]DiscoveredPlugin, []PluginConflict) {
	type dirResult struct {
		plugins   []DiscoveredPlugin
		conflicts []PluginConflict
		errs      []error
	}

	dirResults := make([]dirResult, len(pathDirs))

	var wg sync.WaitGroup

	for i, dir := range pathDirs {
		localSeen := maps.Clone(baseSeen)

		wg.Go(func() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			// A conflict here means the name was already seen in baseSeen
			// (i.e. claimed by a managed/local plugin); genuine cross-dir
			// PATH conflicts are detected below during the merge step.
			p, c, e := discoverInDir(ctx, dir, localSeen)
			dirResults[i] = dirResult{plugins: p, conflicts: c, errs: e}
		})
	}

	wg.Wait()

	// Merge in directory order; cross-dir dedup starts from baseSeen.
	crossDirSeen := maps.Clone(baseSeen)

	var plugins []DiscoveredPlugin

	var conflicts []PluginConflict

	for i, dr := range dirResults {
		for _, e := range dr.errs {
			log.Debug("Plugin discovery error", "dir", pathDirs[i], "error", e)
		}

		conflicts = append(conflicts, dr.conflicts...)

		for _, p := range dr.plugins {
			if crossDirSeen[p.Manifest.Name] {
				conflicts = append(conflicts, PluginConflict{Name: p.Manifest.Name, Path: p.Executable})

				continue
			}

			crossDirSeen[p.Manifest.Name] = true

			plugins = append(plugins, p)
		}
	}

	return plugins, conflicts
}

// discoverManagedPlugins discovers plugins installed via `dr plugin install`
// These are in subdirectories with a manifest.json and platform-specific scripts.
func discoverManagedPlugins(dir string, seen map[string]bool) ([]DiscoveredPlugin, []PluginConflict, []error) {
	plugins := make([]DiscoveredPlugin, 0)

	var conflicts []PluginConflict

	var errs []error

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, nil, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, []error{err}
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		plugin, conflict, err := loadManagedPlugin(dir, entry.Name(), seen)
		if err != nil {
			errs = append(errs, err)

			continue
		}

		if plugin != nil {
			plugins = append(plugins, *plugin)
		}

		if conflict != nil {
			conflicts = append(conflicts, *conflict)
		}
	}

	return plugins, conflicts, errs
}

func loadManagedPlugin(dir, name string, seen map[string]bool) (*DiscoveredPlugin, *PluginConflict, error) {
	pluginDir := filepath.Join(dir, name)
	manifestPath := filepath.Join(pluginDir, "manifest.json")

	if _, err := os.Stat(manifestPath); err != nil {
		return nil, nil, nil
	}

	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, nil, err
	}

	var manifest PluginManifest

	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, nil, err
	}

	if manifest.Name == "" {
		return nil, nil, errMissingManifestField("name")
	}

	if seen[manifest.Name] {
		return nil, &PluginConflict{Name: manifest.Name, Path: pluginDir}, nil
	}

	executable, err := resolvePlatformExecutable(pluginDir, &manifest)
	if err != nil {
		return nil, nil, err
	}

	seen[manifest.Name] = true

	return &DiscoveredPlugin{
		Manifest:   manifest,
		Executable: executable,
	}, nil, nil
}

// resolvePlatformExecutable returns the appropriate script path for the current platform.
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

func discoverInDir(ctx context.Context, dir string, seen map[string]bool) ([]DiscoveredPlugin, []PluginConflict, []error) {
	// Check if directory exists
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, nil, nil // Directory doesn't exist, not an error
	}

	// Read directory entries
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, []error{err}
	}

	// Phase 1: collect valid executables (fast, no goroutines)
	var executables []string

	for _, entry := range entries {
		name := entry.Name()

		// Must match dr-* pattern
		if !strings.HasPrefix(name, PluginPrefix) {
			continue
		}

		fullPath := filepath.Join(dir, name)

		// Validate plugin is executable by Go runtime
		if _, err := exec.LookPath(fullPath); err != nil {
			log.Debug("Plugin not executable by Go runtime", "path", fullPath, "error", err)

			continue
		}

		executables = append(executables, fullPath)
	}

	// Phase 2 & 3: fetch manifests in parallel and deduplicate on manifest.Name.
	return getManifestsParallel(ctx, executables, seen)
}

// getManifestsParallel calls getManifest concurrently for each executable, then
// deduplicates results against seen in lexicographic (input) order. Preserves the
// "first binary wins" guarantee that os.ReadDir's alphabetical ordering provides.
// Goroutines that have not yet called getManifest are skipped when ctx is done.
func getManifestsParallel(ctx context.Context, executables []string, seen map[string]bool) ([]DiscoveredPlugin, []PluginConflict, []error) {
	type result struct {
		path     string
		manifest *PluginManifest
		err      error
	}

	results := make([]result, len(executables))

	var wg sync.WaitGroup

	for i, fullPath := range executables {
		wg.Go(func() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			manifest, err := getManifest(ctx, fullPath)
			results[i] = result{path: fullPath, manifest: manifest, err: err}
		})
	}

	wg.Wait()

	// Deduplicate on manifest.Name (the actual command name), preserving lexicographic order
	var plugins []DiscoveredPlugin

	var conflicts []PluginConflict

	var errs []error

	for _, r := range results {
		if r.err != nil {
			errs = append(errs, r.err)

			continue
		}

		if r.manifest == nil {
			continue // goroutine skipped due to context cancellation
		}

		if seen[r.manifest.Name] {
			conflicts = append(conflicts, PluginConflict{Name: r.manifest.Name, Path: r.path})

			continue
		}

		seen[r.manifest.Name] = true

		plugins = append(plugins, DiscoveredPlugin{
			Manifest:   *r.manifest,
			Executable: r.path,
		})
	}

	return plugins, conflicts, errs
}

func getManifest(ctx context.Context, executable string) (*PluginManifest, error) {
	// Default timeout if not configured
	timeout := 500 * time.Millisecond
	if viperx.IsSet("plugin.manifest_timeout_ms") {
		timeout = time.Duration(viperx.GetInt("plugin.manifest_timeout_ms")) * time.Millisecond
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, executable, PluginManifestFlag)

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
