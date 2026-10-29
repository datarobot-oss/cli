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

package runs

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"

	"github.com/datarobot/cli/cmd/job/internal/jobtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const runDoc = `{"id": "run-1", "jobId": "job-1", "artifactId": "art-1", "status": "failed",
	"exitCode": 3, "terminationReason": "Error",
	"createdAt": "2026-10-28T12:00:00Z", "updatedAt": "2026-10-28T12:00:00Z"}`

func execute(t *testing.T, args ...string) error {
	t.Helper()

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs(args)

	return cmd.Execute()
}

func TestRuns_ListsWithEveryStatus(t *testing.T) {
	var asked []string

	jobtest.Serve(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/jobs/job-1/runs/", r.URL.Path)

		asked = r.URL.Query()["status"]

		fmt.Fprintf(w, `{"data": [%s], "totalCount": 1}`, runDoc)
	})

	require.NoError(t, execute(t, "job-1", "--status", "FAILED,errored", "--status", "stopped"))
	assert.Equal(t, []string{"failed", "errored", "stopped"}, asked)
}

func TestRuns_RejectsAnUnknownStatus(t *testing.T) {
	jobtest.Serve(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("no request may be sent")
	})

	err := execute(t, "job-1", "--status", "done")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --status")
}

func TestRuns_ARunIDDisplaysThatRun(t *testing.T) {
	jobtest.Serve(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/jobs/job-1/runs/run-1/", r.URL.Path)

		fmt.Fprint(w, runDoc)
	})

	require.NoError(t, execute(t, "job-1", "run-1"))
}

func TestRuns_ListFlagsDoNotApplyToOneRun(t *testing.T) {
	jobtest.Serve(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("no request may be sent")
	})

	err := execute(t, "job-1", "run-1", "--status", "failed")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "drop them to display one run")
}
