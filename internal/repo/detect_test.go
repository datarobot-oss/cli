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

package repo_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/datarobot/cli/internal/repo"
	"github.com/stretchr/testify/suite"
)

type DetectTestSuite struct {
	suite.Suite
	tempDir    string
	originalWd string
}

func TestDetectTestSuite(t *testing.T) {
	suite.Run(t, new(DetectTestSuite))
}

func (suite *DetectTestSuite) SetupTest() {
	var err error

	suite.tempDir, err = os.MkdirTemp("", "repo-detect-test")
	if err != nil {
		suite.T().Fatalf("Failed to create temp directory: %v", err)
	}

	suite.originalWd, err = os.Getwd()
	if err != nil {
		suite.T().Fatalf("Failed to get current working directory: %v", err)
	}
}

func (suite *DetectTestSuite) TearDownTest() {
	if suite.originalWd != "" {
		_ = os.Chdir(suite.originalWd)
	}

	if suite.tempDir != "" {
		_ = os.RemoveAll(suite.tempDir)
	}
}

func (suite *DetectTestSuite) createAnswersDir() {
	// Create .datarobot/answers directory
	err := os.MkdirAll(filepath.Join(suite.tempDir, ".datarobot", "answers"), 0o755)
	suite.Require().NoError(err)
}

func (suite *DetectTestSuite) createCliDir() {
	// Create .datarobot/cli directory
	err := os.MkdirAll(filepath.Join(suite.tempDir, ".datarobot", "cli"), 0o755)
	suite.Require().NoError(err)
}

func (suite *DetectTestSuite) createCliVersionsYaml() {
	suite.createCliDir()
	// Create .datarobot/cli/versions.yaml file
	versionsFile := filepath.Join(suite.tempDir, ".datarobot", "cli", "versions.yaml")
	file, err := os.OpenFile(versionsFile, os.O_RDONLY|os.O_CREATE, 0o644)
	suite.Require().NoError(err)
	err = file.Close()
	suite.Require().NoError(err)
}

func (suite *DetectTestSuite) createCliStateYaml() {
	suite.createCliDir()
	// Create .datarobot/cli/state.yaml file
	stateFile := filepath.Join(suite.tempDir, ".datarobot", "cli", "state.yaml")
	file, err := os.OpenFile(stateFile, os.O_RDONLY|os.O_CREATE, 0o644)
	suite.Require().NoError(err)
	err = file.Close()
	suite.Require().NoError(err)
}

func (suite *DetectTestSuite) TestFindRepoRootFindsDataRobotCLI() {
	suite.createAnswersDir()

	// Change to temp directory
	err := os.Chdir(suite.tempDir)
	suite.Require().NoError(err)

	// Should find the repo root
	repoRoot, err := repo.FindRepoRoot()
	suite.Require().NoError(err)

	// Use EvalSymlinks to resolve any symlinks (e.g., /var -> /private/var on macOS)
	expectedPath, err := filepath.EvalSymlinks(suite.tempDir)
	suite.Require().NoError(err)

	actualPath, err := filepath.EvalSymlinks(repoRoot)
	suite.Require().NoError(err)

	suite.Equal(expectedPath, actualPath)
}

func (suite *DetectTestSuite) TestFindRepoRootFromNestedDirectory() {
	suite.createAnswersDir()

	// Create nested directory structure
	nestedPath := filepath.Join(suite.tempDir, "src", "components", "deep")
	err := os.MkdirAll(nestedPath, 0o755)
	suite.Require().NoError(err)

	// Change to nested directory
	err = os.Chdir(nestedPath)
	suite.Require().NoError(err)

	// Should find the repo root by walking up
	repoRoot, err := repo.FindRepoRoot()
	suite.Require().NoError(err)

	// Use EvalSymlinks to resolve any symlinks (e.g., /var -> /private/var on macOS)
	expectedPath, err := filepath.EvalSymlinks(suite.tempDir)
	suite.Require().NoError(err)

	actualPath, err := filepath.EvalSymlinks(repoRoot)
	suite.Require().NoError(err)

	suite.Equal(expectedPath, actualPath)
}

func (suite *DetectTestSuite) TestFindRepoRootNotInRepo() {
	// Don't create .datarobot/answers directory
	err := os.Chdir(suite.tempDir)
	suite.Require().NoError(err)

	// Should not find a repo root
	repoRoot, err := repo.FindRepoRoot()
	suite.Require().Error(err)
	suite.Empty(repoRoot)
}

func (suite *DetectTestSuite) TestIsInRepoReturnsTrueWhenInRepoAnswers() {
	suite.createAnswersDir()

	// Change to temp directory
	err := os.Chdir(suite.tempDir)
	suite.Require().NoError(err)

	// Should return true
	suite.True(repo.IsInRepo())
}

func (suite *DetectTestSuite) TestIsInRepoReturnsTrueWhenInRepoVersions() {
	suite.createCliVersionsYaml()

	// Change to temp directory
	err := os.Chdir(suite.tempDir)
	suite.Require().NoError(err)

	// Should return true
	suite.True(repo.IsInRepo())
}

func (suite *DetectTestSuite) TestIsInRepoReturnsTrueWhenInRepoAll() {
	suite.createAnswersDir()
	suite.createCliVersionsYaml()
	suite.createCliStateYaml()

	// Change to temp directory
	err := os.Chdir(suite.tempDir)
	suite.Require().NoError(err)

	// Should return true
	suite.True(repo.IsInRepo())
}

func (suite *DetectTestSuite) TestIsInRepoReturnsFalseWhenNotInRepo() {
	// Don't create .datarobot/answers directory
	err := os.Chdir(suite.tempDir)
	suite.Require().NoError(err)

	// Should return false
	suite.False(repo.IsInRepo())
}

func (suite *DetectTestSuite) TestIsInRepoReturnsFalseWhenOnlyStateYaml() {
	suite.createCliStateYaml()

	// Change to temp directory
	err := os.Chdir(suite.tempDir)
	suite.Require().NoError(err)

	// Should return false
	suite.False(repo.IsInRepo())
}
