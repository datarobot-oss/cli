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

package workload

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// start and stop hand the command's own stderr and the id they acted on to the
// renderer: the follow-up names that workload on the stream the user sees, and
// under JSON there is nothing on stderr at all.
func TestStartStop_NextStep(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"status":"accepted","workloadId":"`+wireID+`"}`)
	}))
	t.Cleanup(srv.Close)

	viperx.Set(config.DataRobotURL, srv.URL)
	viperx.Set(config.DataRobotAPIKey, "test-token")
	viperx.Set(config.SkipAuthKey, true)

	t.Cleanup(viperx.Reset)

	testutil.SetTestHomeDir(t, t.TempDir())

	for _, verb := range []string{"start", "stop"} {
		for _, asJSON := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s json=%t", verb, asJSON), func(t *testing.T) {
				args := []string{verb, wireID}
				if asJSON {
					args = append(args, "--output-format", "json")
				}

				var (
					stderr bytes.Buffer
					err    error
				)

				cmd := Cmd()
				cmd.SetArgs(args)
				cmd.SetErr(&stderr)

				stdout := testutil.CaptureStdout(t, func() { err = cmd.Execute() })
				require.NoError(t, err)

				if asJSON {
					assert.True(t, json.Valid([]byte(stdout)), "stdout is the envelope and nothing else")
					assert.Empty(t, stderr.String())

					return
				}

				assert.Equal(t, "accepted\n", stdout)
				assert.Equal(t, "\nNext:\n  dr workload status "+wireID+"  Check the workload status\n",
					ansi.Strip(stderr.String()))
			})
		}
	}
}
