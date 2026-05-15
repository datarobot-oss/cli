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

package start

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/state"
	"github.com/datarobot/cli/internal/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// chdir changes to dir for the duration of the test, restoring the original on cleanup.
func chdir(t *testing.T, dir string) {
	t.Helper()

	original, err := os.Getwd()
	require.NoError(t, err)

	require.NoError(t, os.Chdir(dir))

	t.Cleanup(func() { _ = os.Chdir(original) })
}

func TestCheckSelfVersion_EmptyDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dr-test-*")
	require.NoError(t, err)

	defer os.RemoveAll(tmpDir)

	originalDir, err := os.Getwd()
	require.NoError(t, err)

	defer func() { _ = os.Chdir(originalDir) }()

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	msg := checkSelfVersion(nil)

	completeMsg, ok := msg.(stepCompleteMsg)
	assert.True(t, ok)
	assert.False(t, completeMsg.selfUpdate)
}

func TestCheckSelfVersion_WithVersionRequirement(t *testing.T) {
	// Note: This test verifies the logic when a version requirement exists,
	// but during development (version.Version == "dev"), SufficientSelfVersion
	// always returns true, so we expect no update prompt.
	// This test ensures the code path works correctly in dev mode.
	tmpDir, err := os.MkdirTemp("", "dr-test-*")
	require.NoError(t, err)

	defer os.RemoveAll(tmpDir)

	// Create .datarobot/cli for versions.yaml
	drDir := filepath.Join(tmpDir, ".datarobot", "cli")
	err = os.MkdirAll(drDir, 0o755)
	require.NoError(t, err)

	// Create .datarobot/answers so FindRepoRoot recognizes it as a repo
	answersDir := filepath.Join(tmpDir, ".datarobot", "answers")
	err = os.MkdirAll(answersDir, 0o755)
	require.NoError(t, err)

	versionsYaml := `dr:
  name: DataRobot CLI
  minimum-version: "999.999.999"
`
	err = os.WriteFile(filepath.Join(drDir, "versions.yaml"), []byte(versionsYaml), 0o644)
	require.NoError(t, err)

	originalDir, err := os.Getwd()
	require.NoError(t, err)

	defer func() { _ = os.Chdir(originalDir) }()

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	msg := checkSelfVersion(nil)

	completeMsg, ok := msg.(stepCompleteMsg)
	assert.True(t, ok)

	// In dev mode, version check always passes, so no update prompt
	assert.False(t, completeMsg.selfUpdate, "In dev mode, should not prompt for update")
	assert.False(t, completeMsg.waiting)
}

// setRequiredTools overrides tools.RequiredTools for the duration of the test,
// restoring the original value on cleanup. The caller must be in a directory
// without a .datarobot/answers/ ancestor so GetRequirements() fails and the
// override is not replaced by a real versions.yaml.
func setRequiredTools(t *testing.T, prereqs []tools.Prerequisite) {
	t.Helper()

	orig := tools.RequiredTools

	t.Cleanup(func() { tools.RequiredTools = orig })

	tools.RequiredTools = prereqs
}

// --- checkPrerequisites ---

func TestCheckPrerequisites_AllSatisfied(t *testing.T) {
	chdir(t, t.TempDir())
	setRequiredTools(t, []tools.Prerequisite{
		{Name: "sh", Command: "sh"},
	})

	msg := checkPrerequisites(&Model{})

	_, ok := msg.(stepCompleteMsg)
	assert.True(t, ok, "expected stepCompleteMsg when all deps are satisfied")
}

func TestCheckPrerequisites_MissingTool(t *testing.T) {
	chdir(t, t.TempDir())
	setRequiredTools(t, []tools.Prerequisite{
		{Name: "FakeTool", Command: "nonexistent_dr_fake_xyz", URL: "https://example.com"},
	})

	msg := checkPrerequisites(&Model{})

	depsMsg, ok := msg.(depsMissingMsg)
	require.True(t, ok, "expected depsMissingMsg when a tool is missing")
	require.Len(t, depsMsg.prerequisites, 1)
	assert.Equal(t, "FakeTool", depsMsg.prerequisites[0].Name)
	assert.Contains(t, depsMsg.message, "FakeTool")
}

