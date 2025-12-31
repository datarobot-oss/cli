// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package fsutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
)

func TestModelTestSuite(t *testing.T) {
	suite.Run(t, new(testSuite))
}

type testSuite struct {
	suite.Suite
	tempDir    string
	currentDir string
}

func (s *testSuite) SetupTest() {
	dir, _ := os.MkdirTemp("", "datarobot-fsutil-test")
	s.tempDir = dir
	s.currentDir, _ = os.Getwd()
	s.T().Setenv("HOME", s.tempDir)
	s.T().Setenv("PARAKEET", s.tempDir)
}

func (s *testSuite) TestLeaveSingleDirNameUnmodified() {
	testFileName := "squak"
	resultDir := AbsolutePath(testFileName)
	expectedDir := filepath.Join(s.currentDir, testFileName)

	s.Equal(expectedDir, resultDir, "Expected path to match")

	testFileName = "squak/squak"
	resultDir = AbsolutePath(testFileName)
	expectedDir = filepath.Join(s.currentDir, testFileName)

	s.Equal(expectedDir, resultDir, "Expected path to match")
}

func (s *testSuite) TestCreateRelativeFilepathExistingFile() {
	testFileName := "squak/squak"
	resultDir := AbsolutePath(testFileName)
	expectedDir := filepath.Join(s.currentDir, testFileName)

	s.Equal(expectedDir, resultDir, "Expected path to match")
}

func (s *testSuite) TestCreateAbsoluteFilepathNonExistingFile() {
	testFileName := filepath.Join(s.tempDir, "squak/squak")
	resultDir := AbsolutePath(testFileName)

	s.Equal(testFileName, resultDir, "Expected path to match")
}

func (s *testSuite) TestCreateAbsoluteFilepathHomeShortcutExistingFile() {
	// In this case, ~ is the shortcut for the actual home directory of the user running the test
	testFileName := "~/squak/squak"
	resultDir := AbsolutePath(testFileName)
	expectedDir := filepath.Join(s.tempDir, "squak/squak")

	s.Equal(expectedDir, resultDir, "Expected path to match")
}

func (s *testSuite) TestCreateAbsoluteFilepathEnvVarExistingFile() {
	// In this case, $HOME and $PARAKEET has been set to s.tempDir in SetupTest
	testFileName := "$HOME/squak/squak"
	resultDir := AbsolutePath(testFileName)
	expectedDir := filepath.Join(s.tempDir, "squak/squak")

	s.Equal(expectedDir, resultDir, "Expected path to match")

	testFileName = "$PARAKEET/squak/squak"
	resultDir = AbsolutePath(testFileName)
	expectedDir = filepath.Join(s.tempDir, "squak/squak")

	s.Equal(expectedDir, resultDir, "Expected path to match")
}
