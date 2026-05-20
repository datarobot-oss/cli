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

package pipeline

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateSchedule_LockedOnlyURLAndBody(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v2/pipelines/p-1/versions/2/schedules", r.URL.Path)

		var body ScheduleCreateRequest

		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "0 * * * *", body.CronExpression)
		assert.Equal(t, "in-1", body.PipelineInputID)
		assert.Equal(t, "America/Los_Angeles", body.Timezone)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schedule_id":"s-1","pipeline_id":"p-1","version":2,"cron_expression":"0 * * * *","timezone":"America/Los_Angeles","status":"ACTIVE"}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	got, err := CreateSchedule("p-1", 2, ScheduleCreateRequest{
		CronExpression:  "0 * * * *",
		PipelineInputID: "in-1",
		Timezone:        "America/Los_Angeles",
	})
	require.NoError(t, err)
	assert.Equal(t, "s-1", got.ScheduleID)
	assert.Equal(t, ScheduleStatusActive, got.Status)
}

func TestListSchedules_QueryAndDecode(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/pipelines/p-1/versions/2/schedules", r.URL.Path)
		assert.Equal(t, "5", r.URL.Query().Get("limit"))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"schedule_id":"s-1","pipeline_id":"p-1","version":2,"cron_expression":"0 0 * * *","timezone":"UTC","status":"ACTIVE"}]`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	items, err := ListSchedules("p-1", 2, 0, 5)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "s-1", items[0].ScheduleID)
}

func TestGetSchedule_TargetsCorrectURL(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/pipelines/p-1/versions/2/schedules/s-1", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schedule_id":"s-1","pipeline_id":"p-1","version":2,"cron_expression":"0 * * * *","timezone":"UTC","status":"PAUSED"}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	got, err := GetSchedule("p-1", 2, "s-1")
	require.NoError(t, err)
	assert.Equal(t, ScheduleStatusPaused, got.Status)
}

func TestUpdateSchedule_OmitsUnsuppliedFields(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, "/api/v2/pipelines/p-1/versions/2/schedules/s-1", r.URL.Path)

		var raw map[string]any

		assert.NoError(t, json.NewDecoder(r.Body).Decode(&raw))
		// Only cron_expression should be in the body; timezone is omitted.
		assert.Equal(t, "*/15 * * * *", raw["cron_expression"])
		_, hasTZ := raw["timezone"]
		assert.False(t, hasTZ, "expected timezone to be omitted")

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schedule_id":"s-1","pipeline_id":"p-1","version":2,"cron_expression":"*/15 * * * *","timezone":"UTC","status":"ACTIVE"}`))
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	cron := "*/15 * * * *"
	got, err := UpdateSchedule("p-1", 2, "s-1", ScheduleUpdateRequest{CronExpression: &cron})
	require.NoError(t, err)
	assert.Equal(t, "*/15 * * * *", got.CronExpression)
}

func TestDeleteSchedule_DeletesLockedURL(t *testing.T) {
	installSkipAuth(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/api/v2/pipelines/p-1/versions/2/schedules/s-1", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))

	defer srv.Close()

	installEndpoint(t, srv.URL)

	require.NoError(t, DeleteSchedule("p-1", 2, "s-1"))
}
