// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

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

func (suite *DetectTestSuite) TestFindRepoRootFindsDataRobotCLI() {
	// Create .datarobot/cli directory
	datarobotCLIPath := filepath.Join(suite.tempDir, ".datarobot", "cli")
	err := os.MkdirAll(datarobotCLIPath, 0o755)
	suite.Require().NoError(err)

	// Change to temp directory
	err = os.Chdir(suite.tempDir)
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
	// Create .datarobot/cli directory
	datarobotCLIPath := filepath.Join(suite.tempDir, ".datarobot", "cli")
	err := os.MkdirAll(datarobotCLIPath, 0o755)
	suite.Require().NoError(err)

	// Create nested directory structure
	nestedPath := filepath.Join(suite.tempDir, "src", "components", "deep")
	err = os.MkdirAll(nestedPath, 0o755)
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

func (suite *DetectTestSuite) TestFindRepoRootStopsAtGitFolder() {
	// Create a .git directory (simulating a git repo boundary)
	gitPath := filepath.Join(suite.tempDir, ".git")
	err := os.MkdirAll(gitPath, 0o755)
	suite.Require().NoError(err)

	// Don't create .datarobot/cli, so it's a git repo but not a DataRobot repo
	err = os.Chdir(suite.tempDir)
	suite.Require().NoError(err)

	// Should not find a repo root
	repoRoot, err := repo.FindRepoRoot()
	suite.Require().NoError(err)
	suite.Empty(repoRoot)
}

func (suite *DetectTestSuite) TestFindRepoRootNotInRepo() {
	// Don't create .datarobot/cli directory
	err := os.Chdir(suite.tempDir)
	suite.Require().NoError(err)

	// Should not find a repo root
	repoRoot, err := repo.FindRepoRoot()
	suite.Require().NoError(err)
	suite.Empty(repoRoot)
}

func (suite *DetectTestSuite) TestIsInRepoReturnsTrueWhenInRepo() {
	// Create .datarobot/cli directory
	datarobotCLIPath := filepath.Join(suite.tempDir, ".datarobot", "cli")
	err := os.MkdirAll(datarobotCLIPath, 0o755)
	suite.Require().NoError(err)

	// Change to temp directory
	err = os.Chdir(suite.tempDir)
	suite.Require().NoError(err)

	// Should return true
	suite.True(repo.IsInRepo())
}

func (suite *DetectTestSuite) TestIsInRepoReturnsFalseWhenNotInRepo() {
	// Don't create .datarobot/cli directory
	err := os.Chdir(suite.tempDir)
	suite.Require().NoError(err)

	// Should return false
	suite.False(repo.IsInRepo())
}
