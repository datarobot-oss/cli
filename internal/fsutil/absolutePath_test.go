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
