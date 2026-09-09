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

package stop

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/internal/testutil"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const boundID = "68b0c1d2e3f4a5b6c7d8e9f0"

// A workload the user did not name is confirmed before it is stopped; one they
// typed is not, because the id is the consent. Under go test stdin is never a
// terminal, so an unanswered prompt surfaces as the non-interactive refusal.
// `start` shares this code path.
func TestCmd_ConfirmsOnlyAnAmbientWorkload(t *testing.T) {
	for _, c := range []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "manifest-named waits to be asked", args: []string{}, wantErr: "confirmation required"},
		{name: "--yes skips the question", args: []string{"--yes"}},
		{name: "a typed id is never asked", args: []string{boundID}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(reader.NonInteractiveEnv, "")

			var (
				mu     sync.Mutex
				stops  int
				server = func() *httptest.Server {
					return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						mu.Lock()
						stops++
						mu.Unlock()

						fmt.Fprint(w, `{"status":"stopping","workloadId":"`+boundID+`"}`)
					}))
				}()
			)

			t.Cleanup(server.Close)

			viperx.Set(config.DataRobotURL, server.URL)
			viperx.Set(config.DataRobotAPIKey, "test-token")
			viperx.Set(config.SkipAuthKey, true)

			t.Cleanup(viperx.Reset)

			home := t.TempDir()
			testutil.SetTestHomeDir(t, home)

			dir := filepath.Join(home, "proj")
			require.NoError(t, os.MkdirAll(dir, 0o750))
			require.NoError(t, os.WriteFile(filepath.Join(dir, manifest.FileName),
				[]byte("workloadId: "+boundID+"\nname: my-app\n"), 0o600))
			t.Chdir(dir)

			cmd := Cmd()
			cmd.SetArgs(c.args)

			err := cmd.Execute()

			mu.Lock()
			defer mu.Unlock()

			if c.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), c.wantErr)
				assert.Zero(t, stops, "nothing may be stopped before the question is answered")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, 1, stops)
		})
	}
}
