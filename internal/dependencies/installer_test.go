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
// It triggers the non-ExitError path in ExecutePlatformCommand by failing the
// stdout/stderr copy goroutine that exec.Cmd runs internally.
type errWriter struct{}

func (errWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestExecutePlatformCommand_Success(t *testing.T) {
	var buf bytes.Buffer

	code, err := ExecutePlatformCommand("echo hello", &buf)

	require.NoError(t, err)
	assert.Equal(t, 0, code)
	assert.Contains(t, buf.String(), "hello")
}

func TestExecutePlatformCommand_NonZeroExitCode(t *testing.T) {
	var buf bytes.Buffer

	code, err := ExecutePlatformCommand("exit 42", &buf)

	require.NoError(t, err)
	assert.Equal(t, 42, code)
}

func TestExecutePlatformCommand_MultiLineOutput(t *testing.T) {
	var buf bytes.Buffer

	code, err := ExecutePlatformCommand("echo line1; echo line2; echo line3", &buf)

	require.NoError(t, err)
	assert.Equal(t, 0, code)

	output := buf.String()

	assert.Contains(t, output, "line1")
	assert.Contains(t, output, "line2")
	assert.Contains(t, output, "line3")
	assert.Equal(t, 3, strings.Count(output, "\n"))
}

func TestExecutePlatformCommand_PipeCommand(t *testing.T) {
	var buf bytes.Buffer

	code, err := ExecutePlatformCommand("echo piped_value | cat", &buf)

	require.NoError(t, err)
	assert.Equal(t, 0, code)
	assert.Contains(t, buf.String(), "piped_value")
}

func TestExecutePlatformCommand_StderrCaptured(t *testing.T) {
	var buf bytes.Buffer

	code, err := ExecutePlatformCommand("echo error_output >&2", &buf)

	require.NoError(t, err)
	assert.Equal(t, 0, code)
	assert.Contains(t, buf.String(), "error_output")
}

func TestExecutePlatformCommand_CommandNotFound(t *testing.T) {
	var buf bytes.Buffer

	code, err := ExecutePlatformCommand("nonexistent_command_dr_cli_test_xyz", &buf)

	require.NoError(t, err)
	assert.NotEqual(t, 0, code)
}

func TestExecutePlatformCommand_OutputBeforeFailure(t *testing.T) {
	var buf bytes.Buffer

	// Verifies output is captured even when the command exits non-zero.
	code, err := ExecutePlatformCommand("echo before_fail; exit 2", &buf)

	require.NoError(t, err)
	assert.Equal(t, 2, code)
	assert.Contains(t, buf.String(), "before_fail")
}

func TestExecutePlatformCommand_WriterError(t *testing.T) {
	// errWriter fails on the first Write call, which causes the exec copy
	// goroutine to error. cmd.Wait() surfaces that as a non-ExitError, so
	// ExecutePlatformCommand returns (1, err) with a non-nil error.
	code, err := ExecutePlatformCommand("echo hello", errWriter{})

	assert.Equal(t, 1, code)
	assert.Error(t, err)
}

func TestExecutePlatformCommand_WriterErrorPreservesMessage(t *testing.T) {
	code, err := ExecutePlatformCommand("echo hello", errWriter{})

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
func prereq(name, installCmd, version string) tools.Prerequisite {
	return tools.Prerequisite{
		Name:           name,
		MinimumVersion: version,
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

	installed, err := InstallPrerequisites(&out, []tools.Prerequisite{prereq("uv", "echo install-uv", "")})

	require.NoError(t, err)
	assert.Equal(t, []string{"uv"}, installed)
	assert.Contains(t, out.String(), "install-uv")
	assert.Contains(t, out.String(), "successfully")
}

func TestInstallPrerequisites_ReturnsAllInstalledNames(t *testing.T) {
	setupFakeRepo(t)

	var out bytes.Buffer

	deps := []tools.Prerequisite{
		prereq("uv", "echo install-uv", ""),
		prereq("task", "echo install-task", ""),
	}

	installed, err := InstallPrerequisites(&out, deps)

	require.NoError(t, err)
	assert.Equal(t, []string{"uv", "task"}, installed)
}

func TestInstallPrerequisites_ReturnsPartialInstalledOnFailure(t *testing.T) {
	var out bytes.Buffer

	deps := []tools.Prerequisite{
		prereq("uv", "echo install-uv", ""),
		prereq("task", "exit 1", ""),
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

	installed, err := InstallPrerequisites(&out, []tools.Prerequisite{prereq("uv", "exit 1", "")})
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

	_, err := InstallPrerequisites(&out, []tools.Prerequisite{prereq("uv", "echo install-uv", "")})
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(repoRoot, ".datarobot", "cli", "state.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "last_success_deps_check")
}

func TestInstallPrerequisites_PartialFailureDoesNotWriteState(t *testing.T) {
	repoRoot := setupFakeRepo(t)

	var out bytes.Buffer

	deps := []tools.Prerequisite{
		prereq("failing", "exit 1", ""),
		prereq("ok", "echo ok", ""),
	}

	_, err := InstallPrerequisites(&out, deps)
	require.Error(t, err)

	data, _ := os.ReadFile(filepath.Join(repoRoot, ".datarobot", "cli", "state.yaml"))
	assert.NotContains(t, string(data), "last_success_deps_check")
}

func TestLastSuccessDepsCheck_UpdatedByInstallPrerequisites(t *testing.T) {
	repoRoot := setupFakeRepo(t)

	var out bytes.Buffer

	_, err := InstallPrerequisites(&out, []tools.Prerequisite{prereq("uv", "echo install-uv", "")})
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(repoRoot, ".datarobot", "cli", "state.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "last_success_deps_check")
}

// --- isPermissionDenied tests ---

func TestIsPermissionDenied_ExitCode126(t *testing.T) {
	assert.True(t, isPermissionDenied(126, ""))
}

func TestIsPermissionDenied_NonSpecialExitCode(t *testing.T) {
	assert.False(t, isPermissionDenied(1, ""))
}

func TestIsPermissionDenied_StderrPermissionDenied(t *testing.T) {
	assert.True(t, isPermissionDenied(1, "sh: permission denied"))
}

func TestIsPermissionDenied_StderrOperationNotPermitted(t *testing.T) {
	assert.True(t, isPermissionDenied(1, "operation not permitted"))
}

func TestIsPermissionDenied_StderrAccessIsDenied(t *testing.T) {
	assert.True(t, isPermissionDenied(1, "Access is denied"))
}

func TestIsPermissionDenied_StderrRequiresRootPrivileges(t *testing.T) {
	assert.True(t, isPermissionDenied(1, "requires root privileges"))
}

func TestIsPermissionDenied_StderrUnauthorizedAccessException(t *testing.T) {
	assert.True(t, isPermissionDenied(1, "UnauthorizedAccessException"))
}

func TestIsPermissionDenied_StderrCaseInsensitive(t *testing.T) {
	assert.True(t, isPermissionDenied(1, "PERMISSION DENIED"))
}

func TestIsPermissionDenied_ExitCodeNonZero(t *testing.T) {
	assert.False(t, isPermissionDenied(1, "Just a normal error"))
}

func TestIsPermissionDenied_ExitCodeZeroNoText(t *testing.T) {
	assert.False(t, isPermissionDenied(0, ""))
}

// --- extractFailedManager tests ---

func TestExtractFailedManager_Brew(t *testing.T) {
	assert.Equal(t, "brew", extractFailedManager("brew install uv"))
}

func TestExtractFailedManager_Pyenv(t *testing.T) {
	assert.Equal(t, "pyenv", extractFailedManager("pyenv install 3.12"))
}

func TestExtractFailedManager_Winget(t *testing.T) {
	assert.Equal(t, "winget", extractFailedManager("winget install OpenJS.NodeJS"))
}

func TestExtractFailedManager_NoMatch(t *testing.T) {
	assert.Empty(t, extractFailedManager("curl -LsSf https://astral.sh/uv/install.sh | sh"))
}

func TestExtractFailedManager_EmptyCmd(t *testing.T) {
	assert.Empty(t, extractFailedManager(""))
}

// --- buildInstallTip tests ---

func TestBuildInstallTip_PermDenied_ContainsSudo(t *testing.T) {
	tip := buildInstallTip(prereq("uv", "brew install uv", ""), true, map[string]bool{}, "linux")

	assert.Contains(t, tip, "sudo")
}

func TestBuildInstallTip_EmptyToolKey(t *testing.T) {
	assert.Empty(t, buildInstallTip(prereq("", "brew install uv", ""), false, map[string]bool{"brew": true}, "linux"))
}

func TestBuildInstallTip_UnknownTool(t *testing.T) {
	p := tools.Prerequisite{Key: "nonexistent-tool", Install: tools.InstallCommands{MacOS: "brew install nonexistent", Linux: "brew install nonexistent", Windows: "brew install nonexistent"}}

	assert.Empty(t, buildInstallTip(p, false, map[string]bool{"brew": true}, "linux"))
}

func TestBuildInstallTip_ManagerStrategy(t *testing.T) {
	tip := buildInstallTip(prereq("uv", "brew install uv", ""), false, map[string]bool{"pyenv": true}, "linux")

	assert.Contains(t, tip, "You have pyenv")
	assert.Contains(t, tip, "pip install uv")
}

func TestBuildInstallTip_ManagerStrategyMultipleCommands(t *testing.T) {
	tip := buildInstallTip(prereq("uv", "brew install uv", ""), false, map[string]bool{"asdf": true}, "linux")

	assert.Contains(t, tip, "You have asdf")
	assert.Contains(t, tip, "asdf install uv latest")
	assert.Contains(t, tip, TAB+TAB)
}

func TestBuildInstallTip_FallbackStrategy(t *testing.T) {
	tip := buildInstallTip(prereq("uv", "brew install uv", ""), false, map[string]bool{}, "linux")

	assert.Contains(t, tip, "curl")
	assert.NotContains(t, tip, "You have")
}

func TestBuildInstallTip_FallbackStrategyWindows(t *testing.T) {
	tip := buildInstallTip(prereq("uv", "winget install uv", ""), false, map[string]bool{}, "windows")

	assert.Contains(t, tip, "iex")
}

func TestBuildInstallTip_SkipsFailedManager(t *testing.T) {
	// brew failed, pyenv is available — should suggest pyenv, not brew
	tip := buildInstallTip(prereq("uv", "brew install uv", ""), false, map[string]bool{"brew": true, "pyenv": true}, "linux")

	assert.Contains(t, tip, "pyenv")
	assert.NotContains(t, tip, "brew")
}

func TestBuildInstallTip_PermDenied_Windows(t *testing.T) {
	tip := buildInstallTip(prereq("uv", "winget install uv", ""), true, map[string]bool{}, "windows")

	assert.Contains(t, tip, "Administrator")
	assert.NotContains(t, tip, "sudo")
}

func TestBuildInstallTip_PermDenied_Darwin(t *testing.T) {
	tip := buildInstallTip(prereq("uv", "brew install uv", ""), true, map[string]bool{}, "darwin")

	assert.Contains(t, tip, "sudo")
	assert.NotContains(t, tip, "Administrator")
}

func TestBuildInstallTip_PermDenied_UnknownOS(t *testing.T) {
	tip := buildInstallTip(prereq("uv", "brew install uv", ""), true, map[string]bool{}, "plan9")

	assert.NotEmpty(t, tip)
	assert.NotContains(t, tip, "sudo")
	assert.NotContains(t, tip, "Administrator")
}

func TestBuildInstallTip_KeyFieldOverridesName(t *testing.T) {
	// Key = "uv" with an unrecognizable Name — Key must take precedence so the
	// correct registry entry is found.
	p := tools.Prerequisite{
		Key:  "uv",
		Name: "SomethingUnrecognized",
		Install: tools.InstallCommands{
			MacOS:   "brew install uv",
			Linux:   "brew install uv",
			Windows: "brew install uv",
		},
	}

	tip := buildInstallTip(p, false, map[string]bool{"pyenv": true}, "linux")

	assert.Contains(t, tip, "pip install uv")
}

func TestBuildInstallTip_VersionSubstituted_Pyenv(t *testing.T) {
	tip := buildInstallTip(prereq("python", "brew install python", "3.9.6"), false, map[string]bool{"pyenv": true}, "linux")

	assert.Contains(t, tip, "pyenv install 3.9.6")
	assert.Contains(t, tip, "pyenv global 3.9.6")
}

func TestBuildInstallTip_VersionSubstituted_Brew(t *testing.T) {
	tip := buildInstallTip(prereq("python", "pyenv install 3.9.6", "3.9.6"), false, map[string]bool{"brew": true}, "linux")

	assert.Contains(t, tip, "brew install python@3.9")
}

func TestBuildInstallTip_EmptyMinimumVersion_DefaultVersionUsed(t *testing.T) {
	// node with nvm detected and no MinimumVersion — DefaultVersion "24" must be
	// substituted so the tip is actionable.
	tip := buildInstallTip(prereq("node", "nvm install 20.0.0", ""), false, map[string]bool{"nvm": true}, "linux")

	assert.Contains(t, tip, "nvm install 24")
	assert.NotContains(t, tip, "{version}")
}

// --- buildInstallFailureMsg tests ---

func TestBuildInstallFailureMsg_AlternativeManagerDetected(t *testing.T) {
	env := map[string]bool{"pyenv": true}

	result := buildInstallFailureMsg(prereq("uv", "brew install uv", ""), 1, false, env, "linux")

	assert.Contains(t, result, "You have pyenv")
	assert.Contains(t, result, "pip install uv")
	assert.NotContains(t, result, "permission denied")
}

func TestBuildInstallFailureMsg_NoManagerDetected(t *testing.T) {
	result := buildInstallFailureMsg(prereq("uv", "brew install uv", ""), 1, false, map[string]bool{}, "linux")

	assert.Contains(t, result, "✗ uv install failed")
	assert.Contains(t, result, "Raw command if you want to retry")
	assert.NotContains(t, result, "You have")
}

func TestBuildInstallFailureMsg_FallbackShownWhenNoManager(t *testing.T) {
	result := buildInstallFailureMsg(prereq("uv", "brew install uv", ""), 1, false, map[string]bool{}, "linux")

	assert.Contains(t, result, "curl")
}

func TestBuildInstallFailureMsg_MultiCommandTip(t *testing.T) {
	env := map[string]bool{"asdf": true}

	result := buildInstallFailureMsg(prereq("uv", "brew install uv", ""), 1, false, env, "linux")

	assert.Contains(t, result, "You have asdf")
	assert.Contains(t, result, TAB+TAB)
}

func TestBuildInstallFailureMsg_PermissionDenied(t *testing.T) {
	result := buildInstallFailureMsg(prereq("uv", "brew install uv", ""), 126, true, map[string]bool{}, "linux")

	assert.Contains(t, result, "permission denied")
	assert.Contains(t, result, "sudo")
}

func TestBuildInstallFailureMsg_AlwaysShowsTriedAndRawCommand(t *testing.T) {
	result := buildInstallFailureMsg(prereq("uv", "brew install uv", ""), 1, false, map[string]bool{}, "linux")

	assert.Contains(t, result, "  Tried: brew install uv")
	assert.Contains(t, result, "  Raw command if you want to retry: brew install uv")
}

func TestBuildInstallFailureMsg_AcceptanceCriteria(t *testing.T) {
	env := map[string]bool{"pyenv": true}

	result := buildInstallFailureMsg(prereq("uv", "brew install uv", ""), 1, false, env, "linux")

	expected := "✗ uv install failed (exit code 1)\n" +
		"  Tried: brew install uv\n" +
		"  Tip: You have pyenv — try: pip install uv\n" +
		"  Raw command if you want to retry: brew install uv\n"
	assert.Equal(t, expected, result)
}

func TestBuildInstallFailureMsg_URLShown(t *testing.T) {
	p := tools.Prerequisite{
		Name: "uv",
		URL:  "https://docs.astral.sh/uv/",
		Install: tools.InstallCommands{
			MacOS:   "brew install uv",
			Linux:   "brew install uv",
			Windows: "brew install uv",
		},
	}

	result := buildInstallFailureMsg(p, 1, false, map[string]bool{}, "linux")

	assert.Contains(t, result, "Refer to https://docs.astral.sh/uv/")
}

func TestBuildInstallFailureMsg_VersionSubstituted_EndToEnd(t *testing.T) {
	// A prerequisite with MinimumVersion set must produce substituted commands
	// in the failure tip all the way through buildInstallFailureMsg.
	result := buildInstallFailureMsg(prereq("python", "brew install python@3.9", "3.9.6"), 1, false, map[string]bool{"pyenv": true}, "linux")

	assert.Contains(t, result, "pyenv install 3.9.6")
	assert.Contains(t, result, "pyenv global 3.9.6")
}
