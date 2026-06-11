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

package logs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCmd_RequiresArg(t *testing.T) {
	cmd := Cmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.Error(t, err)
}

func TestCmd_InvalidOutputFormat(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"68b0c1d2e3f4a5b6c7d8e9f0", "--output-format", "yaml"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid output format "yaml"`)
}

func TestCmd_RejectsNonPositiveLimit(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"68b0c1d2e3f4a5b6c7d8e9f0", "--limit", "0"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --limit 0")
}

func TestCmd_ParsesFollowFlag(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil

	// -f is the shorthand for --follow.
	require.NoError(t, cmd.ParseFlags([]string{"-f", "--poll-interval", "500ms"}))

	follow, _ := cmd.Flags().GetBool("follow")
	interval, _ := cmd.Flags().GetDuration("poll-interval")

	assert.True(t, follow)
	assert.Equal(t, "500ms", interval.String())
}

func TestCmd_HidesPollInterval(t *testing.T) {
	assert.True(t, Cmd().Flag("poll-interval").Hidden)
}

func TestCmd_RejectsInvalidLevel(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"68b0c1d2e3f4a5b6c7d8e9f0", "--level", "eror"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid log level "eror"`)
}

func TestCmd_RejectsNonPositivePollInterval(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"68b0c1d2e3f4a5b6c7d8e9f0", "--follow", "--poll-interval", "0s"})

	// Rejected at flag-parse time by pollflags.PositiveDuration, before any
	// request is made.
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a positive duration")
}
