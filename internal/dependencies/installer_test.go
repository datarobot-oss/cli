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

package dependencies

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errWriter is an io.Writer that always returns an error.
// It triggers the non-ExitError path in ExecuteShLine by failing the
// stdout/stderr copy goroutine that exec.Cmd runs internally.
type errWriter struct{}

func (errWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestExecuteShLine_Success(t *testing.T) {
	var buf bytes.Buffer

	code, err := ExecuteShLine("echo hello", &buf)

	require.NoError(t, err)
	assert.Equal(t, 0, code)
	assert.Contains(t, buf.String(), "hello")
}

func TestExecuteShLine_NonZeroExitCode(t *testing.T) {
	var buf bytes.Buffer

	code, err := ExecuteShLine("exit 42", &buf)

	require.NoError(t, err)
	assert.Equal(t, 42, code)
}

func TestExecuteShLine_MultiLineOutput(t *testing.T) {
	var buf bytes.Buffer

	code, err := ExecuteShLine("echo line1; echo line2; echo line3", &buf)

	require.NoError(t, err)
	assert.Equal(t, 0, code)

	output := buf.String()

	assert.Contains(t, output, "line1")
	assert.Contains(t, output, "line2")
	assert.Contains(t, output, "line3")
	assert.Equal(t, 3, strings.Count(output, "\n"))
}

func TestExecuteShLine_PipeCommand(t *testing.T) {
	var buf bytes.Buffer

	code, err := ExecuteShLine("echo piped_value | cat", &buf)

	require.NoError(t, err)
	assert.Equal(t, 0, code)
	assert.Contains(t, buf.String(), "piped_value")
}

func TestExecuteShLine_StderrCaptured(t *testing.T) {
	var buf bytes.Buffer

	code, err := ExecuteShLine("echo error_output >&2", &buf)

	require.NoError(t, err)
	assert.Equal(t, 0, code)
	assert.Contains(t, buf.String(), "error_output")
}

func TestExecuteShLine_CommandNotFound(t *testing.T) {
	var buf bytes.Buffer

	code, err := ExecuteShLine("nonexistent_command_dr_cli_test_xyz", &buf)

	require.NoError(t, err)
	assert.NotEqual(t, 0, code)
}

func TestExecuteShLine_OutputBeforeFailure(t *testing.T) {
	var buf bytes.Buffer

	// Verifies output is captured even when the command exits non-zero.
	code, err := ExecuteShLine("echo before_fail; exit 2", &buf)

	require.NoError(t, err)
	assert.Equal(t, 2, code)
	assert.Contains(t, buf.String(), "before_fail")
}

func TestExecuteShLine_WriterError(t *testing.T) {
	// errWriter fails on the first Write call, which causes the exec copy
	// goroutine to error. cmd.Wait() surfaces that as a non-ExitError, so
	// ExecuteShLine returns (1, err) with a non-nil error.
	code, err := ExecuteShLine("echo hello", errWriter{})

	assert.Equal(t, 1, code)
	assert.Error(t, err)
}

func TestExecuteShLine_WriterErrorPreservesMessage(t *testing.T) {
	code, err := ExecuteShLine("echo hello", errWriter{})

	assert.Equal(t, 1, code)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write failed")
}

// --- InstallPrerequisites tests ---

// setupFakeRepo creates a temp dir with the .datarobot/cli structure required
// by repo.FindRepoRoot(), changes the working directory into it, and restores
// it via t.Cleanup.
func setupFakeRepo(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()

	resolved, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	tmpDir = resolved

	localStateDir := filepath.Join(tmpDir, ".datarobot", "cli")

	err = os.MkdirAll(localStateDir, 0o755)
	require.NoError(t, err)

	versionsFile := filepath.Join(localStateDir, "versions.yaml")

	err = os.WriteFile(versionsFile, []byte("tools: []\n"), 0o644)
	require.NoError(t, err)

	originalWd, err := os.Getwd()
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, os.Chdir(originalWd))
	})

	require.NoError(t, os.Chdir(tmpDir))

	return tmpDir
}

// prereq builds a Prerequisite with the same install command on all platforms.
func prereq(name, installCmd string) tools.Prerequisite {
	return tools.Prerequisite{
		Name: name,
		Install: tools.InstallCommands{
			MacOS:   installCmd,
			Linux:   installCmd,
			Windows: installCmd,
		},
	}
}

