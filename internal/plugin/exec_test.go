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
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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

	exitCode := ExecutePlugin(context.Background(), PluginManifest{}, path, []string{}, nil)
	s.Equal(0, exitCode)
}

func (s *ExecTestSuite) TestExecutePluginExitCodeOne() {
	path := s.createScript("fail-one", 1)

	exitCode := ExecutePlugin(context.Background(), PluginManifest{}, path, []string{}, nil)
	s.Equal(1, exitCode)
}

func (s *ExecTestSuite) TestExecutePluginExitCodeFortyTwo() {
	path := s.createScript("fail-42", 42)

	exitCode := ExecutePlugin(context.Background(), PluginManifest{}, path, []string{}, nil)
	s.Equal(42, exitCode)
}

func (s *ExecTestSuite) TestExecutePluginCommandNotFound() {
	exitCode := ExecutePlugin(context.Background(), PluginManifest{}, filepath.Join(s.tempDir, "nonexistent"), []string{}, nil)
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

	exitCode := ExecutePlugin(context.Background(), PluginManifest{}, path, []string{"expected", "args"}, nil)
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

	exitCode := ExecutePlugin(context.Background(), PluginManifest{}, path, []string{"wrong", "arguments"}, nil)
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

			result := ExecutePlugin(context.Background(), PluginManifest{}, path, []string{}, nil)
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
		done <- ExecutePlugin(ctx, PluginManifest{}, scriptPath, []string{}, nil)
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
		done <- ExecutePlugin(ctx, PluginManifest{}, scriptPath, []string{}, nil)
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

	ExecutePlugin(context.Background(), manifest, scriptPath, []string{}, nil)

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

func TestCheckAndInstallPluginPrereqs_SkipsWhenNoVersionsYaml(t *testing.T) {
	manifest := PluginManifest{
		BasicPluginManifest: BasicPluginManifest{Name: "nonexistent-test-dr-cli-plugin-xyz"},
	}

	result := checkAndInstallPluginDeps(manifest, []string{})

	assert.True(t, result)
}

func TestCheckAndInstallPluginPrereqs_TrueWhenAllDepsSatisfied(t *testing.T) {
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

// --- universalFlagEnv unit tests ---

// setupUniversalTestFlags creates an isolated flagset with the standard
// universal flags annotated, for passing directly to universalFlagEnv.
func setupUniversalTestFlags(t *testing.T) *pflag.FlagSet {
	t.Helper()

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	fs.Bool("debug", false, "")
	fs.Bool("disable-telemetry", false, "")
	fs.Lookup("debug").Annotations = map[string][]string{config.UniversalAnnotationKey: {"DEBUG"}}
	fs.Lookup("disable-telemetry").Annotations = map[string][]string{config.UniversalAnnotationKey: {"DISABLE_TELEMETRY"}}

	return fs
}

func TestUniversalFlagEnv_AllUnset(t *testing.T) {
	viperx.Reset()

	fs := setupUniversalTestFlags(t)

	result := universalFlagEnv(fs)

	assert.Empty(t, result, "no env vars should be emitted when no universal flags are set")
}

func TestUniversalFlagEnv_DebugSet(t *testing.T) {
	viperx.Reset()

	fs := setupUniversalTestFlags(t)

	viperx.Set("debug", true)

	result := universalFlagEnv(fs)

	assert.Contains(t, result, config.EnvPrefix+"DEBUG=1")
	assert.NotContains(t, result, config.EnvPrefix+"DISABLE_TELEMETRY=1")
}

func TestUniversalFlagEnv_DisableTelemetrySet(t *testing.T) {
	viperx.Reset()

	fs := setupUniversalTestFlags(t)

	viperx.Set("disable-telemetry", true)

	result := universalFlagEnv(fs)

	assert.Contains(t, result, config.EnvPrefix+"DISABLE_TELEMETRY=1")
	assert.NotContains(t, result, config.EnvPrefix+"DEBUG=1")
}

func TestUniversalFlagEnv_BothSet(t *testing.T) {
	viperx.Reset()

	fs := setupUniversalTestFlags(t)

	viperx.Set("debug", true)
	viperx.Set("disable-telemetry", true)

	result := universalFlagEnv(fs)

	assert.Contains(t, result, config.EnvPrefix+"DEBUG=1")
	assert.Contains(t, result, config.EnvPrefix+"DISABLE_TELEMETRY=1")
}

func TestUniversalFlagEnv_BoolFalseOmitted(t *testing.T) {
	viperx.Reset()

	fs := setupUniversalTestFlags(t)

	viperx.Set("debug", false)
	viperx.Set("disable-telemetry", false)

	result := universalFlagEnv(fs)

	assert.Empty(t, result, "false bool flags must not be emitted")
}

// --- TraverseChildren / core-blind invariant tests ---

// buildTestTree returns an isolated cobra command tree that mirrors the real CLI
// wiring: root with TraverseChildren + two persistent flags, and a plugin-style
// child with DisableFlagParsing:true that records the args it receives.
func buildTestTree(t *testing.T) (root *cobra.Command, receivedArgs *[]string, debugSet *bool, skipUpdateSet *bool) {
	t.Helper()

	var got []string

	var dbg bool

	var skip bool

	child := &cobra.Command{
		Use:                "plug",
		DisableFlagParsing: true,
		DisableSuggestions: true,
		Run: func(_ *cobra.Command, args []string) {
			got = args
		},
	}

	root = &cobra.Command{
		Use:              "dr",
		TraverseChildren: true,
		SilenceErrors:    true,
		SilenceUsage:     true,
	}
	root.PersistentFlags().Bool("debug", false, "debug output")
	root.PersistentFlags().Bool("skip-plugin-update-check", false, "skip plugin update checks")

	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		dbg, _ = root.PersistentFlags().GetBool("debug")
		skip, _ = root.PersistentFlags().GetBool("skip-plugin-update-check")

		return nil
	}

	// Mirror setUnknownArgGuards: make root runnable so unknown positional args
	// produce a clear error rather than silently showing help.
	root.Args = func(_ *cobra.Command, args []string) error {
		if len(args) == 0 {
			return nil
		}

		return fmt.Errorf("unknown command: %s", args[0])
	}

	root.RunE = func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	}

	root.AddCommand(child)

	return root, &got, &dbg, &skip
}

