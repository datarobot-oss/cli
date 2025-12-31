// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package clone

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
)

func TestModelTestSuite(t *testing.T) {
	suite.Run(t, new(ModelTestSuite))
}

type ModelTestSuite struct {
	suite.Suite
	tempDir    string
	currentDir string
}

func (suite *ModelTestSuite) SetupTest() {
	dir, _ := os.MkdirTemp("", "datarobot-config-test")
	suite.tempDir = dir
	suite.currentDir, _ = os.Getwd()
	suite.T().Setenv("HOME", suite.tempDir)
	suite.T().Setenv("PARAKEET", suite.tempDir)
}

func (suite *ModelTestSuite) TestLeaveSingleDirNameUnmodified() {
	testFileName := "squak"
	resultDir := cleanDirPath(testFileName)
	expectedDir := filepath.Join(suite.currentDir, testFileName)

	suite.Equal(expectedDir, resultDir, "Expected directory status message to match")

	testFileName = "squak/squak"
	resultDir = cleanDirPath(testFileName)
	expectedDir = filepath.Join(suite.currentDir, testFileName)

	suite.Equal(expectedDir, resultDir, "Expected directory status message to match")
}

func (suite *ModelTestSuite) TestCreateRelativeFilepathExistingFile() {
	testFileName := "squak/squak"
	resultDir := cleanDirPath(testFileName)
	expectedDir := filepath.Join(suite.currentDir, testFileName)

	suite.Equal(expectedDir, resultDir, "Expected directory status message to match")
}

func (suite *ModelTestSuite) TestCreateAbsoluteFilepathNonExistingFile() {
	testFileName := filepath.Join(suite.tempDir, "squak/squak")
	resultDir := cleanDirPath(testFileName)

	suite.Equal(testFileName, resultDir, "Expected directory status message to match")
}

func (suite *ModelTestSuite) TestCreateAbsoluteFilepathHomeShortcutExistingFile() {
	// In this case, ~ is the shortcut for the actual home directory of the user running the test
	testFileName := "~/squak/squak"
	resultDir := cleanDirPath(testFileName)
	expectedDir := filepath.Join(suite.tempDir, "squak/squak")

	suite.Equal(expectedDir, resultDir, "Expected directory status message to match")
}

func (suite *ModelTestSuite) TestCreateAbsoluteFilepathEnvVarExistingFile() {
	// In this case, $HOME and $PARAKEET has been set to suite.tempDir in SetupTest
	testFileName := "$HOME/squak/squak"
	resultDir := cleanDirPath(testFileName)
	expectedDir := filepath.Join(suite.tempDir, "squak/squak")

	suite.Equal(expectedDir, resultDir, "Expected directory status message to match")

	testFileName = "$PARAKEET/squak/squak"
	resultDir = cleanDirPath(testFileName)
	expectedDir = filepath.Join(suite.tempDir, "squak/squak")

	suite.Equal(expectedDir, resultDir, "Expected directory status message to match")
}