func TestInstallPrerequisites_Empty(t *testing.T) {
	setupFakeRepo(t)

	var out bytes.Buffer

	installed, err := InstallPrerequisites(&out, nil)

	require.NoError(t, err)
	assert.Empty(t, installed)
	assert.Contains(t, out.String(), "successfully")
}

func TestInstallPrerequisites_Success(t *testing.T) {
	setupFakeRepo(t)

	var out bytes.Buffer

	installed, err := InstallPrerequisites(&out, []tools.Prerequisite{prereq("uv", "echo install-uv")})

	require.NoError(t, err)
	assert.Equal(t, []string{"uv"}, installed)
	assert.Contains(t, out.String(), "install-uv")
	assert.Contains(t, out.String(), "successfully")
}

func TestInstallPrerequisites_ReturnsAllInstalledNames(t *testing.T) {
	setupFakeRepo(t)

	var out bytes.Buffer

	deps := []tools.Prerequisite{
		prereq("uv", "echo install-uv"),
		prereq("task", "echo install-task"),
	}

	installed, err := InstallPrerequisites(&out, deps)

	require.NoError(t, err)
	assert.Equal(t, []string{"uv", "task"}, installed)
}

func TestInstallPrerequisites_ReturnsPartialInstalledOnFailure(t *testing.T) {
	var out bytes.Buffer

	deps := []tools.Prerequisite{
		prereq("uv", "echo install-uv"),
		prereq("task", "exit 1"),
	}

	installed, err := InstallPrerequisites(&out, deps)

	require.Error(t, err)
	assert.Equal(t, []string{"uv"}, installed, "should return names installed before the failure")
	assert.Contains(t, err.Error(), "task")
}

func TestInstallPrerequisites_ReturnsEmptyWhenNoPlatformCommand(t *testing.T) {
	var out bytes.Buffer

	dep := tools.Prerequisite{Name: "uv"} // no install commands set

	installed, err := InstallPrerequisites(&out, []tools.Prerequisite{dep})

	require.Error(t, err)
	assert.Empty(t, installed)
}

func TestInstallPrerequisites_FailureShowsRawCommand(t *testing.T) {
	var out bytes.Buffer

	installed, err := InstallPrerequisites(&out, []tools.Prerequisite{prereq("uv", "exit 1")})
	assert.Empty(t, installed)

	require.Error(t, err)
	assert.Contains(t, out.String(), "exit 1")
}

func TestInstallPrerequisites_NoPlatformCommand(t *testing.T) {
	var out bytes.Buffer

	dep := tools.Prerequisite{Name: "uv"}

	_, err := InstallPrerequisites(&out, []tools.Prerequisite{dep})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "uv")
}

func TestInstallPrerequisites_AbortsOnFirstFailure(t *testing.T) {
	var out bytes.Buffer

	deps := []tools.Prerequisite{
		{
			Name: "uv",
			Install: tools.InstallCommands{
				MacOS:   "exit 1",
				Linux:   "exit 1",
				Windows: "exit 1",
			},
		},
		{
			Name: "task",
			Install: tools.InstallCommands{
				MacOS:   "echo install-task",
				Linux:   "echo install-task",
				Windows: "echo install-task",
			},
		},
	}

	_, err := InstallPrerequisites(&out, deps)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "uv")
	assert.NotContains(t, out.String(), "install-task")
}

func TestInstallPrerequisites_WritesStateOnSuccess(t *testing.T) {
	repoRoot := setupFakeRepo(t)

	var out bytes.Buffer

	_, err := InstallPrerequisites(&out, []tools.Prerequisite{prereq("uv", "echo install-uv")})
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(repoRoot, ".datarobot", "cli", "state.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "last_success_deps_check")
}

func TestInstallPrerequisites_PartialFailureDoesNotWriteState(t *testing.T) {
	repoRoot := setupFakeRepo(t)

	var out bytes.Buffer

	deps := []tools.Prerequisite{
		prereq("failing", "exit 1"),
		prereq("ok", "echo ok"),
	}

	_, err := InstallPrerequisites(&out, deps)
	require.Error(t, err)

	data, _ := os.ReadFile(filepath.Join(repoRoot, ".datarobot", "cli", "state.yaml"))
	assert.NotContains(t, string(data), "last_success_deps_check")
}

func TestLastSuccessDepsCheck_UpdatedByInstallPrerequisites(t *testing.T) {
	repoRoot := setupFakeRepo(t)

	var out bytes.Buffer

	_, err := InstallPrerequisites(&out, []tools.Prerequisite{prereq("uv", "echo install-uv")})
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(repoRoot, ".datarobot", "cli", "state.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "last_success_deps_check")
}
