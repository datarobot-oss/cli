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

package check

import (
	"testing"

	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckCmd_PropExtractor_AllSatisfied(t *testing.T) {
	orig := tools.RequiredTools

	defer func() { tools.RequiredTools = orig }()

	tools.RequiredTools = []tools.Prerequisite{{Name: "sh", Command: "sh"}}

	cmd := Cmd()

	err := cmd.RunE(cmd, []string{})
	require.NoError(t, err)

	event, ok := telemetry.EventFor(cmd, []string{})
	require.True(t, ok)
	assert.Empty(t, event.EventProperties["missing_deps"])
	assert.Empty(t, event.EventProperties["wrong_version_deps"])
	assert.Empty(t, event.EventProperties["validation_violations"])
}

func TestCheckCmd_PropExtractor_MissingDep(t *testing.T) {
	orig := tools.RequiredTools

	defer func() { tools.RequiredTools = orig }()

	tools.RequiredTools = []tools.Prerequisite{
		{Name: "FakeTool", Command: "nonexistent_dr_fake_xyz", URL: "https://example.com"},
	}

	cmd := Cmd()

	err := cmd.RunE(cmd, []string{})
	require.Error(t, err)

	event, ok := telemetry.EventFor(cmd, []string{})
	require.True(t, ok)

	missingMsgs, _ := event.EventProperties["missing_deps"].([]string)
	require.Len(t, missingMsgs, 1)
	assert.Contains(t, missingMsgs[0], "FakeTool")
	assert.Empty(t, event.EventProperties["wrong_version_deps"])
}

func TestCheckCmd_PropExtractor_WrongVersion(t *testing.T) {
	orig := tools.RequiredTools

	defer func() { tools.RequiredTools = orig }()

	tools.RequiredTools = []tools.Prerequisite{
		{Name: "Echo", Command: "echo 1.0.0", MinimumVersion: "2.0.0", URL: "https://example.com"},
	}

	cmd := Cmd()

	err := cmd.RunE(cmd, []string{})
	require.Error(t, err)

	event, ok := telemetry.EventFor(cmd, []string{})
	require.True(t, ok)

	assert.Empty(t, event.EventProperties["missing_deps"])

	wrongVersionMsgs, _ := event.EventProperties["wrong_version_deps"].([]string)
	require.Len(t, wrongVersionMsgs, 1)
	assert.Contains(t, wrongVersionMsgs[0], "Echo")
}
