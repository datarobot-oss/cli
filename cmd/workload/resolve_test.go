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
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/testutil"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const wireID = "68b0c1d2e3f4a5b6c7d8e9f0"

// TestUnboundID covers the whole feature end to end for every verb that takes
// an id: the workloadId in .datarobot.yaml reaches the request URL, and the
// notice naming it goes to the command's stderr rather than the process's.
// The per-leaf tests only check arity, so this is what catches a leaf that
// forgot to resolve.
func TestUnboundID(t *testing.T) {
	var (
		mu   sync.Mutex
		seen []string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()

		seen = append(seen, r.Method+" "+r.URL.Path)
		mu.Unlock()

		// One body serves every verb: encoding/json ignores unknown fields, so
		// this decodes as a workload, an operation response and a logs page.
		fmt.Fprint(w, `{"id":"`+wireID+`","name":"my-app","status":"running",`+
			`"endpoint":"https://app.example/","data":[]}`)
	}))
	t.Cleanup(srv.Close)

	viperx.Set(config.DataRobotURL, srv.URL)
	viperx.Set(config.DataRobotAPIKey, "test-token")
	viperx.Set(config.SkipAuthKey, true)

	t.Cleanup(viperx.Reset)

	home := t.TempDir()
	testutil.SetTestHomeDir(t, home)

	dir := filepath.Join(home, "proj")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, manifest.FileName),
		[]byte("workloadId: "+wireID+"\nname: my-app\n"), 0o600))
	t.Chdir(dir)

	for _, c := range []struct {
		verb string
		want string
	}{
		{"status", "GET /api/v2/workloads/" + wireID + "/"},
		{"get", "GET /api/v2/workloads/" + wireID + "/"},
		{"endpoint", "GET /api/v2/workloads/" + wireID + "/"},
		{"logs", "GET /api/v2/otel/workload/" + wireID + "/logs/"},
	} {
		t.Run(c.verb, func(t *testing.T) {
			mu.Lock()
			seen = nil
			mu.Unlock()

			var stderr bytes.Buffer

			cmd := Cmd()
			cmd.SetArgs([]string{c.verb})
			cmd.SetErr(&stderr)

			require.NoError(t, cmd.Execute())

			mu.Lock()
			defer mu.Unlock()

			assert.Contains(t, seen, c.want)
			assert.Contains(t, stderr.String(), wireID)
			assert.Contains(t, stderr.String(), manifest.FileName)
		})
	}
}

// The notice is stderr chatter, and stderr chatter breaks
// `--output-format json 2>&1 | jq .`.
func TestUnboundID_SilentInJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"id":"`+wireID+`","name":"my-app","status":"running"}`)
	}))
	t.Cleanup(srv.Close)

	viperx.Set(config.DataRobotURL, srv.URL)
	viperx.Set(config.DataRobotAPIKey, "test-token")
	viperx.Set(config.SkipAuthKey, true)

	t.Cleanup(viperx.Reset)

	home := t.TempDir()
	testutil.SetTestHomeDir(t, home)

	dir := filepath.Join(home, "proj")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, manifest.FileName),
		[]byte("workloadId: "+wireID+"\nname: my-app\n"), 0o600))
	t.Chdir(dir)

	var stderr bytes.Buffer

	cmd := Cmd()
	cmd.SetArgs([]string{"status", "--output-format", "json"})
	cmd.SetErr(&stderr)

	require.NoError(t, cmd.Execute())
	assert.Empty(t, stderr.String())
}
