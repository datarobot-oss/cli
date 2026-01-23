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
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// ExecTestSuite tests plugin execution functions
type ExecTestSuite struct {
	suite.Suite
	tempDir string
}

func TestExecTestSuite(t *testing.T) {
	suite.Run(t, new(ExecTestSuite))
}

func (s *ExecTestSuite) SetupTest() {
	var err error

	s.tempDir, err = os.MkdirTemp("", "plugin-exec-test")
	s.Require().NoError(err)
}

func (s *ExecTestSuite) TearDownTest() {
	if s.tempDir != "" {
		_ = os.RemoveAll(s.tempDir)
	}
}

// createScript creates a shell script that exits with the given code
func (s *ExecTestSuite) createScript(name string, exitCode int) string {
	script := fmt.Sprintf(`#!/bin/sh
exit %d
`, exitCode)

	path := filepath.Join(s.tempDir, name)
	err := os.WriteFile(path, []byte(script), 0o755)
	s.Require().NoError(err)

	return path
}

func (s *ExecTestSuite) TestExecutePluginSuccessfulExecution() {
	path := s.createScript("success", 0)

	exitCode := ExecutePlugin(path, []string{})
	s.Equal(0, exitCode)
}

func (s *ExecTestSuite) TestExecutePluginExitCodeOne() {
	path := s.createScript("fail-one", 1)

	exitCode := ExecutePlugin(path, []string{})
	s.Equal(1, exitCode)
}

func (s *ExecTestSuite) TestExecutePluginExitCodeFortyTwo() {
	path := s.createScript("fail-42", 42)

	exitCode := ExecutePlugin(path, []string{})
	s.Equal(42, exitCode)
}

func (s *ExecTestSuite) TestExecutePluginCommandNotFound() {
	exitCode := ExecutePlugin(filepath.Join(s.tempDir, "nonexistent"), []string{})
	s.Equal(1, exitCode)
}

func (s *ExecTestSuite) TestExecutePluginWithArguments() {
	// Create a script that uses arguments
	script := `#!/bin/sh
if [ "$1" = "expected" ] && [ "$2" = "args" ]; then
  exit 0
else
  exit 1
fi
`
	path := filepath.Join(s.tempDir, "with-args")
	s.Require().NoError(os.WriteFile(path, []byte(script), 0o755))

	exitCode := ExecutePlugin(path, []string{"expected", "args"})
	s.Equal(0, exitCode)
}

func (s *ExecTestSuite) TestExecutePluginWithWrongArguments() {
	// Create a script that uses arguments
	script := `#!/bin/sh
if [ "$1" = "expected" ] && [ "$2" = "args" ]; then
  exit 0
else
  exit 1
fi
`
	path := filepath.Join(s.tempDir, "with-args-fail")
	s.Require().NoError(os.WriteFile(path, []byte(script), 0o755))

	exitCode := ExecutePlugin(path, []string{"wrong", "arguments"})
	s.Equal(1, exitCode)
}

// TestExecutePluginExitCodes tests various exit codes are properly propagated
func TestExecutePluginExitCodes(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "plugin-exitcode-test")
	require.NoError(t, err)

	defer os.RemoveAll(tempDir)

	tests := []struct {
		name         string
		exitCode     int
		expectedCode int
	}{
		{"exit 0", 0, 0},
		{"exit 1", 1, 1},
		{"exit 2", 2, 2},
		{"exit 42", 42, 42},
		{"exit 127", 127, 127},
		{"exit 255", 255, 255},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script := fmt.Sprintf(`#!/bin/sh
exit %d
`, tt.exitCode)

			path := filepath.Join(tempDir, fmt.Sprintf("exit-%d", tt.exitCode))
			require.NoError(t, os.WriteFile(path, []byte(script), 0o755))

			result := ExecutePlugin(path, []string{})
			require.Equal(t, tt.expectedCode, result)
		})
	}
}