func TestCheckPrerequisites_WrongVersionTool(t *testing.T) {
	chdir(t, t.TempDir())
	setRequiredTools(t, []tools.Prerequisite{
		{Name: "Echo", Command: "echo 1.0.0", MinimumVersion: "2.0.0", URL: "https://example.com"},
	})

	msg := checkPrerequisites(&Model{})

	depsMsg, ok := msg.(depsMissingMsg)
	require.True(t, ok, "expected depsMissingMsg when tool has wrong version")
	require.Len(t, depsMsg.prerequisites, 1)
	assert.Equal(t, "Echo", depsMsg.prerequisites[0].Name)
}

func TestCheckPrerequisites_SkipsWhenSuccessDepsCheckRecently(t *testing.T) {
	tmpDir := t.TempDir()

	resolved, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	localStateDir := filepath.Join(resolved, ".datarobot", "cli")
	err = os.MkdirAll(localStateDir, 0o755)
	require.NoError(t, err)

	err = state.UpdateAfterSuccessDepsCheck(resolved)
	require.NoError(t, err)

	msg := checkPrerequisites(&Model{repoRoot: resolved})

	_, ok := msg.(stepCompleteMsg)
	assert.True(t, ok, "expected stepCompleteMsg when deps were installed within 24 hours")
}

// --- handleDepsMissing ---

func TestHandleDepsMissing_WaitsForConfirmation(t *testing.T) {
	m := Model{}

	msg := depsMissingMsg{
		prerequisites: []tools.Prerequisite{{Name: "uv"}},
		message:       "missing: uv",
	}

	result, cmd := m.handleDepsMissing(msg)

	resultModel := result.(Model)
	assert.True(t, resultModel.waitingToInstall)
	assert.Equal(t, "missing: uv", resultModel.stepCompleteMessage)
	assert.Equal(t, msg.prerequisites, resultModel.depsToInstall)
	assert.Nil(t, cmd)
}

func TestHandleDepsMissing_AutoInstallsWhenAnswerYes(t *testing.T) {
	viperx.Set("yes", true)
	t.Cleanup(func() { viperx.Set("yes", false) })

	m := Model{}

	msg := depsMissingMsg{
		prerequisites: []tools.Prerequisite{{Name: "uv"}},
		message:       "missing: uv",
	}

	result, cmd := m.handleDepsMissing(msg)

	resultModel := result.(Model)
	assert.False(t, resultModel.waitingToInstall)
	assert.NotNil(t, cmd)
}

func TestHandleDepsMissing_AutoInstallsWhenAnswerYesFlag(t *testing.T) {
	m := Model{opts: Options{AnswerYes: true}}

	msg := depsMissingMsg{
		prerequisites: []tools.Prerequisite{{Name: "uv"}},
		message:       "missing: uv",
	}

	result, cmd := m.handleDepsMissing(msg)

	resultModel := result.(Model)
	assert.False(t, resultModel.waitingToInstall)
	assert.NotNil(t, cmd)
}

func TestHandleDepsMissing_CapturesTelemetry(t *testing.T) {
	m := Model{}

	checkResult := tools.CheckResult{
		ValidationViolations: []string{"[uv] 'name' is required"},
		MissingMsgs:          []string{"uv (https://example.com)"},
		WrongVersionMsgs:     []string{"task (minimal: v3.35.0, installed: v3.32.0)"},
	}

	msg := depsMissingMsg{
		prerequisites: []tools.Prerequisite{{Name: "uv"}},
		message:       "missing: uv",
		checkResult:   checkResult,
	}

	result, _ := m.handleDepsMissing(msg)

	resultModel := result.(Model)
	assert.Equal(t, checkResult.ValidationViolations, resultModel.telemetry.validationViolations)
	assert.Equal(t, checkResult.MissingMsgs, resultModel.telemetry.missingMsgs)
	assert.Equal(t, checkResult.WrongVersionMsgs, resultModel.telemetry.wrongVersionMsgs)
}

// --- handleInstallConfirmKey ---

