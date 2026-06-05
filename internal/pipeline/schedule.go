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

// schedule.go wraps the pipeline schedule endpoints described in
// pipelines-api/.../controllers/pipeline_schedule.py. Schedules are only
// valid for locked pipeline versions, so the URL always carries a
// /versions/{ver} segment and there is no Scope parameter on this side.

package pipeline

import (
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ScheduleStatus mirrors PipelineScheduleStatus in the pipelines-api enums.
type ScheduleStatus string

const (
	ScheduleStatusActive  ScheduleStatus = "ACTIVE"
	ScheduleStatusPaused  ScheduleStatus = "PAUSED"
	ScheduleStatusDeleted ScheduleStatus = "DELETED"
)

// Schedule mirrors PipelineScheduleResponse.
type Schedule struct {
	ScheduleID     string         `json:"id"`
	PipelineID     string         `json:"pipelineId"`
	Version        int            `json:"version"`
	CronExpression string         `json:"cronExpression"`
	Timezone       string         `json:"timezone"`
	Status         ScheduleStatus `json:"status"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
}

// ScheduleCreateRequest mirrors PipelineScheduleCreateRequest.
type ScheduleCreateRequest struct {
	CronExpression  string `json:"cron_expression"`
	PipelineInputID string `json:"pipeline_input_id"`
	Timezone        string `json:"timezone,omitempty"`
}

// ScheduleUpdateRequest mirrors PipelineScheduleUpdateRequest. Both fields
// are optional; the API treats omitted values as no-op.
type ScheduleUpdateRequest struct {
	CronExpression *string `json:"cron_expression,omitempty"`
	Timezone       *string `json:"timezone,omitempty"`
}

// CreateSchedule registers a new recurring run for a locked version.
func CreateSchedule(pipelineID string, version int, body ScheduleCreateRequest) (*Schedule, error) {
	endpoint, err := EndpointFor(pipelineID, ScopeLocked, &version, "schedules")
	if err != nil {
		return nil, err
	}

	var result Schedule

	err = doJSON(http.MethodPost, endpoint, body, "create schedule", &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// ListSchedules returns a paginated list of schedules for a locked version.
func ListSchedules(pipelineID string, version, offset, limit int) ([]Schedule, error) {
	endpoint, err := EndpointFor(pipelineID, ScopeLocked, &version, "schedules")
	if err != nil {
		return nil, err
	}

	query := url.Values{}
	if offset > 0 {
		query.Set("offset", strconv.Itoa(offset))
	}

	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}

	if encoded := query.Encode(); encoded != "" {
		endpoint = endpoint + "?" + encoded
	}

	var page DataPage[Schedule]

	err = doJSON(http.MethodGet, endpoint, nil, "schedules", &page)
	if err != nil {
		return nil, err
	}

	return page.Data, nil
}

// GetSchedule fetches a single schedule by id.
func GetSchedule(pipelineID string, version int, scheduleID string) (*Schedule, error) {
	endpoint, err := EndpointFor(pipelineID, ScopeLocked, &version, "schedules/"+scheduleID)
	if err != nil {
		return nil, err
	}

	var schedule Schedule

	err = doJSON(http.MethodGet, endpoint, nil, "schedule", &schedule)
	if err != nil {
		return nil, err
	}

	return &schedule, nil
}

// UpdateSchedule patches a schedule's cron expression and/or timezone.
func UpdateSchedule(pipelineID string, version int, scheduleID string, body ScheduleUpdateRequest) (*Schedule, error) {
	endpoint, err := EndpointFor(pipelineID, ScopeLocked, &version, "schedules/"+scheduleID)
	if err != nil {
		return nil, err
	}

	var result Schedule

	err = doJSON(http.MethodPatch, endpoint, body, "update schedule", &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// DeleteSchedule removes a schedule.
func DeleteSchedule(pipelineID string, version int, scheduleID string) error {
	endpoint, err := EndpointFor(pipelineID, ScopeLocked, &version, "schedules/"+scheduleID)
	if err != nil {
		return err
	}

	return doDelete(endpoint, "delete schedule")
}
