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

func TestCmd_RequiresArg(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"--role", "user", "--user", "alice"})

	require.Error(t, cmd.Execute())
}

func TestCmd_RequiresRole(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"enc-1", "--user", "alice"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "role")
}

func TestCmd_RejectsMultipleRecipients(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"enc-1", "--role", "user", "--user", "alice", "--org", "org-1"})

	require.Error(t, cmd.Execute())
}

func TestCmd_HasTelemetry(t *testing.T) {
	assert.Contains(t, Cmd().Annotations, "telemetry")
}
