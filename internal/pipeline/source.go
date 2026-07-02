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

// source.go wraps the pipeline source endpoints:
//
//	GET /api/v2/pipelines/{id}/source
//	GET /api/v2/pipelines/{id}/versions/{v}/source
package pipeline

import "net/http"

// PipelineSourceResponse mirrors PipelineSourceResponse from the pipelines-api.
type PipelineSourceResponse struct {
	Source string `json:"source"`
}

// GetPipelineSource fetches the full source.py content of a pipeline.
// For draft scope it calls GET /pipelines/{id}/source; for locked scope
// it calls GET /pipelines/{id}/versions/{v}/source.
func GetPipelineSource(pipelineID string, scope Scope, version *int) (*PipelineSourceResponse, error) {
	endpoint, err := EndpointFor(pipelineID, scope, version, "source")
	if err != nil {
		return nil, err
	}

	var result PipelineSourceResponse

	err = doJSON(http.MethodGet, endpoint, nil, "pipeline source", &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}
