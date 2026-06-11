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

// task.go wraps the pipeline task-detail endpoints added in pipelines-api
// CMPT-6040:
//
//	GET /api/v2/pipelines/{pipeline_id}/tasks/{task_id}
//	GET /api/v2/pipelines/{pipeline_id}/versions/{version_id}/tasks/{task_id}
//
// These endpoints return the per-task source code, function signature
// parameters, and (for locked versions) the latest VALID pipeline input
// payload. The same draft/locked URL split is used as for inputs and runs.
package pipeline

import "net/http"

// TaskParameter mirrors TaskParameter from the pipelines-api schema.
// It holds a single parameter from a @task function signature.
type TaskParameter struct {
	Name       string  `json:"name"`
	Annotation *string `json:"annotation,omitempty"`
}

// PipelineTask mirrors TaskResponse from the pipelines-api.
// The wire-level "id" is stored as TaskID to match the CLI vocabulary used
// across other pipeline resource types (InputID, RunID, etc.).
// ResourceBundle and TaskGroupID are null until the executor AST-extraction
// and task-grouping features land (pipelines-api CMPT-6040).
type PipelineTask struct {
	TaskID         string          `json:"id"`
	PipelineID     string          `json:"pipelineId"`
	VersionID      *int            `json:"versionId,omitempty"`
	Name           string          `json:"name"`
	Parameters     []TaskParameter `json:"parameters"`
	Inputs         map[string]any  `json:"inputs,omitempty"`
	Source         string          `json:"source"`
	ResourceBundle map[string]any  `json:"resourceBundle,omitempty"`
	TaskGroupID    *int            `json:"taskGroupId,omitempty"`
}

// GetTask fetches per-task detail from the pipelines-api. For draft scope
// the response always has Inputs=nil; for locked scope Inputs is the latest
// VALID pipeline input payload or nil if none exists.
func GetTask(pipelineID string, scope Scope, version *int, taskID string) (*PipelineTask, error) {
	endpoint, err := EndpointFor(pipelineID, scope, version, "tasks/"+taskID)
	if err != nil {
		return nil, err
	}

	var task PipelineTask

	err = doJSON(http.MethodGet, endpoint, nil, "task", &task)
	if err != nil {
		return nil, err
	}

	return &task, nil
}
