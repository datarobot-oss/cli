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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// Test manifests
var (
	validManifest   = `{"name":"test-plugin","version":"1.0.0","description":"Test plugin"}`
	invalidManifest = `{invalid json`
)

// createMockPlugin creates a shell script that responds to --dr-plugin-manifest
func createMockPlugin(t *testing.T, dir, name, manifestJSON string) string {
	t.Helper()

	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "--dr-plugin-manifest" ]; then
  echo '%s'
else
  exit 0
fi
`, manifestJSON)

	path := filepath.Join(dir, name)
	err := os.WriteFile(path, []byte(script), 0o755)
	require.NoError(t, err)

	return path
}

// DiscoverTestSuite tests discovery functions with filesystem fixtures
type DiscoverTestSuite struct {
	suite.Suite
	tempDir string
}

func TestDiscoverTestSuite(t *testing.T) {
	suite.Run(t, new(DiscoverTestSuite))
}

func (s *DiscoverTestSuite) SetupTest() {
	var err error

	s.tempDir, err = os.MkdirTemp("", "plugin-discover-test")
	s.Require().NoError(err)

	// Reset viper and raise the manifest timeout so subprocess calls
	// do not fail under the heavy load of -race / -shuffle test runs.
	viperx.Reset()
	viperx.Set("plugin.manifest_timeout_ms", 5000)
}

func (s *DiscoverTestSuite) TearDownTest() {
	if s.tempDir != "" {
		_ = os.RemoveAll(s.tempDir)
	}

	// Clear any test-specific viper state so it does not leak to other suites.
	viperx.Reset()
}

func (s *DiscoverTestSuite) TestDiscoverInDirEmptyDirectory() {
	seen := make(map[string]bool)
	plugins, conflicts, errs := discoverInDir(context.Background(), s.tempDir, seen)

	s.Empty(plugins)
	s.Empty(conflicts)
	s.Empty(errs)
}

func (s *DiscoverTestSuite) TestDiscoverInDirNonExistent() {
	seen := make(map[string]bool)
	plugins, conflicts, errs := discoverInDir(context.Background(), filepath.Join(s.tempDir, "nonexistent"), seen)

	s.Nil(plugins)
	s.Nil(conflicts)
	s.Nil(errs)
}

func (s *DiscoverTestSuite) TestDiscoverInDirValidPlugin() {
	createMockPlugin(s.T(), s.tempDir, "dr-testplugin", validManifest)

	seen := make(map[string]bool)
	plugins, conflicts, errs := discoverInDir(context.Background(), s.tempDir, seen)

	s.Require().Len(plugins, 1, "Expected exactly one plugin")
	s.Empty(conflicts)
	s.Empty(errs)
	s.Equal("test-plugin", plugins[0].Manifest.Name)
	s.Equal("1.0.0", plugins[0].Manifest.Version)
	s.Equal("Test plugin", plugins[0].Manifest.Description)
	s.True(seen["test-plugin"]) // seen map now uses manifest.Name
}

func (s *DiscoverTestSuite) TestDiscoverInDirSkipsNonDrFiles() {
	// Create a valid plugin
	createMockPlugin(s.T(), s.tempDir, "dr-valid", validManifest)

	// Create non-dr files
	s.Require().NoError(os.WriteFile(filepath.Join(s.tempDir, "other-binary"), []byte("#!/bin/sh\nexit 0"), 0o755))
	s.Require().NoError(os.WriteFile(filepath.Join(s.tempDir, "random.txt"), []byte("text"), 0o644))

	seen := make(map[string]bool)
	plugins, _, _ := discoverInDir(context.Background(), s.tempDir, seen)

	s.Require().Len(plugins, 1, "Expected exactly one plugin (non-dr files should be skipped)")
	s.Equal("test-plugin", plugins[0].Manifest.Name)
}

func (s *DiscoverTestSuite) TestDiscoverInDirSkipsNonExecutable() {
	// Create a dr-prefixed file that is not executable
	path := filepath.Join(s.tempDir, "dr-notexec")
	s.Require().NoError(os.WriteFile(path, []byte("not executable"), 0o644))

	seen := make(map[string]bool)
	plugins, conflicts, errs := discoverInDir(context.Background(), s.tempDir, seen)

	s.Empty(plugins)
	s.Empty(conflicts)
	s.Empty(errs)
}

func (s *DiscoverTestSuite) TestDiscoverInDirHandlesDuplicates() {
	createMockPlugin(s.T(), s.tempDir, "dr-duplicate", validManifest)

	// Pre-populate seen map with manifest name (not filename)
	seen := map[string]bool{
		"test-plugin": true, // validManifest has name: "test-plugin"
	}

	plugins, conflicts, _ := discoverInDir(context.Background(), s.tempDir, seen)

	// Should be skipped due to duplicate manifest name
	s.Empty(plugins)
	s.Require().Len(conflicts, 1, "pre-existing manifest name must be recorded as a conflict")
	s.Equal("test-plugin", conflicts[0].Name)
}

func (s *DiscoverTestSuite) TestDiscoverInDirDeduplicatesByManifestName() {
	// Two different binary names, same manifest name
	manifest := `{"name":"shared-name","version":"1.0.0","description":"Test"}`
	createMockPlugin(s.T(), s.tempDir, "dr-first", manifest)
	createMockPlugin(s.T(), s.tempDir, "dr-second", manifest)

	seen := make(map[string]bool)
	plugins, conflicts, errs := discoverInDir(context.Background(), s.tempDir, seen)

	// Log manifest-fetch errors so future test failures show *why* discovery
	// returned zero plugins instead of only asserting *that* it did.
	for _, err := range errs {
		log.Debug("Deduplication test manifest error", "error", err)
	}

	// Only one should be registered (first one wins based on directory order)
	s.Require().Len(plugins, 1, "Expected exactly one plugin when two share the same manifest name")
	s.Empty(errs)
	s.Equal("shared-name", plugins[0].Manifest.Name)
	s.Require().Len(conflicts, 1, "the second binary sharing the manifest name must be recorded as a conflict")
	s.Equal("shared-name", conflicts[0].Name)
}

func (s *DiscoverTestSuite) TestDiscoverInDirInvalidManifest() {
	createMockPlugin(s.T(), s.tempDir, "dr-invalid", invalidManifest)

	seen := make(map[string]bool)
	plugins, conflicts, errs := discoverInDir(context.Background(), s.tempDir, seen)

	s.Empty(plugins)
	s.Empty(conflicts)
	s.Len(errs, 1) // JSON parse error logged
}

func (s *DiscoverTestSuite) TestDiscoverInDirMultipleValidPlugins() {
	manifest1 := `{"name":"plugin-one","version":"1.0.0","description":"First plugin"}`
	manifest2 := `{"name":"plugin-two","version":"2.0.0","description":"Second plugin"}`

	createMockPlugin(s.T(), s.tempDir, "dr-one", manifest1)
	createMockPlugin(s.T(), s.tempDir, "dr-two", manifest2)

	seen := make(map[string]bool)
	plugins, conflicts, errs := discoverInDir(context.Background(), s.tempDir, seen)

	s.Len(plugins, 2)
	s.Empty(conflicts)
	s.Empty(errs)

	// Verify both plugins discovered
	names := make(map[string]bool)
	for _, p := range plugins {
		names[p.Manifest.Name] = true
	}

	s.True(names["plugin-one"])
	s.True(names["plugin-two"])
}

func (s *DiscoverTestSuite) TestDiscoverInDirCancelledContext() {
	createMockPlugin(s.T(), s.tempDir, "dr-testplugin", validManifest)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	seen := make(map[string]bool)
	plugins, conflicts, errs := discoverInDir(ctx, s.tempDir, seen)

	s.Empty(plugins)
	s.Empty(conflicts)
	s.Empty(errs)
}

// PathDirsTestSuite tests discoverPathDirsParallel
type PathDirsTestSuite struct {
	suite.Suite
	dir1 string
	dir2 string
}

func TestPathDirsTestSuite(t *testing.T) {
	suite.Run(t, new(PathDirsTestSuite))
}

func (s *PathDirsTestSuite) SetupTest() {
	var err error

	s.dir1, err = os.MkdirTemp("", "plugin-path-dir1")
	s.Require().NoError(err)

	s.dir2, err = os.MkdirTemp("", "plugin-path-dir2")
	s.Require().NoError(err)

	viperx.Reset()
	viperx.Set("plugin.manifest_timeout_ms", 5000)
}

func (s *PathDirsTestSuite) TearDownTest() {
	_ = os.RemoveAll(s.dir1)
	_ = os.RemoveAll(s.dir2)

	viperx.Reset()
}

func (s *PathDirsTestSuite) TestEmptyPathDirs() {
	plugins, conflicts := discoverPathDirsParallel(context.Background(), []string{}, map[string]bool{})

	s.Empty(plugins)
	s.Empty(conflicts)
}

func (s *PathDirsTestSuite) TestMultipleDirsCollectsAll() {
	m1 := `{"name":"plugin-alpha","version":"1.0.0","description":"Alpha"}`
	m2 := `{"name":"plugin-beta","version":"1.0.0","description":"Beta"}`

	createMockPlugin(s.T(), s.dir1, "dr-alpha", m1)
	createMockPlugin(s.T(), s.dir2, "dr-beta", m2)

	plugins, conflicts := discoverPathDirsParallel(context.Background(), []string{s.dir1, s.dir2}, map[string]bool{})

	s.Len(plugins, 2)
	s.Empty(conflicts)

	names := make(map[string]bool)
	for _, p := range plugins {
		names[p.Manifest.Name] = true
	}

	s.True(names["plugin-alpha"])
	s.True(names["plugin-beta"])
}

func (s *PathDirsTestSuite) TestCrossDirDeduplicationFirstDirWins() {
	// Same manifest name in both dirs — dir1 must win because results are
	// merged in directory order after all goroutines complete.
	manifest := `{"name":"shared-plugin","version":"1.0.0","description":"Shared"}`
	createMockPlugin(s.T(), s.dir1, "dr-shared", manifest)
	createMockPlugin(s.T(), s.dir2, "dr-shared", manifest)

	plugins, conflicts := discoverPathDirsParallel(context.Background(), []string{s.dir1, s.dir2}, map[string]bool{})

	s.Require().Len(plugins, 1)
	s.Equal("shared-plugin", plugins[0].Manifest.Name)
	s.Equal(filepath.Join(s.dir1, "dr-shared"), plugins[0].Executable)

	s.Require().Len(conflicts, 1, "the second dir's binary sharing the manifest name must be recorded as a conflict")
	s.Equal("shared-plugin", conflicts[0].Name)
	s.Equal(filepath.Join(s.dir2, "dr-shared"), conflicts[0].Path)
}

func (s *PathDirsTestSuite) TestBaseSeenFiltersPlugins() {
	// validManifest has name "test-plugin" — pre-populate baseSeen so it is skipped.
	createMockPlugin(s.T(), s.dir1, "dr-testplugin", validManifest)

	baseSeen := map[string]bool{"test-plugin": true}
	plugins, conflicts := discoverPathDirsParallel(context.Background(), []string{s.dir1}, baseSeen)

	s.Empty(plugins)
	s.Require().Len(conflicts, 1)
	s.Equal("test-plugin", conflicts[0].Name)
}

func (s *PathDirsTestSuite) TestBaseSeenNotMutated() {
	// discoverPathDirsParallel must not modify the caller's baseSeen map.
	m1 := `{"name":"plugin-alpha","version":"1.0.0","description":"Alpha"}`
	createMockPlugin(s.T(), s.dir1, "dr-alpha", m1)

	baseSeen := map[string]bool{}
	_, _ = discoverPathDirsParallel(context.Background(), []string{s.dir1}, baseSeen)

	s.Empty(baseSeen)
}

func (s *PathDirsTestSuite) TestCancelledContextReturnsEmpty() {
	createMockPlugin(s.T(), s.dir1, "dr-alpha", `{"name":"plugin-alpha","version":"1.0.0","description":"Alpha"}`)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	plugins, _ := discoverPathDirsParallel(ctx, []string{s.dir1}, map[string]bool{})

	s.Empty(plugins)
}

func (s *PathDirsTestSuite) TestPartialResultsWhenContextExpiresAfterFastPlugin() {
	// dir1: fast plugin — responds immediately.
	createMockPlugin(s.T(), s.dir1, "dr-fast", `{"name":"fast-plugin","version":"1.0.0","description":"Fast"}`)

	// dir2: slow plugin — blocks well beyond the context deadline before emitting its manifest.
	// Use "exec sleep" so the shell is replaced by sleep itself: Go's SIGKILL lands on the
	// sleep process directly, stdout closes immediately, and cmd.Output() returns without
	// waiting for an orphaned child to finish.
	slowScript := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"%s\" ]; then\n  exec sleep 30\nfi\n", PluginManifestFlag)
	s.Require().NoError(os.WriteFile(filepath.Join(s.dir2, "dr-slow"), []byte(slowScript), 0o755))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	plugins, _ := discoverPathDirsParallel(ctx, []string{s.dir1, s.dir2}, map[string]bool{})

	names := make(map[string]bool)
	for _, p := range plugins {
		names[p.Manifest.Name] = true
	}

	s.True(names["fast-plugin"], "fast plugin should be discovered before deadline")
	s.False(names["slow-plugin"], "slow plugin should be skipped after deadline expires")
}

// TestGetManifest tests manifest retrieval
type ManifestTestSuite struct {
	suite.Suite
	tempDir string
}

func TestManifestTestSuite(t *testing.T) {
	suite.Run(t, new(ManifestTestSuite))
}

func (s *ManifestTestSuite) SetupTest() {
	var err error

	s.tempDir, err = os.MkdirTemp("", "plugin-manifest-test")
	s.Require().NoError(err)

	// Reset viper state for each test
	viperx.Reset()
}

func (s *ManifestTestSuite) TearDownTest() {
	if s.tempDir != "" {
		_ = os.RemoveAll(s.tempDir)
	}

	viperx.Reset()
}

func (s *ManifestTestSuite) TestGetManifestValid() {
	path := createMockPlugin(s.T(), s.tempDir, "dr-test", validManifest)

	manifest, err := getManifest(context.Background(), path)
	s.Require().NoError(err)
	s.NotNil(manifest)
	s.Equal("test-plugin", manifest.Name)
	s.Equal("1.0.0", manifest.Version)
	s.Equal("Test plugin", manifest.Description)
}

func (s *ManifestTestSuite) TestGetManifestInvalidJSON() {
	path := createMockPlugin(s.T(), s.tempDir, "dr-invalid", invalidManifest)

	manifest, err := getManifest(context.Background(), path)
	s.Require().Error(err)
	s.Nil(manifest)
}

func (s *ManifestTestSuite) TestGetManifestNonExistent() {
	manifest, err := getManifest(context.Background(), filepath.Join(s.tempDir, "nonexistent"))
	s.Require().Error(err)
	s.Nil(manifest)
}

func (s *ManifestTestSuite) TestGetManifestWithConfiguredTimeout() {
	// Set a custom timeout via viper
	viperx.Set("plugin.manifest_timeout_ms", 1000)

	path := createMockPlugin(s.T(), s.tempDir, "dr-test", validManifest)

	manifest, err := getManifest(context.Background(), path)
	s.Require().NoError(err)
	s.NotNil(manifest)
}

func (s *ManifestTestSuite) TestGetManifestCommandFailure() {
	// Create a script that exits with error
	script := `#!/bin/sh
