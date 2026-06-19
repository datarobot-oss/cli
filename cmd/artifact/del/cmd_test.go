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

	err := cmd.Execute()
	require.Error(t, err)
}

// In tests stdin is not a terminal, so without --yes the command must stop
// with the confirmation-required guidance before any network call.
func TestCmd_RequiresYesWhenNonInteractive(t *testing.T) {
	t.Setenv(reader.NonInteractiveEnv, "")

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"68b0c1d2e3f4a5b6c7d8e9f0"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "confirmation required")
	assert.Contains(t, err.Error(), "--yes")
}

// Bypass works via --yes or DATAROBOT_CLI_NON_INTERACTIVE; anything else
// requires confirmation (stdin is not a terminal under go test).
func TestConfirmDelete_YesSources(t *testing.T) {
	cases := []struct {
		name          string
		env           string
		setYesFlag    bool
		wantConfirmed bool
		wantErr       bool
	}{
		{name: "env 1 bypasses prompt", env: "1", wantConfirmed: true},
		{name: "env true bypasses prompt", env: "true", wantConfirmed: true},
		{name: "empty env requires confirmation", env: "", wantErr: true},
		{name: "env false requires confirmation", env: "false", wantErr: true},
		{name: "yes flag bypasses prompt", env: "", setYesFlag: true, wantConfirmed: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(reader.NonInteractiveEnv, c.env)

			cmd := Cmd()

			if c.setYesFlag {
				require.NoError(t, cmd.Flags().Set("yes", "true"))
			}

			confirmed, err := confirmDelete(cmd, "art-1")

			if c.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "confirmation required")
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, c.wantConfirmed, confirmed)
		})
	}
}
