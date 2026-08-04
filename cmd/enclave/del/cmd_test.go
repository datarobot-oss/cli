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

package del

import (
	"testing"

	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCmd_RequiresArg(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{})

	require.Error(t, cmd.Execute())
}

// In tests stdin is not a terminal, so without --yes the command must stop
// with the confirmation-required guidance before any network call.
func TestCmd_RequiresYesWhenNonInteractive(t *testing.T) {
	t.Setenv(reader.NonInteractiveEnv, "")

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"3fa85f64-5717-4562-b3fc-2c963f66afa6"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "confirmation required")
	assert.Contains(t, err.Error(), "--yes")
}

func TestCmd_HasTelemetry(t *testing.T) {
	assert.Contains(t, Cmd().Annotations, "telemetry")
}
