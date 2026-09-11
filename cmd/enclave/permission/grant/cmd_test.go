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

package grant

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCmd_RequiresPermission(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"--org", "org-1"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission")
}

func TestCmd_RejectsUnknownPermission(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"--permission", "deploy", "--org", "org-1"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create")
}

func TestCmd_RejectsMultipleRecipients(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"--permission", "create", "--org", "org-1", "--group", "grp-1"})

	require.Error(t, cmd.Execute())
}

func TestCmd_RejectsUsernameSelector(t *testing.T) {
	// The createAccess endpoint identifies recipients by id only, so --user is
	// deliberately not offered here (unlike "dr enclave access").
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"--permission", "create", "--user", "alice@corp.io"})

	require.Error(t, cmd.Execute())
}

func TestCmd_TakesNoPositionalArgs(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"enc-1", "--permission", "create", "--org", "org-1"})

	require.Error(t, cmd.Execute())
}

func TestCmd_HasTelemetry(t *testing.T) {
	assert.Contains(t, Cmd().Annotations, "telemetry")
}
