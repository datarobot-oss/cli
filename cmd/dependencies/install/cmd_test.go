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

package install

import (
	"bytes"
	"testing"

	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeMissingTool is a prerequisite that is never present on the system but
// installs instantly via echo, suitable for testing the install flow without
// real tool installation.
func fakeMissingTool(name string) tools.Prerequisite {
	return tools.Prerequisite{
		Name:    name,
		Command: "nonexistent_dr_fake_xyz",
		URL:     "https://example.com",
		Install: tools.InstallCommands{
			MacOS:   "echo installed",
			Linux:   "echo installed",
			Windows: "echo installed",
		},
	}
}

func TestInstallCmd_PropExtractor_AllSatisfied(t *testing.T) {
	orig := tools.RequiredTools

	defer func() { tools.RequiredTools = orig }()

	tools.RequiredTools = []tools.Prerequisite{{Name: "sh", Command: "sh"}}

	cmd := Cmd()

	var out bytes.Buffer

	cmd.SetOut(&out)

	err := cmd.RunE(cmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, out.String(), "up to date")

	event, ok := telemetry.EventFor(cmd, []string{})
	require.True(t, ok)
	assert.Empty(t, event.EventProperties["missing_deps"])
	assert.Empty(t, event.EventProperties["install_success"])
	assert.Empty(t, event.EventProperties["install_error"])
	assert.Equal(t, false, event.EventProperties["yes_flag"])
	assert.Equal(t, false, event.EventProperties["non_interactive"])
}

// TestInstallCmd_YesFlag_SkipsPromptAndInstalls verifies that --yes bypasses the
// interactive prompt, that the install succeeds, and that the PropExtractor records
// yes_flag=true / non_interactive=false.
func TestInstallCmd_YesFlag_SkipsPromptAndInstalls(t *testing.T) {
	orig := tools.RequiredTools

	defer func() { tools.RequiredTools = orig }()

	tools.RequiredTools = []tools.Prerequisite{fakeMissingTool("FakeTool")}

	cmd := Cmd()
	require.NoError(t, cmd.Flags().Set("yes", "true"))

	var out bytes.Buffer

	cmd.SetOut(&out)

	err := cmd.RunE(cmd, []string{})
	require.NoError(t, err)

	event, ok := telemetry.EventFor(cmd, []string{})
	require.True(t, ok)

	installSuccess, _ := event.EventProperties["install_success"].([]string)
	assert.Contains(t, installSuccess, "FakeTool")
	assert.Empty(t, event.EventProperties["install_error"])
	assert.Equal(t, true, event.EventProperties["yes_flag"])
	assert.Equal(t, false, event.EventProperties["non_interactive"])
}

// TestInstallCmd_NonInteractive_SkipsPromptAndInstalls verifies that
// DATAROBOT_CLI_NON_INTERACTIVE (viperx "yes") bypasses the interactive prompt
// and that the PropExtractor records yes_flag=false / non_interactive=true.
func TestInstallCmd_NonInteractive_SkipsPromptAndInstalls(t *testing.T) {
	orig := tools.RequiredTools

	defer func() { tools.RequiredTools = orig }()

	viperx.Set("yes", true)
	t.Cleanup(func() { viperx.Set("yes", false) })

	tools.RequiredTools = []tools.Prerequisite{fakeMissingTool("FakeTool")}

	cmd := Cmd()

	var out bytes.Buffer

	cmd.SetOut(&out)

	err := cmd.RunE(cmd, []string{})
	require.NoError(t, err)

	event, ok := telemetry.EventFor(cmd, []string{})
	require.True(t, ok)

	installSuccess, _ := event.EventProperties["install_success"].([]string)
	assert.Contains(t, installSuccess, "FakeTool")
	assert.Empty(t, event.EventProperties["install_error"])
	assert.Equal(t, false, event.EventProperties["yes_flag"])
	assert.Equal(t, true, event.EventProperties["non_interactive"])
}
