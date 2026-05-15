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

	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	assert.Empty(t, event.EventProperties["missing_msgs"])
	assert.Empty(t, event.EventProperties["install_success"])
	assert.Empty(t, event.EventProperties["install_error"])
}

// TestInstallCmd_OptsYes_SkipsPromptAndInstalls verifies that opts.Yes=true
// bypasses the interactive install prompt and that the PropExtractor captures
// successful installs. The fake prerequisite uses "echo" as its install command
// so the test runs without real tool installation.
func TestInstallCmd_OptsYes_SkipsPromptAndInstalls(t *testing.T) {
	orig := tools.RequiredTools

	defer func() { tools.RequiredTools = orig }()

	tools.RequiredTools = []tools.Prerequisite{
		{
			Name:    "FakeTool",
			Command: "nonexistent_dr_fake_xyz",
			URL:     "https://example.com",
			Install: tools.InstallCommands{
				MacOS:   "echo installed",
				Linux:   "echo installed",
				Windows: "echo installed",
			},
		},
	}

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
}
