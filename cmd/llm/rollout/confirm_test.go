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

package rollout

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/stretchr/testify/require"
)

func serveWorkload(t *testing.T, recipe map[string]any) {
	t.Helper()

	prevSkip := viperx.GetBool(config.SkipAuthKey)
	prevTok := viperx.GetString(config.DataRobotAPIKey)
	prevURL := viperx.GetString(config.DataRobotURL)

	viperx.Set(config.SkipAuthKey, true)
	viperx.Set(config.DataRobotAPIKey, "test-token")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "w1", "runtime": map[string]any{"recipe": recipe}})
	}))

	t.Cleanup(srv.Close)

	viperx.Set(config.DataRobotURL, srv.URL)

	t.Cleanup(func() {
		viperx.Set(config.SkipAuthKey, prevSkip)
		viperx.Set(config.DataRobotAPIKey, prevTok)
		viperx.Set(config.DataRobotURL, prevURL)
	})
}

func TestConfirmRecipe(t *testing.T) {
	p := &plan{workloadID: "w1", toRecipe: "r1", toVersion: 3}

	serveWorkload(t, map[string]any{"id": "r1", "version": 3})
	require.NoError(t, confirmRecipe(p), "the workload names the requested version")

	serveWorkload(t, map[string]any{"id": "r1", "version": 2})
	require.ErrorContains(t, confirmRecipe(p), "unconfirmed", "a vanished replacement is not success")
}
