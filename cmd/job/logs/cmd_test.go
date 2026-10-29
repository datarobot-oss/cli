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
	"testing"

	"github.com/datarobot/cli/cmd/job/internal/jobtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func execute(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	var out, errOut bytes.Buffer

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)

	err = cmd.Execute()

	return out.String(), errOut.String(), err
}

func TestLogs_ReadsTheRunsLinesFromTheJobsWorkload(t *testing.T) {
	jobtest.Serve(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/otel/workload/job-1/logs/", r.URL.Path)
		assert.Equal(t, "proton_id", r.URL.Query().Get("searchKeys"))
		assert.Equal(t, "run-1", r.URL.Query().Get("searchValues"))

		fmt.Fprint(w, `{"data": [{"timestamp": "2026-10-28 12:00:00", "level": "INFO", "message": "done"}], "count": 1}`)
	})

	_, _, err := execute(t, "job-1", "run-1")

	require.NoError(t, err)
}

func TestLogs_SaysWhatAnEmptyLogCanMean(t *testing.T) {
	jobtest.Serve(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"data": [], "count": 0}`)
	})

	_, stderr, err := execute(t, "job-1", "run-1")

	require.NoError(t, err)
	assert.Contains(t, stderr, "No logs for run run-1 yet")
}

func TestLogs_RefusesAnEmptySearchTerm(t *testing.T) {
	jobtest.Serve(t, func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("no request may be sent")
	})

	_, _, err := execute(t, "job-1", "run-1", "--grep", " ")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty search term")
}