func TestHandleInstallConfirmKey_ConfirmKeys(t *testing.T) {
	confirmKeys := []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("y")},
		{Type: tea.KeyRunes, Runes: []rune("Y")},
		{Type: tea.KeyEnter},
	}

	for _, key := range confirmKeys {
		m := Model{waitingToInstall: true}

		result, cmd := m.handleInstallConfirmKey(key)

		resultModel := result.(Model)
		assert.False(t, resultModel.waitingToInstall, "key %q should clear waitingToInstall", key.String())
		assert.NotNil(t, cmd, "key %q should return an install Cmd", key.String())
	}
}

func TestHandleInstallConfirmKey_CancelKeys(t *testing.T) {
	cancelKeys := []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("n")},
		{Type: tea.KeyRunes, Runes: []rune("N")},
		{Type: tea.KeyRunes, Runes: []rune("q")},
		{Type: tea.KeyEsc},
	}

	for _, key := range cancelKeys {
		m := Model{waitingToInstall: true}

		result, _ := m.handleInstallConfirmKey(key)

		resultModel := result.(Model)
		require.Error(t, resultModel.err, "key %q should set an error", key.String())
		assert.Contains(t, resultModel.err.Error(), "Installation cancelled", "key %q error message", key.String())
	}
}

func TestHandleInstallConfirmKey_OtherKeysIgnored(t *testing.T) {
	m := Model{waitingToInstall: true}

	result, cmd := m.handleInstallConfirmKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})

	resultModel := result.(Model)
	assert.True(t, resultModel.waitingToInstall, "unknown key should not clear waitingToInstall")
	assert.Nil(t, cmd, "unknown key should return nil Cmd")
}

// --- handleDepsInstallComplete ---

func TestHandleDepsInstallComplete_Success(t *testing.T) {
	m := Model{
		steps: []step{
			{description: "step 1", fn: startQuickstart},
			{description: "step 2", fn: startQuickstart},
		},
		current: 0,
	}

	msg := depsInstallCompleteMsg{err: nil, output: "✅ All dependencies installed successfully.\n", installed: []string{"uv"}}

	result, cmd := m.handleDepsInstallComplete(msg)

	resultModel := result.(Model)
	assert.Equal(t, 1, resultModel.current, "should advance to next step on success")
	assert.Equal(t, msg.output, resultModel.stepCompleteMessage)
	assert.NotNil(t, cmd)
}

func TestHandleDepsInstallComplete_CapturesSuccessTelemetry(t *testing.T) {
	m := Model{}

	msg := depsInstallCompleteMsg{
		err:       nil,
		output:    "✅ All dependencies installed successfully.\n",
		installed: []string{"uv", "task"},
	}

	result, _ := m.handleDepsInstallComplete(msg)

	resultModel := result.(Model)
	assert.Equal(t, []string{"uv", "task"}, resultModel.telemetry.installSuccess)
	assert.Empty(t, resultModel.telemetry.installError)
}

func TestHandleDepsInstallComplete_Error(t *testing.T) {
	m := Model{}

	installErr := errors.New("install failed for \"uv\" (exit code 1)")
	msg := depsInstallCompleteMsg{err: installErr}

	result, _ := m.handleDepsInstallComplete(msg)

	resultModel := result.(Model)
	assert.Equal(t, installErr, resultModel.err)
}

func TestHandleDepsInstallComplete_CapturesErrorTelemetry(t *testing.T) {
	m := Model{}

	installErr := errors.New("install failed for \"uv\" (exit code 1)")
	msg := depsInstallCompleteMsg{err: installErr, installed: nil}

	result, _ := m.handleDepsInstallComplete(msg)

	resultModel := result.(Model)
	assert.Empty(t, resultModel.telemetry.installSuccess)
	assert.Equal(t, installErr.Error(), resultModel.telemetry.installError)
}

// --- View ---

func TestView_WaitingToInstall_ShowsInstallFooter(t *testing.T) {
	m := Model{
		steps:            []step{{description: "Checking prerequisites...", fn: startQuickstart}},
		waitingToInstall: true,
	}

	view := m.View()

	assert.Contains(t, view, "install")
	assert.NotContains(t, view, "confirm")
}

func TestView_WaitingToExecute_ShowsConfirmFooter(t *testing.T) {
	m := Model{
		steps:            []step{{description: "Finding start command...", fn: startQuickstart}},
		waitingToExecute: true,
	}

	view := m.View()

	assert.Contains(t, view, "confirm")
	assert.NotContains(t, view, "install")
}
