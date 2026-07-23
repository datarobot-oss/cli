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

package config

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/wlconfig"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestCmd builds the config command with auth bypassed and output captured.
func newTestCmd(args []string) (*cobra.Command, *bytes.Buffer) {
	cmd := Cmd()
	cmd.PreRunE = nil

	var out bytes.Buffer

	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)

	return cmd, &out
}

// swapSelectionFns injects fakes for the network + TUI selection seams and
// forces the interactive (terminal) path so the picker is reachable in tests.
func swapSelectionFns(
	t *testing.T,
	list func(int, []string) ([]workload.Workload, error),
	pick func([]workload.Workload) (workloadItem, error),
	ask func(string) (string, error),
) {
	t.Helper()

	origList, origPick, origAsk, origTerm, origPrompt := listWorkloadsFn, runPickerFn, askFn, isStdinTerminalFn, promptBuildFn

	if list != nil {
		listWorkloadsFn = list
	}

	if pick != nil {
		runPickerFn = pick
	}

	if ask != nil {
		askFn = ask
	}

	isStdinTerminalFn = func() bool { return true }
	// Stub the build-mode prompt so interactive tests don't read real stdin.
	promptBuildFn = func(*cobra.Command, *wlconfig.Config) error { return nil }

	t.Cleanup(func() {
		listWorkloadsFn, runPickerFn, askFn, isStdinTerminalFn, promptBuildFn = origList, origPick, origAsk, origTerm, origPrompt
	})
}

// failPicker is a runPickerFn stub that fails the test if the picker is reached.
func failPicker(t *testing.T) func([]workload.Workload) (workloadItem, error) {
	return func([]workload.Workload) (workloadItem, error) {
		t.Error("picker should not be invoked")

		return workloadItem{}, nil
	}
}

func TestConfig_YesWithWorkloadID(t *testing.T) {
	dir := t.TempDir()

	cmd, out := newTestCmd([]string{"--yes", "--dir", dir, "--workload-id", "wl-xyz"})
	require.NoError(t, cmd.Execute())

	cfg, err := wlconfig.Load(dir)
	require.NoError(t, err)
	assert.Equal(t, "wl-xyz", cfg.WorkloadID)
	assert.Contains(t, out.String(), wlconfig.Path(dir))
}

func TestConfig_YesWithName(t *testing.T) {
	dir := t.TempDir()

	cmd, _ := newTestCmd([]string{"--yes", "--dir", dir, "--name", "my-app"})
	require.NoError(t, cmd.Execute())

	cfg, err := wlconfig.Load(dir)
	require.NoError(t, err)
	assert.Equal(t, "my-app", cfg.Name)
	assert.Empty(t, cfg.WorkloadID)
}

func TestConfig_YesWithNeitherErrors(t *testing.T) {
	dir := t.TempDir()

	cmd, _ := newTestCmd([]string{"--yes", "--dir", dir})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--workload-id")
}

func TestConfig_JSONOutput(t *testing.T) {
	dir := t.TempDir()

	cmd, out := newTestCmd([]string{"--yes", "--dir", dir, "--name", "app", "--output-format", "json"})
	require.NoError(t, cmd.Execute())

	var res configResult

	require.NoError(t, json.Unmarshal(out.Bytes(), &res))
	assert.Equal(t, "app", res.Name)
	assert.True(t, res.CreateOnUp)
	assert.Equal(t, wlconfig.Path(dir), res.Path)
}

func TestConfig_InteractivePickExisting(t *testing.T) {
	dir := t.TempDir()

	swapSelectionFns(t,
		func(int, []string) ([]workload.Workload, error) {
			return []workload.Workload{{ID: "wl-1", Name: "a", Status: "running"}}, nil
		},
		func([]workload.Workload) (workloadItem, error) {
			return workloadItem{id: "wl-1", name: "a"}, nil
		},
		nil,
	)

	cmd, _ := newTestCmd([]string{"--dir", dir})
	require.NoError(t, cmd.Execute())

	cfg, err := wlconfig.Load(dir)
	require.NoError(t, err)
	assert.Equal(t, "wl-1", cfg.WorkloadID)
	assert.Equal(t, "a", cfg.Name)
}

