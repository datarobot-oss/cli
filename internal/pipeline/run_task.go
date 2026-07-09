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

// run_task.go wraps the per-task execution endpoints under
// /api/v2/pipelines/{pid}/dispatches/{did}/tasks/...
// These endpoints expose the lifecycle, logs, and result of individual
// @task electrons within a dispatch (run).

package pipeline

import (
	"net/url"
	"strconv"
	"time"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
)

// TaskExecution mirrors TaskExecutionResponse.
type TaskExecution struct {
	TaskID      *int       `json:"taskId,omitempty"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	ErrorDetail *string    `json:"errorDetail,omitempty"`
}

// TaskExecutionLogs mirrors TaskExecutionLogsResponse (live K8s pod logs).
type TaskExecutionLogs struct {
	Logs              string `json:"logs"`
	FilteredLineCount int    `json:"filteredLineCount"`
}

// TaskExecutionDurableLog mirrors TaskExecutionDurableLogResponse
// (S3-uploaded log content read back inline).
type TaskExecutionDurableLog struct {
	Content           string `json:"content"`
	ContentType       string `json:"contentType"`
	TotalBytes        int    `json:"totalBytes"`
	Truncated         bool   `json:"truncated"`
	FilteredLineCount int    `json:"filteredLineCount"`
}

// TaskExecutionResult mirrors TaskExecutionResultResponse (presigned S3 URL
// for the task's cloudpickle result blob).
type TaskExecutionResult struct {
	URL                    string  `json:"url"`
	ExpiresIn              int     `json:"expiresIn"`
	ContentType            string  `json:"contentType"`
	Value                  any     `json:"value,omitempty"`
	ValueAvailable         bool    `json:"valueAvailable"`
	ValueUnavailableReason *string `json:"valueUnavailableReason,omitempty"`
}

func taskBase(pipelineID, runID string) (string, error) {
	return config.GetEndpointURL(
		"/api/v2/pipelines/" + pipelineID + "/dispatches/" + runID + "/tasks",
	)
}

// ListTaskExecutions returns all task execution records for a run.
func ListTaskExecutions(pipelineID, runID string) ([]TaskExecution, error) {
	endpoint, err := taskBase(pipelineID, runID)
	if err != nil {
		return nil, err
	}

	var tasks []TaskExecution

	err = drapi.GetJSON(endpoint, "task executions", &tasks)
	if err != nil {
		return nil, err
	}

	return tasks, nil
}

// GetTaskExecution fetches a single task execution by its sequential task ID.
func GetTaskExecution(pipelineID, runID string, taskID int) (*TaskExecution, error) {
	base, err := taskBase(pipelineID, runID)
	if err != nil {
		return nil, err
	}

	endpoint := base + "/" + strconv.Itoa(taskID)

	var task TaskExecution

	err = drapi.GetJSON(endpoint, "task execution", &task)
	if err != nil {
		return nil, err
	}

	return &task, nil
}

// GetTaskLogs reads live K8s pod logs for a task. tailLines limits the number
// of trailing lines returned (nil = no limit). verbosity is "user" or "all".
func GetTaskLogs(pipelineID, runID string, taskID int, tailLines *int, verbosity string) (*TaskExecutionLogs, error) {
	base, err := taskBase(pipelineID, runID)
	if err != nil {
		return nil, err
	}

	query := url.Values{}
	if tailLines != nil {
		query.Set("tail_lines", strconv.Itoa(*tailLines))
	}

	if verbosity != "" {
		query.Set("verbosity", verbosity)
	}

	endpoint := base + "/" + strconv.Itoa(taskID) + "/logs"

	if encoded := query.Encode(); encoded != "" {
		endpoint = endpoint + "?" + encoded
	}

	var logs TaskExecutionLogs

	err = drapi.GetJSON(endpoint, "task logs", &logs)
	if err != nil {
		return nil, err
	}

	return &logs, nil
}

// GetTaskDurableLog reads the S3-uploaded log for a task. stream must be
// "stdout" or "stderr". verbosity is "user" or "all".
func GetTaskDurableLog(pipelineID, runID string, taskID int, stream, verbosity string) (*TaskExecutionDurableLog, error) {
	base, err := taskBase(pipelineID, runID)
	if err != nil {
		return nil, err
	}

	query := url.Values{}
	if verbosity != "" {
		query.Set("verbosity", verbosity)
	}

	endpoint := base + "/" + strconv.Itoa(taskID) + "/logs/" + stream

	if encoded := query.Encode(); encoded != "" {
		endpoint = endpoint + "?" + encoded
	}

	var log TaskExecutionDurableLog

	err = drapi.GetJSON(endpoint, "task durable log", &log)
	if err != nil {
		return nil, err
	}

	return &log, nil
}

// GetTaskResult returns the presigned S3 URL for a completed task's result.
func GetTaskResult(pipelineID, runID string, taskID int) (*TaskExecutionResult, error) {
	base, err := taskBase(pipelineID, runID)
	if err != nil {
		return nil, err
	}

	endpoint := base + "/" + strconv.Itoa(taskID) + "/result"

	var result TaskExecutionResult

	err = drapi.GetJSON(endpoint, "task result", &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}
