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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/datarobot/cli/internal/config"
)

type Artifact struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Spec      Spec      `json:"spec"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Spec struct {
	ContainerGroups []ContainerGroup `json:"containerGroups"`
}

type ContainerGroup struct {
	Containers []Container `json:"containers"`
}

type Container struct {
	CodeRef *CodeRef `json:"codeRef"`
}

type CodeRef struct {
	Datarobot *DatarobotCodeRef `json:"datarobot"`
}

type DatarobotCodeRef struct {
	CatalogID        string `json:"catalogId"`
	CatalogVersionID string `json:"catalogVersionId"`
}

func ExtractCodeRef(artifact Artifact) *DatarobotCodeRef {
	if len(artifact.Spec.ContainerGroups) == 0 {
		return nil
	}

	if len(artifact.Spec.ContainerGroups[0].Containers) == 0 {
		return nil
	}

	codeRef := artifact.Spec.ContainerGroups[0].Containers[0].CodeRef
	if codeRef == nil {
		return nil
	}

	return codeRef.Datarobot
}

func GetArtifact(ctx context.Context, baseURL, token, artifactID string) (*Artifact, error) {
	url := baseURL + "/api/v2/artifacts/" + artifactID + "/"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to reach DataRobot: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", config.GetUserAgentHeader())

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach DataRobot: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("artifact %s not found", artifactID)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, errors.New("Authentication failed. Check DATAROBOT_ENDPOINT and DATAROBOT_API_TOKEN.")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected response: %d", resp.StatusCode)
	}

	var artifact Artifact

	if err := json.NewDecoder(resp.Body).Decode(&artifact); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &artifact, nil
}
