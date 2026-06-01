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

// version.go wraps the version-scoped read endpoints exposed by
// pipelines-api/.../controllers/pipeline.py:
//
//   GET /pipelines/{id}/versions
//   GET /pipelines/{id}/versions/{ver}
//   GET /pipelines/{id}/graph                     (draft DAG)
//   GET /pipelines/{id}/versions/{ver}/graph      (locked DAG)
//
// The list/detail endpoints return PipelineVersion records (already
// defined in pipeline.go). The two graph endpoints return a free-form
// dict; we surface its known shape via the Graph struct below.

package pipeline

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/datarobot/cli/internal/config"
)

// GraphNode mirrors PipelineGraphNode from the graph endpoint. IDs are
// integer indices assigned by Covalent's TransportGraph.
type GraphNode struct {
	ID             int     `json:"id"`
	Type           string  `json:"type"`
	Name           string  `json:"name"`
	Source         *string `json:"source,omitempty"`
	ResourceBundle any     `json:"resourceBundle,omitempty"`
	TaskGroupID    any     `json:"taskGroupId,omitempty"`
}

// GraphEdge mirrors PipelineGraphEdge. Source and Target are integer node IDs.
// EdgeName, ParamType, and ArgIndex carry Covalent transport metadata.
type GraphEdge struct {
	Source    int    `json:"source"`
	Target    int    `json:"target"`
	EdgeName  string `json:"edgeName,omitempty"`
	ParamType string `json:"paramType,omitempty"`
	ArgIndex  int    `json:"argIndex,omitempty"`
}

// GraphPipeline mirrors PipelineGraphMeta — the pipeline header in the
// graph payload (action 059: wire key renamed from "lattice" to "pipeline").
type GraphPipeline struct {
	Name          string `json:"name"`
	PythonVersion string `json:"pythonVersion"`
}

// Graph mirrors the JSON returned by GET /pipelines/{id}/graph and
// GET /pipelines/{id}/versions/{ver}/graph.
type Graph struct {
	Pipeline GraphPipeline `json:"pipeline"`
	Nodes    []GraphNode   `json:"nodes"`
	Edges    []GraphEdge   `json:"edges"`
}

// ListVersions fetches a paginated list of versions for a pipeline.
func ListVersions(pipelineID string, offset, limit int) ([]PipelineVersion, error) {
	endpoint, err := config.GetEndpointURL("/api/v2/pipelines/" + pipelineID + "/versions")
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

	var page DataPage[PipelineVersion]

	err = doJSON(http.MethodGet, endpoint, nil, "pipeline versions", &page)
	if err != nil {
		return nil, err
	}

	return page.Data, nil
}

// GetVersion fetches a single version of a pipeline.
func GetVersion(pipelineID string, versionID int) (*PipelineVersion, error) {
	endpoint, err := config.GetEndpointURL("/api/v2/pipelines/" + pipelineID + "/versions/" + strconv.Itoa(versionID))
	if err != nil {
		return nil, err
	}

	var version PipelineVersion

	err = doJSON(http.MethodGet, endpoint, nil, "pipeline version", &version)
	if err != nil {
		return nil, err
	}

	return &version, nil
}

// GetGraph fetches the DAG visualization payload for a pipeline. When
// scope is ScopeDraft the latest draft graph is returned; with
// ScopeLocked + version, the graph for that locked version is returned.
func GetGraph(pipelineID string, scope Scope, version *int) (*Graph, error) {
	endpoint, err := EndpointFor(pipelineID, scope, version, "graph")
	if err != nil {
		return nil, err
	}

	var graph Graph

	err = doJSON(http.MethodGet, endpoint, nil, "graph", &graph)
	if err != nil {
		return nil, err
	}

	return &graph, nil
}