exit 1
`
	path := filepath.Join(s.tempDir, "dr-failing")
	s.Require().NoError(os.WriteFile(path, []byte(script), 0o755))

	manifest, err := getManifest(context.Background(), path)
	s.Require().Error(err)
	s.Nil(manifest)
}

// captureLogOutput redirects os.Stderr to a pipe, reinitialises the logger,
// runs fn, then returns everything written during fn's execution.
func captureLogOutput(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)

	origStderr := os.Stderr
	os.Stderr = w

	log.StartStderr()

	fn()

	w.Close()

	os.Stderr = origStderr

	t.Cleanup(log.StopStderr)

	var buf bytes.Buffer

	_, err = buf.ReadFrom(r)
	require.NoError(t, err)

	r.Close()

	return buf.String()
}

// pluginByName returns the first plugin with the given manifest name, or nil.
func pluginByName(plugins []DiscoveredPlugin, name string) *DiscoveredPlugin {
	for i := range plugins {
		if plugins[i].Manifest.Name == name {
			return &plugins[i]
		}
	}

	return nil
}

// DiscoverWithContextSuite tests DiscoverPluginsWithContext end-to-end.
// Tests control the PATH env var so only known plugins are discovered from PATH.
// Managed-plugin and local-plugin directories are not controlled here and are
// typically empty in CI.
type DiscoverWithContextSuite struct {
	suite.Suite
	pluginDir string
}

func TestDiscoverWithContextSuite(t *testing.T) {
	suite.Run(t, new(DiscoverWithContextSuite))
}

func (s *DiscoverWithContextSuite) SetupTest() {
	var err error

	s.pluginDir, err = os.MkdirTemp("", "plugin-discoverctx-test")
	s.Require().NoError(err)

	viperx.Reset()
	viperx.Set("plugin.manifest_timeout_ms", 5000)
}

func (s *DiscoverWithContextSuite) TearDownTest() {
	_ = os.RemoveAll(s.pluginDir)

	viperx.Reset()
}

func (s *DiscoverWithContextSuite) TestDiscoversPATHPlugin() {
	createMockPlugin(s.T(), s.pluginDir, "dr-ctx-alpha",
		`{"name":"ctx-alpha","version":"1.0.0","description":"Alpha"}`)
	s.T().Setenv("PATH", s.pluginDir)

	plugins, _ := DiscoverPluginsWithContext(context.Background())

	s.NotNil(pluginByName(plugins, "ctx-alpha"), "plugin from controlled PATH dir must be discovered")
}

func (s *DiscoverWithContextSuite) TestPartialResultsOnTimeout() {
	// Fast plugin: responds immediately.
	createMockPlugin(s.T(), s.pluginDir, "dr-ctx-fast",
		`{"name":"ctx-fast","version":"1.0.0","description":"Fast"}`)

	// Slow plugin: blocks well past any reasonable deadline.
	// exec replaces the shell so SIGKILL from exec.CommandContext lands directly
	// on sleep, closing stdout immediately without orphan processes.
	slowDir, err := os.MkdirTemp("", "plugin-discoverctx-slow")
	s.Require().NoError(err)

	defer os.RemoveAll(slowDir)

	slowScript := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"%s\" ]; then\n  exec sleep 30\nfi\n", PluginManifestFlag)
	s.Require().NoError(os.WriteFile(filepath.Join(slowDir, "dr-ctx-slow"), []byte(slowScript), 0o755))

	s.T().Setenv("PATH", s.pluginDir+string(os.PathListSeparator)+slowDir)

	// 2 s gives the fast plugin plenty of time to respond; the slow plugin is
	// killed by the per-manifest timeout (5 s set in SetupTest capped by the
	// outer ctx) or the outer ctx itself — either way it is absent.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	plugins, _ := DiscoverPluginsWithContext(ctx)

	s.NotNil(pluginByName(plugins, "ctx-fast"), "fast plugin must be returned before deadline")
	s.Nil(pluginByName(plugins, "ctx-slow"), "slow plugin must be absent after deadline")
}

func (s *DiscoverWithContextSuite) TestTimeoutLogsWarn() {
	// A pre-cancelled context immediately satisfies ctx.Err() != nil after
	// discoverPathDirsParallel returns (goroutines bail on the first Done check),
	// which is the condition that triggers the WARN — no real-time dependency.
	createMockPlugin(s.T(), s.pluginDir, "dr-ctx-warn",
		`{"name":"ctx-warn","version":"1.0.0","description":"Warn"}`)
	s.T().Setenv("PATH", s.pluginDir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	output := captureLogOutput(s.T(), func() {
		DiscoverPluginsWithContext(ctx)
	})

	s.Contains(output, "timed out")
	s.Contains(output, "--plugin-discovery-timeout")
}

func (s *DiscoverWithContextSuite) TestCancelledContextSkipsPATHPlugins() {
	createMockPlugin(s.T(), s.pluginDir, "dr-ctx-skip",
		`{"name":"ctx-skip","version":"1.0.0","description":"Skip"}`)
	s.T().Setenv("PATH", s.pluginDir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	plugins, _ := DiscoverPluginsWithContext(ctx)

	s.Nil(pluginByName(plugins, "ctx-skip"), "cancelled context must skip PATH plugins")
}

func (s *DiscoverWithContextSuite) TestDuplicatePATHEntryDoesNotWarn() {
	createMockPlugin(s.T(), s.pluginDir, "dr-ctx-dup",
		`{"name":"ctx-dup","version":"1.0.0","description":"Dup"}`)
	// The same directory listed twice in PATH used to make discovery scan it
	// twice, reporting a false conflict against itself.
	s.T().Setenv("PATH", s.pluginDir+string(os.PathListSeparator)+s.pluginDir)

	plugins, conflicts := DiscoverPluginsWithContext(context.Background())

	s.Empty(conflicts, "duplicate PATH entries for the same dir must not produce a conflict")

	matches := 0

	for _, p := range plugins {
		if p.Manifest.Name == "ctx-dup" {
			matches++
		}
	}

	s.Equal(1, matches, "plugin must only appear once even though its dir is listed twice in PATH")
}

// createManagedTestPlugin creates a minimal managed plugin directory structure under pluginsDir.
func createManagedTestPlugin(t *testing.T, pluginsDir, dirName, pluginName string) {
	t.Helper()

	pluginDir := filepath.Join(pluginsDir, dirName)

	require.NoError(t, os.MkdirAll(filepath.Join(pluginDir, "scripts"), 0o755))

	manifestJSON := fmt.Sprintf(
		`{"name":%q,"version":"1.0.0","scripts":{"posix":"scripts/run.sh","windows":"scripts/run.ps1"}}`,
		pluginName,
	)

	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "manifest.json"), []byte(manifestJSON), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "scripts", "run.sh"), []byte("#!/bin/sh\nexit 0\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "scripts", "run.ps1"), []byte("exit 0"), 0o644))
}

func TestDiscoverPlugins_FindsPluginsInXDGConfigDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME env override is Unix-specific")
	}

	tmpHome := t.TempDir()
	tmpXDG := t.TempDir()
	tmpConfigDir := t.TempDir()

	t.Setenv("HOME", tmpHome)
	testutil.SetXDGEnv(t, "XDG_CONFIG_HOME", tmpXDG)
	testutil.SetXDGEnv(t, "XDG_CONFIG_DIRS", tmpConfigDir)

	viperx.Reset()
	viperx.Set("plugin.manifest_timeout_ms", 5000)

	xdgPluginsDir := filepath.Join(tmpXDG, "datarobot", "plugins")
	configDirPluginsDir := filepath.Join(tmpConfigDir, "datarobot", "plugins")

	createManagedTestPlugin(t, xdgPluginsDir, "xdg-plugin", "xdg-plugin")
	createManagedTestPlugin(t, configDirPluginsDir, "config-dir-plugin", "config-dir-plugin")

	plugins, _ := DiscoverPluginsWithContext(context.Background())

	names := make(map[string]bool)

	for _, p := range plugins {
		names[p.Manifest.Name] = true
	}

	assert.True(t, names["xdg-plugin"], "plugin in XDG_CONFIG_HOME directory should be discovered")
	assert.True(t, names["config-dir-plugin"], "plugin in XDG_CONFIG_DIRS directory should be discovered")
}

func TestUniqueDirs(t *testing.T) {
	assert.Equal(t, []string{"/a", "/b", "/c"}, uniqueDirs([]string{"/a", "/b", "/a", "/c", "/b"}))
	assert.Empty(t, uniqueDirs(nil))
}

func TestLogConflicts(t *testing.T) {
	output := captureLogOutput(t, func() {
		LogConflicts([]PluginConflict{
			{Name: "potato", Path: "/usr/local/bin/dr-potato"},
			{Name: "carrot", Path: "/opt/bin/dr-carrot"},
		})
	})

	assert.Contains(t, output, "potato")
	assert.Contains(t, output, "/usr/local/bin/dr-potato")
	assert.Contains(t, output, "carrot")
	assert.Contains(t, output, "/opt/bin/dr-carrot")
}

func TestLogConflictsEmpty(t *testing.T) {
	output := captureLogOutput(t, func() {
		LogConflicts(nil)
	})

	assert.Empty(t, output)
}

func TestConflictsForName(t *testing.T) {
	conflicts := []PluginConflict{
		{Name: "potato", Path: "/usr/local/bin/dr-potato"},
		{Name: "carrot", Path: "/opt/bin/dr-carrot"},
		{Name: "potato", Path: "/opt/bin/dr-potato"},
	}

	assert.Equal(t, []PluginConflict{
		{Name: "potato", Path: "/usr/local/bin/dr-potato"},
		{Name: "potato", Path: "/opt/bin/dr-potato"},
	}, ConflictsForName(conflicts, "potato"))

	assert.Empty(t, ConflictsForName(conflicts, "turnip"))
	assert.Empty(t, ConflictsForName(nil, "potato"))
}
