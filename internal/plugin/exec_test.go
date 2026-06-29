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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/stretchr/testify/assert"
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

	exitCode := ExecutePlugin(context.Background(), PluginManifest{}, path, []string{})
	s.Equal(0, exitCode)
}

func (s *ExecTestSuite) TestExecutePluginExitCodeOne() {
	path := s.createScript("fail-one", 1)

	exitCode := ExecutePlugin(context.Background(), PluginManifest{}, path, []string{})
	s.Equal(1, exitCode)
}

func (s *ExecTestSuite) TestExecutePluginExitCodeFortyTwo() {
	path := s.createScript("fail-42", 42)

	exitCode := ExecutePlugin(context.Background(), PluginManifest{}, path, []string{})
	s.Equal(42, exitCode)
}

func (s *ExecTestSuite) TestExecutePluginCommandNotFound() {
	exitCode := ExecutePlugin(context.Background(), PluginManifest{}, filepath.Join(s.tempDir, "nonexistent"), []string{})
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

	exitCode := ExecutePlugin(context.Background(), PluginManifest{}, path, []string{"expected", "args"})
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

	exitCode := ExecutePlugin(context.Background(), PluginManifest{}, path, []string{"wrong", "arguments"})
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

			result := ExecutePlugin(context.Background(), PluginManifest{}, path, []string{})
			require.Equal(t, tt.expectedCode, result)
		})
	}
}

// TestExecutePluginContextCancellation verifies that cancelling the context sends
// SIGTERM to the plugin subprocess (not SIGKILL), allowing graceful shutdown.
func TestExecutePluginContextCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGTERM handling is Unix-specific")
	}

	// Create a script that traps SIGTERM, writes a marker, and exits cleanly
	markerFile := filepath.Join(t.TempDir(), "sigterm-received")
	script := fmt.Sprintf(`#!/bin/sh
trap 'touch %s; exit 55' TERM
while true; do sleep 0.1; done
`, markerFile)

	scriptPath := filepath.Join(t.TempDir(), "trap-term.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan int, 1)

	go func() {
		done <- ExecutePlugin(ctx, PluginManifest{}, scriptPath, []string{})
	}()

	// Give the subprocess time to start and install its signal handler
	time.Sleep(500 * time.Millisecond)

	cancel()

	select {
	case exitCode := <-done:
		// The script should have caught SIGTERM and exited with code 55
		assert.Equal(t, 55, exitCode, "plugin should exit with code 55 after catching SIGTERM")

		_, err := os.Stat(markerFile)
		require.NoError(t, err, "SIGTERM handler marker file should exist")
	case <-time.After(10 * time.Second):
		t.Fatal("plugin did not exit within 10 seconds of context cancellation")
	}
}

// TestExecutePluginContextCancellationKillsUnresponsive verifies that if a
// plugin ignores SIGTERM, cmd.WaitDelay causes SIGKILL after the grace period.
func TestExecutePluginContextCancellationKillsUnresponsive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGTERM handling is Unix-specific")
	}

	// Create a script that ignores SIGTERM entirely
	script := `#!/bin/sh
trap '' TERM
while true; do sleep 0.1; done
`

	scriptPath := filepath.Join(t.TempDir(), "ignore-term.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan int, 1)

	go func() {
		done <- ExecutePlugin(ctx, PluginManifest{}, scriptPath, []string{})
	}()

	// Give the subprocess time to start
	time.Sleep(500 * time.Millisecond)

	cancel()

	select {
	case exitCode := <-done:
		// After WaitDelay, the process is SIGKILLed (exit code -1 on some systems, or signal-based)
		assert.NotEqual(t, 0, exitCode, "unresponsive plugin should be force-killed")
	case <-time.After(15 * time.Second):
		t.Fatal("unresponsive plugin was not killed within 15 seconds")
	}
}

// TestExecutePluginCustomUserAgent verifies that plugins use custom User-Agent during authentication
func TestExecutePluginCustomUserAgent(t *testing.T) {
	var capturedUserAgent string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUserAgent = r.Header.Get("User-Agent")

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	viperx.Reset()
	viperx.Set(config.DataRobotURL, server.URL)
	viperx.Set(config.DataRobotAPIKey, "test-token")

	os.Unsetenv("DATAROBOT_ENDPOINT")
	os.Unsetenv("DATAROBOT_API_TOKEN")

	scriptPath := filepath.Join(t.TempDir(), "test.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 0\n"), 0o755))

	manifest := PluginManifest{
		BasicPluginManifest: BasicPluginManifest{Name: "test-plugin", Version: "1.2.3", Authentication: true},
	}

	ExecutePlugin(context.Background(), manifest, scriptPath, []string{})

	assert.Equal(t, "DataRobot CLI plugin: test-plugin (version 1.2.3)", capturedUserAgent)
}

func TestConfirmPluginDepsInstall_YFlag(t *testing.T) {
	result := confirmPluginDepsInstall([]string{"-y"})

	assert.True(t, result)
}

func TestConfirmPluginDepsInstall_YesFlag(t *testing.T) {
	result := confirmPluginDepsInstall([]string{"--yes"})

	assert.True(t, result)
}

func TestConfirmPluginDepsInstall_NonInteractiveEnv(t *testing.T) {
	t.Setenv("DATAROBOT_CLI_NON_INTERACTIVE", "1")

	result := confirmPluginDepsInstall([]string{})

	assert.True(t, result)
}

func TestCheckAndInstallPluginPrereps_SkipsWhenNoVersionsYaml(t *testing.T) {
	manifest := PluginManifest{
		BasicPluginManifest: BasicPluginManifest{Name: "nonexistent-test-dr-cli-plugin-xyz"},
	}

	result := checkAndInstallPluginDeps(manifest, []string{})

	assert.True(t, result)
}

func TestCheckAndInstallPluginPrereps_TrueWhenAllDepsSatisfied(t *testing.T) {
	const versionsYAML = `echo-tool:
  name: Echo tool
  minimum-version: "1.0.0"
  command: "echo 1.0.0"
  url: https://example.com
  install:
    macos: "echo install"
    linux: "echo install"
`

	managedDir, err := ManagedPluginsDir()
	require.NoError(t, err)

	pluginName := "test-dr-cli-prereq-plugin-xyz"

	pluginDir := filepath.Join(managedDir, pluginName)

	require.NoError(t, os.MkdirAll(pluginDir, 0o755))

	t.Cleanup(func() { _ = os.RemoveAll(pluginDir) })

	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "versions.yaml"), []byte(versionsYAML), 0o644))

	manifest := PluginManifest{
		BasicPluginManifest: BasicPluginManifest{Name: pluginName},
	}

	result := checkAndInstallPluginDeps(manifest, []string{})

	assert.True(t, result)
}
