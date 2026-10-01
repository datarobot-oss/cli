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
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The diagnose hint goes with an empty answer on the plain path only: not
// under JSON, where stderr stays clear, and not under --level, where an
// empty answer is the usual one for a healthy workload.
func TestCmd_EmptyAnswerPointsAtDiagnose(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"data": [], "count": 0, "next": null, "previous": null}`)
	}))

	defer srv.Close()

	viperx.Set(config.DataRobotURL, srv.URL)
	viperx.Set(config.DataRobotAPIKey, "test-token")
	viperx.Set(config.SkipAuthKey, true)

	t.Cleanup(viperx.Reset)

	run := func(args ...string) string {
		cmd := Cmd()
		cmd.PreRunE = nil
		cmd.SetArgs(append([]string{"68b0c1d2e3f4a5b6c7d8e9f0"}, args...))

		var stderr bytes.Buffer

		cmd.SetErr(&stderr)
		require.NoError(t, cmd.Execute())

		return stderr.String()
	}

	assert.Contains(t, run(), "dr workload diagnose 68b0c1d2e3f4a5b6c7d8e9f0")
	assert.NotContains(t, run("--level", "error"), "diagnose", "no error lines is the usual answer under a level")
	assert.Empty(t, run("--output-format", "json"), "JSON keeps stderr clean")
}

func TestCmd_ArgIsOptional(t *testing.T) {
	// Args is called directly: with the id optional, cmd.Execute() would run
	// PreRunE and start a real authentication flow.
	cmd := Cmd()

	require.NoError(t, cmd.Args(cmd, []string{"68b0c1d2e3f4a5b6c7d8e9f0"}))
	require.NoError(t, cmd.Args(cmd, nil))
	require.Error(t, cmd.Args(cmd, []string{"a", "b"}))
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
	assert.Contains(t, err.Error(), "must be a positive integer")
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
