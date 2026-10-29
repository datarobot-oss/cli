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

package job

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	jobDoc = `{"id": "job-1", "name": "train", "artifactId": "art-1",
		"runtime": {"containerGroups": [{"name": "default", "resourceBundles": ["gpu.a100.1"]}]},
		"createdAt": "2026-10-28T12:00:00Z", "updatedAt": "2026-10-28T12:00:00Z"}`
	runDoc = `{"id": "run-1", "jobId": "job-1", "artifactId": "art-1", "status": "%s",
		"createdAt": "2026-10-28T12:00:00Z", "updatedAt": "2026-10-28T12:00:00Z"}`
)

func serve(t *testing.T, handler http.HandlerFunc) {
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

func body(t *testing.T, r *http.Request) map[string]any {
	t.Helper()

	raw, err := io.ReadAll(r.Body)
	require.NoError(t, err)

	var decoded map[string]any

	require.NoError(t, json.Unmarshal(raw, &decoded))

	return decoded
}

func TestCreateJob_PostsTheRequestAndKeepsTheRuntimeVerbatim(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v2/jobs/", r.URL.Path)

		sent := body(t, r)
		assert.Equal(t, "train", sent["name"])
		assert.Equal(t, "art-1", sent["artifactId"])
		assert.Equal(t, "uc-1", sent["useCaseId"])
		assert.Contains(t, sent["runtime"], "futureField")

		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, jobDoc)
	})

	job, err := CreateJob(map[string]any{
		"name":       "train",
		"artifactId": "art-1",
		"useCaseId":  "uc-1",
		"runtime":    map[string]any{"futureField": true},
	})

	require.NoError(t, err)
	assert.Equal(t, "job-1", job.ID)
	assert.JSONEq(t, `{"containerGroups": [{"name": "default", "resourceBundles": ["gpu.a100.1"]}]}`,
		string(job.Runtime))
}

func TestCreateJob_ShowsTheServersReason(t *testing.T) {
	serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"detail": "Jobs are disabled on this cluster"}`)
	})

	_, err := CreateJob(map[string]any{"name": "train", "artifactId": "art-1"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Jobs are disabled on this cluster")
}

func TestGetJob_EscapesTheID(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/jobs/job%3F1/", r.URL.EscapedPath())
		fmt.Fprint(w, jobDoc)
	})

	_, err := GetJob("job?1")

	require.NoError(t, err)
}

func TestListJobs_FiltersByNameAndFollowsPages(t *testing.T) {
	var calls int

	serve(t, func(w http.ResponseWriter, r *http.Request) {
		calls++

		assert.Equal(t, "/api/v2/jobs/", r.URL.Path)

		if calls == 1 {
			assert.Equal(t, "train", r.URL.Query().Get("name"))

			next := viperx.GetString(config.DataRobotURL) + "/api/v2/jobs/?offset=1"
			fmt.Fprintf(w, `{"data": [%s], "totalCount": 2, "next": %q}`, jobDoc, next)

			return
		}

		fmt.Fprintf(w, `{"data": [%s], "totalCount": 2, "next": null}`, jobDoc)
	})

	jobs, err := ListJobs("train", 10, 0)

	require.NoError(t, err)
	assert.Len(t, jobs, 2)
	assert.Equal(t, 2, calls)
}

func TestUpdateJob_PatchesOnlyTheGivenFields(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, "/api/v2/jobs/job-1/", r.URL.Path)
		assert.Equal(t, map[string]any{"artifactId": "art-2"}, body(t, r))
		fmt.Fprint(w, jobDoc)
	})

	_, err := UpdateJob("job-1", map[string]any{"artifactId": "art-2"})

	require.NoError(t, err)
}

func TestDeleteJob(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/api/v2/jobs/job-1/", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	})

	require.NoError(t, DeleteJob("job-1"))
}

func TestTriggerRun_AcceptsTheRun(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v2/jobs/job-1/runs/", r.URL.Path)
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprintf(w, runDoc, "submitted")
	})

	run, err := TriggerRun("job-1")

	require.NoError(t, err)
	assert.Equal(t, "run-1", run.ID)
	assert.Equal(t, StatusSubmitted, run.Status)
}

func TestTriggerRun_ShowsTheRunLimit(t *testing.T) {
	serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprint(w, `{"detail": "Only 5 active run(s) are allowed per user."}`)
	})

	_, err := TriggerRun("job-1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Only 5 active run(s)")
}

func TestListRuns_SendsEveryStatus(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/jobs/job-1/runs/", r.URL.Path)
		assert.Equal(t, []string{"failed", "errored"}, r.URL.Query()["status"])
		assert.Equal(t, "5", r.URL.Query().Get("limit"))
		fmt.Fprintf(w, `{"data": [`+runDoc+`], "totalCount": 1, "next": null}`, "failed")
	})

	runs, err := ListRuns("job-1", []string{"failed", "errored"}, 5, 0)

	require.NoError(t, err)
	assert.Len(t, runs, 1)
}

func TestGetRun_ReadsHowItEnded(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/jobs/job-1/runs/run-1/", r.URL.Path)
		fmt.Fprint(w, `{"id": "run-1", "jobId": "job-1", "status": "failed", "exitCode": 137,
			"terminationReason": "OOMKilled", "startedAt": "2026-10-28T12:00:00Z",
			"finishedAt": "2026-10-28T12:30:00Z", "createdAt": "2026-10-28T12:00:00Z",
			"updatedAt": "2026-10-28T12:30:00Z"}`)
	})

	run, err := GetRun("job-1", "run-1")

	require.NoError(t, err)
	require.NotNil(t, run.ExitCode)
	assert.Equal(t, 137, *run.ExitCode)
	assert.Equal(t, "OOMKilled", run.TerminationReason)
	assert.True(t, run.Ended())
	assert.False(t, run.Succeeded())
}

func TestCancelRun(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v2/jobs/job-1/runs/run-1/cancel", r.URL.Path)
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprintf(w, runDoc, "stopping")
	})

	run, err := CancelRun("job-1", "run-1")

	require.NoError(t, err)
	assert.Equal(t, StatusStopping, run.Status)
}

func TestCancelRun_ARunThatEndedIsAConflict(t *testing.T) {
	serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, `{"detail": "Run run-1 already ended as succeeded"}`)
	})

	_, err := CancelRun("job-1", "run-1")

	var httpErr *drapi.HTTPError

	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusConflict, httpErr.StatusCode)
	assert.Contains(t, err.Error(), "already ended as succeeded")
}

func TestParseRunStatuses(t *testing.T) {
	parsed, err := ParseRunStatuses([]string{"FAILED", " errored "})

	require.NoError(t, err)
	assert.Equal(t, []string{"failed", "errored"}, parsed)

	_, err = ParseRunStatuses([]string{"bogus"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be one of")
}

func TestRunEnded(t *testing.T) {
	for _, status := range []string{StatusSucceeded, StatusFailed, StatusErrored, StatusStopped} {
		assert.True(t, Run{Status: status}.Ended(), status)
	}

	for _, status := range []string{StatusSubmitted, StatusProvisioning, StatusLaunching, StatusRunning, StatusStopping} {
		assert.False(t, Run{Status: status}.Ended(), status)
	}
}
