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

package workload

import (
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
)

// Document is a server response kept as it arrived. The typed Workload and
// Artifact structs are deliberate projections: they parse the handful of
// fields the CLI renders and drop the rest. That is right for printing and
// wrong for copying, because a field this CLI does not know about is still a
// field the workload is running with.
//
// Setup uses these when it binds to a live workload: the manifest it writes
// has to be able to say everything the workload already says, including
// blocks no release of this CLI has heard of, or the first deploy from that
// file would quietly strip them.
type Document map[string]any

// GetWorkloadDocument fetches a workload as the server has it.
func GetWorkloadDocument(workloadID string) (Document, error) {
	url, err := config.GetEndpointURL("/api/v2/workloads/" + escapeID(workloadID) + "/")
	if err != nil {
		return nil, err
	}

	var doc Document

	if err := drapi.GetJSON(url, "workload", &doc); err != nil {
		return nil, err
	}

	return doc, nil
}

// GetArtifactDocument fetches an artifact as the server has it.
func GetArtifactDocument(artifactID string) (Document, error) {
	url, err := config.GetEndpointURL("/api/v2/artifacts/" + escapeID(artifactID) + "/")
	if err != nil {
		return nil, err
	}

	var doc Document

	if err := drapi.GetJSON(url, "artifact", &doc); err != nil {
		return nil, err
	}

	return doc, nil
}

// String returns the document's value for key, "" when absent or not a
// string.
func (d Document) String(key string) string {
	value, _ := d[key].(string)

	return value
}

// Map returns the document's nested object for key, nil when absent or not an
// object.
func (d Document) Map(key string) map[string]any {
	value, _ := d[key].(map[string]any)

	return value
}