func TestConfig_InteractivePickCreateNew(t *testing.T) {
	dir := t.TempDir()

	swapSelectionFns(t,
		func(int, []string) ([]workload.Workload, error) {
			return []workload.Workload{{ID: "wl-1", Name: "a"}}, nil
		},
		func([]workload.Workload) (workloadItem, error) {
			return workloadItem{id: createNewID}, nil
		},
		func(string) (string, error) { return "newapp", nil },
	)

	cmd, _ := newTestCmd([]string{"--dir", dir})
	require.NoError(t, cmd.Execute())

	cfg, err := wlconfig.Load(dir)
	require.NoError(t, err)
	assert.Equal(t, "newapp", cfg.Name)
	assert.Empty(t, cfg.WorkloadID)
}

func TestConfig_WorkloadIDWithoutYesIsHonored(t *testing.T) {
	dir := t.TempDir()

	// Flags win even without --yes and must not touch the picker.
	swapSelectionFns(t, nil, failPicker(t), nil)

	cmd, _ := newTestCmd([]string{"--dir", dir, "--workload-id", "wl-flag"})
	require.NoError(t, cmd.Execute())

	cfg, err := wlconfig.Load(dir)
	require.NoError(t, err)
	assert.Equal(t, "wl-flag", cfg.WorkloadID)
}

func TestConfig_RerunPreservesRecordedWorkloadID(t *testing.T) {
	dir := t.TempDir()

	// A previous config+up recorded a workload id.
	require.NoError(t, wlconfig.Save(dir, wlconfig.Config{WorkloadID: "wl-old", Name: "first"}))

	// Re-running config with a new name must keep the binding: a draft artifact
	// allows only one workload, so dropping the id causes a 409 on the next up.
	cmd, _ := newTestCmd([]string{"--yes", "--dir", dir, "--name", "renamed"})
	require.NoError(t, cmd.Execute())

	cfg, err := wlconfig.Load(dir)
	require.NoError(t, err)
	assert.Equal(t, "wl-old", cfg.WorkloadID)
	assert.Equal(t, "renamed", cfg.Name)
}

func TestConfig_RerunExplicitWorkloadIDOverrides(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, wlconfig.Save(dir, wlconfig.Config{WorkloadID: "wl-old", Name: "first"}))

	cmd, _ := newTestCmd([]string{"--yes", "--dir", dir, "--workload-id", "wl-new"})
	require.NoError(t, cmd.Execute())

	cfg, err := wlconfig.Load(dir)
	require.NoError(t, err)
	assert.Equal(t, "wl-new", cfg.WorkloadID, "an explicit --workload-id rebinds")
}

func TestConfig_NonInteractiveWithoutFlagsErrors(t *testing.T) {
	dir := t.TempDir()

	origTerm := isStdinTerminalFn
	isStdinTerminalFn = func() bool { return false }

	t.Cleanup(func() { isStdinTerminalFn = origTerm })

	cmd, _ := newTestCmd([]string{"--dir", dir})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-interactive")
}

func TestConfig_InteractiveNoExistingWorkloadsPromptsName(t *testing.T) {
	dir := t.TempDir()

	// With zero existing workloads the picker is skipped entirely.
	swapSelectionFns(t,
		func(int, []string) ([]workload.Workload, error) { return nil, nil },
		failPicker(t),
		func(string) (string, error) { return "fresh", nil },
	)

	cmd, _ := newTestCmd([]string{"--dir", dir})
	require.NoError(t, cmd.Execute())

	cfg, err := wlconfig.Load(dir)
	require.NoError(t, err)
	assert.Equal(t, "fresh", cfg.Name)
	assert.Empty(t, cfg.WorkloadID)
}
