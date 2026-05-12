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

package task

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func TestDiscoveryTestSuite(t *testing.T) {
	suite.Run(t, new(DiscoveryTestSuite))
}

type DiscoveryTestSuite struct {
	suite.Suite
	tempDir string
}

func (suite *DiscoveryTestSuite) SetupTest() {
	dir, err := os.MkdirTemp("", "task_discovery_test")
	suite.Require().NoError(err)
	suite.tempDir = dir
}

func (suite *DiscoveryTestSuite) TearDownTest() {
	os.RemoveAll(suite.tempDir)
}

func TestDepth(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected int
	}{
		{
			name:     "current directory",
			path:     ".",
			expected: 0,
		},
		{
			name:     "single level",
			path:     "dir",
			expected: 1,
		},
		{
			name:     "two levels",
			path:     "dir/subdir",
			expected: 2,
		},
		{
			name:     "three levels",
			path:     "dir/subdir/nested",
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := depth(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func (suite *DiscoveryTestSuite) TestFindComponentsNormalizesWindowsPaths() {
	discovery := &Discovery{
		RootTaskfileName: "Taskfile.yaml",
	}

	// Create nested taskfile structure:
	//   tempDir/
	//   ├── component1/
	//   │   └── taskfile.yaml
	//   └── nested/
	//       └── component2/
	//           └── taskfile.yaml
	comp1Dir := filepath.Join(suite.tempDir, "component1")
	comp2Dir := filepath.Join(suite.tempDir, "nested", "component2")

	suite.Require().NoError(os.MkdirAll(comp1Dir, os.ModePerm))
	suite.Require().NoError(os.MkdirAll(comp2Dir, os.ModePerm))

	// Each taskfile.yaml contains:
	// tasks:
	//   test:
	//     cmds:
	//       - echo test
	comp1File := filepath.Join(comp1Dir, "taskfile.yaml")
	comp2File := filepath.Join(comp2Dir, "taskfile.yaml")

	taskfileContent := "tasks:\n  test:\n    cmds:\n      - echo test\n"
	suite.Require().NoError(os.WriteFile(comp1File, []byte(taskfileContent), 0o644))
	suite.Require().NoError(os.WriteFile(comp2File, []byte(taskfileContent), 0o644))

	includes, err := discovery.findComponents(suite.tempDir, 5)

	suite.Require().NoError(err)
	suite.Len(includes, 2)

	// Verify that paths use forward slashes, not backslashes
	for _, include := range includes {
		suite.NotContains(include.Taskfile, "\\", "Taskfile path should not contain backslashes")
		suite.NotContains(include.Dir, "\\", "Dir path should not contain backslashes")
		suite.True(filepath.IsAbs(include.Taskfile) || include.Taskfile[0] == '.', "Path should be relative or absolute")
	}
}

func (suite *DiscoveryTestSuite) TestFindComponentsRespectsMaxDepth() {
	discovery := &Discovery{
		RootTaskfileName: "Taskfile.yaml",
	}

	// Create nested structure with different depths:
	//   tempDir/
	//   ├── shallow/
	//   │   └── taskfile.yaml (depth 1)
	//   └── a/b/c/d/
	//       └── taskfile.yaml (depth 4)
	shallow := filepath.Join(suite.tempDir, "shallow")
	deep := filepath.Join(suite.tempDir, "a", "b", "c", "d")

	suite.Require().NoError(os.MkdirAll(shallow, os.ModePerm))
	suite.Require().NoError(os.MkdirAll(deep, os.ModePerm))

	// Each taskfile.yaml contains minimal structure:
	// tasks: {}
	suite.Require().NoError(os.WriteFile(filepath.Join(shallow, "taskfile.yaml"), []byte("tasks: {}"), 0o644))
	suite.Require().NoError(os.WriteFile(filepath.Join(deep, "taskfile.yaml"), []byte("tasks: {}"), 0o644))

	// Test with maxDepth=2 - should find shallow (depth 1) but not deep (depth 4)
	includes, err := discovery.findComponents(suite.tempDir, 2)
	suite.Require().NoError(err)

	// Should only find the shallow component
	suite.Len(includes, 1)
	suite.Equal("shallow", includes[0].Name)
}

func (suite *DiscoveryTestSuite) TestFindComponentsSkipsHiddenDirs() {
	discovery := &Discovery{
		RootTaskfileName: "Taskfile.yaml",
	}

	// Create visible and hidden directories:
	//   tempDir/
	//   ├── visible/
	//   │   └── taskfile.yaml (should be discovered)
	//   └── .hidden/
	//       └── taskfile.yaml (should be skipped)
	visibleDir := filepath.Join(suite.tempDir, "visible")
	hiddenDir := filepath.Join(suite.tempDir, ".hidden")

	suite.Require().NoError(os.MkdirAll(visibleDir, os.ModePerm))
	suite.Require().NoError(os.MkdirAll(hiddenDir, os.ModePerm))

	// Each taskfile.yaml contains minimal structure:
	// tasks: {}
	suite.Require().NoError(os.WriteFile(filepath.Join(visibleDir, "taskfile.yaml"), []byte("tasks: {}"), 0o644))
	suite.Require().NoError(os.WriteFile(filepath.Join(hiddenDir, "taskfile.yaml"), []byte("tasks: {}"), 0o644))

	includes, err := discovery.findComponents(suite.tempDir, 5)

	suite.Require().NoError(err)
	suite.Len(includes, 1)
	suite.Equal("visible", includes[0].Name)
}