// TestTraverseChildren_PrePluginFlagConsumedByCore verifies that --debug placed
// BEFORE the plugin name is parsed by core (debug set) and NOT forwarded to the
// plugin as a literal arg.
func TestTraverseChildren_PrePluginFlagConsumedByCore(t *testing.T) {
	root, receivedArgs, debugSet, _ := buildTestTree(t)
	root.SetArgs([]string{"--debug", "plug", "foo", "bar"})

	err := root.Execute()
	require.NoError(t, err)

	assert.True(t, *debugSet, "core must see --debug when it appears before plugin name")
	assert.Equal(t, []string{"foo", "bar"}, *receivedArgs,
		"plugin must receive only its own args, not the consumed --debug flag")
}

// TestTraverseChildren_MultipleFlagsConsumedByCore verifies that multiple
// persistent flags placed BEFORE the plugin name are all consumed by core
// and none leak into the plugin's raw args.
func TestTraverseChildren_MultipleFlagsConsumedByCore(t *testing.T) {
	root, receivedArgs, debugSet, skipUpdateSet := buildTestTree(t)
	root.SetArgs([]string{"--skip-plugin-update-check", "--debug", "plug", "foo", "bar"})

	err := root.Execute()
	require.NoError(t, err)

	assert.True(t, *debugSet, "core must see --debug when it appears before plugin name")
	assert.True(t, *skipUpdateSet, "core must see --skip-plugin-update-check when it appears before plugin name")
	assert.Equal(t, []string{"foo", "bar"}, *receivedArgs,
		"plugin must receive only its own args, not any consumed root flags")
}

// TestTraverseChildren_PostPluginFlagsInvisibleToCore is the hard invariant:
// core stays BLIND to any args after the plugin name (kubectl/helm model).
// --debug after the plugin name must NOT set core debug, and must pass through
// to the plugin verbatim.
func TestTraverseChildren_PostPluginFlagsInvisibleToCore(t *testing.T) {
	root, receivedArgs, debugSet, _ := buildTestTree(t)
	root.SetArgs([]string{"plug", "--debug", "foo"})

	err := root.Execute()
	require.NoError(t, err)

	assert.False(t, *debugSet, "core must NOT see --debug when it appears after plugin name")
	assert.Equal(t, []string{"--debug", "foo"}, *receivedArgs,
		"plugin must receive --debug verbatim when it appears after the plugin name")
}
