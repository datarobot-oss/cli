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
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/viper"
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

// TestIsExecutable tests the isExecutable function
func TestIsExecutable(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "plugin-exec-test")
	require.NoError(t, err)

	defer os.RemoveAll(tempDir)

	t.Run("non-existent file", func(t *testing.T) {
		result := isExecutable(filepath.Join(tempDir, "nonexistent"))
		require.False(t, result)
	})

	if runtime.GOOS != "windows" {
		t.Run("unix file with execute bit", func(t *testing.T) {
			path := filepath.Join(tempDir, "executable")
			err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0"), 0o755)
			require.NoError(t, err)

			result := isExecutable(path)
			require.True(t, result)
		})

		t.Run("unix file without execute bit", func(t *testing.T) {
			path := filepath.Join(tempDir, "non-executable")
			err := os.WriteFile(path, []byte("just a file"), 0o644)
			require.NoError(t, err)

			result := isExecutable(path)
			require.False(t, result)
		})
	}

	if runtime.GOOS == "windows" {
		for _, ext := range []string{".exe", ".bat", ".cmd", ".ps1"} {
			t.Run("windows "+ext, func(t *testing.T) {
				path := filepath.Join(tempDir, "test"+ext)
				err := os.WriteFile(path, []byte("test"), 0o644)
				require.NoError(t, err)

				result := isExecutable(path)
				require.True(t, result)
			})
		}

		t.Run("windows non-executable extension", func(t *testing.T) {
			path := filepath.Join(tempDir, "test.txt")
			err := os.WriteFile(path, []byte("test"), 0o644)
			require.NoError(t, err)

			result := isExecutable(path)
			require.False(t, result)
		})
	}
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
}

func (s *DiscoverTestSuite) TearDownTest() {
	if s.tempDir != "" {
		_ = os.RemoveAll(s.tempDir)
	}
}

func (s *DiscoverTestSuite) TestDiscoverInDirEmptyDirectory() {
	seen := make(map[string]bool)
	plugins, errs := discoverInDir(s.tempDir, seen)

	s.Empty(plugins)
	s.Empty(errs)
}

func (s *DiscoverTestSuite) TestDiscoverInDirNonExistent() {
	seen := make(map[string]bool)
	plugins, errs := discoverInDir(filepath.Join(s.tempDir, "nonexistent"), seen)

	s.Nil(plugins)
	s.Nil(errs)
}

func (s *DiscoverTestSuite) TestDiscoverInDirValidPlugin() {
	createMockPlugin(s.T(), s.tempDir, "dr-testplugin", validManifest)

	seen := make(map[string]bool)
	plugins, errs := discoverInDir(s.tempDir, seen)

	s.Len(plugins, 1)
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
	plugins, _ := discoverInDir(s.tempDir, seen)

	s.Len(plugins, 1)
	s.Equal("test-plugin", plugins[0].Manifest.Name)
}

func (s *DiscoverTestSuite) TestDiscoverInDirSkipsNonExecutable() {
	// Create a dr-prefixed file that is not executable
	path := filepath.Join(s.tempDir, "dr-notexec")
	s.Require().NoError(os.WriteFile(path, []byte("not executable"), 0o644))

	seen := make(map[string]bool)
	plugins, errs := discoverInDir(s.tempDir, seen)

	s.Empty(plugins)
	s.Empty(errs)
}

func (s *DiscoverTestSuite) TestDiscoverInDirHandlesDuplicates() {
	createMockPlugin(s.T(), s.tempDir, "dr-duplicate", validManifest)

	// Pre-populate seen map with manifest name (not filename)
	seen := map[string]bool{
		"test-plugin": true, // validManifest has name: "test-plugin"
	}

	plugins, _ := discoverInDir(s.tempDir, seen)

	// Should be skipped due to duplicate manifest name
	s.Empty(plugins)
}

func (s *DiscoverTestSuite) TestDiscoverInDirDeduplicatesByManifestName() {
	// Two different binary names, same manifest name
	manifest := `{"name":"shared-name","version":"1.0.0","description":"Test"}`
	createMockPlugin(s.T(), s.tempDir, "dr-first", manifest)
	createMockPlugin(s.T(), s.tempDir, "dr-second", manifest)

	seen := make(map[string]bool)
	plugins, errs := discoverInDir(s.tempDir, seen)

	// Only one should be registered (first one wins based on directory order)
	s.Len(plugins, 1)
	s.Empty(errs)
	s.Equal("shared-name", plugins[0].Manifest.Name)
}

func (s *DiscoverTestSuite) TestDiscoverInDirInvalidManifest() {
	createMockPlugin(s.T(), s.tempDir, "dr-invalid", invalidManifest)

	seen := make(map[string]bool)
	plugins, errs := discoverInDir(s.tempDir, seen)

	s.Empty(plugins)
	s.Len(errs, 1) // JSON parse error logged
}

func (s *DiscoverTestSuite) TestDiscoverInDirMultipleValidPlugins() {
	manifest1 := `{"name":"plugin-one","version":"1.0.0","description":"First plugin"}`
	manifest2 := `{"name":"plugin-two","version":"2.0.0","description":"Second plugin"}`

	createMockPlugin(s.T(), s.tempDir, "dr-one", manifest1)
	createMockPlugin(s.T(), s.tempDir, "dr-two", manifest2)

	seen := make(map[string]bool)
	plugins, errs := discoverInDir(s.tempDir, seen)

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
	viper.Reset()
}

func (s *ManifestTestSuite) TearDownTest() {
	if s.tempDir != "" {
		_ = os.RemoveAll(s.tempDir)
	}

	viper.Reset()
}

func (s *ManifestTestSuite) TestGetManifestValid() {
	path := createMockPlugin(s.T(), s.tempDir, "dr-test", validManifest)

	manifest, err := getManifest(path)
	s.Require().NoError(err)
	s.NotNil(manifest)
	s.Equal("test-plugin", manifest.Name)
	s.Equal("1.0.0", manifest.Version)
	s.Equal("Test plugin", manifest.Description)
}

func (s *ManifestTestSuite) TestGetManifestInvalidJSON() {
	path := createMockPlugin(s.T(), s.tempDir, "dr-invalid", invalidManifest)

	manifest, err := getManifest(path)
	s.Require().Error(err)
	s.Nil(manifest)
}

func (s *ManifestTestSuite) TestGetManifestNonExistent() {
	manifest, err := getManifest(filepath.Join(s.tempDir, "nonexistent"))
	s.Require().Error(err)
	s.Nil(manifest)
}

func (s *ManifestTestSuite) TestGetManifestWithConfiguredTimeout() {
	// Set a custom timeout via viper
	viper.Set("plugin.manifest_timeout_ms", 1000)

	path := createMockPlugin(s.T(), s.tempDir, "dr-test", validManifest)

	manifest, err := getManifest(path)
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

	manifest, err := getManifest(path)
	s.Require().Error(err)
	s.Nil(manifest)
}
