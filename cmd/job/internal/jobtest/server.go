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

// Package jobtest serves a fake Workload API to the job command tests.
package jobtest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/stretchr/testify/require"
)

// JobDoc is a job as the API returns it.
const JobDoc = `{"id": "job-1", "name": "train", "artifactId": "art-1",
	"createdAt": "2026-10-28T12:00:00Z", "updatedAt": "2026-10-28T12:00:00Z"}`

// Serve points the CLI at a fake API answered by handler, authenticated, for
// the length of the test.
func Serve(t *testing.T, handler http.HandlerFunc) {
	t.Helper()

	prevSkip := viperx.GetBool(config.SkipAuthKey)
	prevTok := viperx.GetString(config.DataRobotAPIKey)
	prevURL := viperx.GetString(config.DataRobotURL)

	srv := httptest.NewServer(handler)

	viperx.Set(config.SkipAuthKey, true)
	viperx.Set(config.DataRobotAPIKey, "test-token")
	viperx.Set(config.DataRobotURL, srv.URL)

	t.Cleanup(func() {
		srv.Close()
		viperx.Set(config.SkipAuthKey, prevSkip)
		viperx.Set(config.DataRobotAPIKey, prevTok)
		viperx.Set(config.DataRobotURL, prevURL)
	})
}

// Body decodes a request's JSON body.
func Body(t *testing.T, r *http.Request) map[string]any {
	t.Helper()

	raw, err := io.ReadAll(r.Body)
	require.NoError(t, err)

	var decoded map[string]any

	require.NoError(t, json.Unmarshal(raw, &decoded))

	return decoded
}
