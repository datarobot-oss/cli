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

package cancel

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"

	"github.com/datarobot/cli/cmd/job/internal/jobtest"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func execute(t *testing.T, args ...string) error {
	t.Helper()

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs(args)

	return cmd.Execute()
}

func TestCancel_WithYesCancels(t *testing.T) {
	var cancelled bool

	jobtest.Serve(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v2/jobs/job-1/runs/run-1/cancel", r.URL.Path)

		cancelled = true

		fmt.Fprint(w, `{"id": "run-1", "jobId": "job-1", "artifactId": "art-1", "status": "stopping",
			"createdAt": "2026-10-28T12:00:00Z", "updatedAt": "2026-10-28T12:00:00Z"}`)
	})

	require.NoError(t, execute(t, "job-1", "run-1", "--yes"))
	assert.True(t, cancelled)
}

func TestCancel_ARunThatEndedShowsTheServersReason(t *testing.T) {
	jobtest.Serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, `{"detail": "Run run-1 already ended."}`)
	})

	err := execute(t, "job-1", "run-1", "--yes")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "already ended")
}

func TestCancel_WithoutAnAnswerCancelsNothing(t *testing.T) {
	t.Setenv(reader.NonInteractiveEnv, "")

	jobtest.Serve(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("no request may be sent")
	})

	err := execute(t, "job-1", "run-1", "--output-format", "json")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "pass --yes")
}
