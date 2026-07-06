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
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/log"
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
	plugins, errs := discoverInDir(context.Background(), s.tempDir, seen)

	s.Empty(plugins)
	s.Empty(errs)
}

func (s *DiscoverTestSuite) TestDiscoverInDirNonExistent() {
	seen := make(map[string]bool)
	plugins, errs := discoverInDir(context.Background(), filepath.Join(s.tempDir, "nonexistent"), seen)

	s.Nil(plugins)
	s.Nil(errs)
}

func (s *DiscoverTestSuite) TestDiscoverInDirValidPlugin() {
	createMockPlugin(s.T(), s.tempDir, "dr-testplugin", validManifest)

	seen := make(map[string]bool)
	plugins, errs := discoverInDir(context.Background(), s.tempDir, seen)

	s.Require().Len(plugins, 1, "Expected exactly one plugin")
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
	plugins, _ := discoverInDir(context.Background(), s.tempDir, seen)

	s.Require().Len(plugins, 1, "Expected exactly one plugin (non-dr files should be skipped)")
	s.Equal("test-plugin", plugins[0].Manifest.Name)
}

func (s *DiscoverTestSuite) TestDiscoverInDirSkipsNonExecutable() {
	// Create a dr-prefixed file that is not executable
	path := filepath.Join(s.tempDir, "dr-notexec")
	s.Require().NoError(os.WriteFile(path, []byte("not executable"), 0o644))

	seen := make(map[string]bool)
	plugins, errs := discoverInDir(context.Background(), s.tempDir, seen)

	s.Empty(plugins)
	s.Empty(errs)
}

func (s *DiscoverTestSuite) TestDiscoverInDirHandlesDuplicates() {
	createMockPlugin(s.T(), s.tempDir, "dr-duplicate", validManifest)

	// Pre-populate seen map with manifest name (not filename)
	seen := map[string]bool{
		"test-plugin": true, // validManifest has name: "test-plugin"
	}

	plugins, _ := discoverInDir(context.Background(), s.tempDir, seen)

	// Should be skipped due to duplicate manifest name
	s.Empty(plugins)
}

func (s *DiscoverTestSuite) TestDiscoverInDirDeduplicatesByManifestName() {
	// Two different binary names, same manifest name
	manifest := `{"name":"shared-name","version":"1.0.0","description":"Test"}`
	createMockPlugin(s.T(), s.tempDir, "dr-first", manifest)
	createMockPlugin(s.T(), s.tempDir, "dr-second", manifest)

	seen := make(map[string]bool)
	plugins, errs := discoverInDir(context.Background(), s.tempDir, seen)

	// Log manifest-fetch errors so future test failures show *why* discovery
	// returned zero plugins instead of only asserting *that* it did.
	for _, err := range errs {
		log.Debug("Deduplication test manifest error", "error", err)
	}

	// Only one should be registered (first one wins based on directory order)
	s.Require().Len(plugins, 1, "Expected exactly one plugin when two share the same manifest name")
	s.Empty(errs)
	s.Equal("shared-name", plugins[0].Manifest.Name)
}

func (s *DiscoverTestSuite) TestDiscoverInDirInvalidManifest() {
	createMockPlugin(s.T(), s.tempDir, "dr-invalid", invalidManifest)

	seen := make(map[string]bool)
	plugins, errs := discoverInDir(context.Background(), s.tempDir, seen)

	s.Empty(plugins)
	s.Len(errs, 1) // JSON parse error logged
}

func (s *DiscoverTestSuite) TestDiscoverInDirMultipleValidPlugins() {
	manifest1 := `{"name":"plugin-one","version":"1.0.0","description":"First plugin"}`
	manifest2 := `{"name":"plugin-two","version":"2.0.0","description":"Second plugin"}`

	createMockPlugin(s.T(), s.tempDir, "dr-one", manifest1)
	createMockPlugin(s.T(), s.tempDir, "dr-two", manifest2)

	seen := make(map[string]bool)
	plugins, errs := discoverInDir(context.Background(), s.tempDir, seen)

	s.Len(plugins, 2)
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
	plugins, errs := discoverInDir(ctx, s.tempDir, seen)

	s.Empty(plugins)
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
	plugins := discoverPathDirsParallel(context.Background(), []string{}, map[string]bool{})

	s.Empty(plugins)
}

func (s *PathDirsTestSuite) TestMultipleDirsCollectsAll() {
	m1 := `{"name":"plugin-alpha","version":"1.0.0","description":"Alpha"}`
	m2 := `{"name":"plugin-beta","version":"1.0.0","description":"Beta"}`
	createMockPlugin(s.T(), s.dir1, "dr-alpha", m1)
	createMockPlugin(s.T(), s.dir2, "dr-beta", m2)

	plugins := discoverPathDirsParallel(context.Background(), []string{s.dir1, s.dir2}, map[string]bool{})

	s.Len(plugins, 2)

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

	plugins := discoverPathDirsParallel(context.Background(), []string{s.dir1, s.dir2}, map[string]bool{})

	s.Require().Len(plugins, 1)
	s.Equal("shared-plugin", plugins[0].Manifest.Name)
	s.Equal(filepath.Join(s.dir1, "dr-shared"), plugins[0].Executable)
}

func (s *PathDirsTestSuite) TestBaseSeenFiltersPlugins() {
	// validManifest has name "test-plugin" — pre-populate baseSeen so it is skipped.
	createMockPlugin(s.T(), s.dir1, "dr-testplugin", validManifest)

	baseSeen := map[string]bool{"test-plugin": true}
	plugins := discoverPathDirsParallel(context.Background(), []string{s.dir1}, baseSeen)

	s.Empty(plugins)
}

func (s *PathDirsTestSuite) TestBaseSeenNotMutated() {
	// discoverPathDirsParallel must not modify the caller's baseSeen map.
	m1 := `{"name":"plugin-alpha","version":"1.0.0","description":"Alpha"}`
	createMockPlugin(s.T(), s.dir1, "dr-alpha", m1)

	baseSeen := map[string]bool{}
	_ = discoverPathDirsParallel(context.Background(), []string{s.dir1}, baseSeen)

	s.Empty(baseSeen)
}

func (s *PathDirsTestSuite) TestCancelledContextReturnsEmpty() {
	createMockPlugin(s.T(), s.dir1, "dr-alpha", `{"name":"plugin-alpha","version":"1.0.0","description":"Alpha"}`)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	plugins := discoverPathDirsParallel(ctx, []string{s.dir1}, map[string]bool{})

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

	plugins := discoverPathDirsParallel(ctx, []string{s.dir1, s.dir2}, map[string]bool{})

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
