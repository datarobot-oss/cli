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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/drapi"
)

const (
	ArtifactStatusDraft  = "draft"
	ArtifactStatusLocked = "locked"
)

func ParseArtifactStatus(s string) (string, error) {
	if s == "" {
		return "", nil
	}

	lower := strings.ToLower(s)

	if lower != ArtifactStatusDraft && lower != ArtifactStatusLocked {
		return "", fmt.Errorf("invalid status %q: use %s or %s", s, ArtifactStatusDraft, ArtifactStatusLocked)
	}

	return lower, nil
}

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

type ArtifactOutput struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	CatalogID string `json:"catalogId"`
	VersionID string `json:"versionId"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func NewArtifactOutput(a Artifact) ArtifactOutput {
	out := ArtifactOutput{
		ID:        a.ID,
		Name:      a.Name,
		Status:    a.Status,
		CreatedAt: a.CreatedAt.Format(time.RFC3339),
		UpdatedAt: a.UpdatedAt.Format(time.RFC3339),
	}

	if codeRef := ExtractCodeRef(a); codeRef != nil {
		out.CatalogID = codeRef.CatalogID
		out.VersionID = codeRef.CatalogVersionID
	}

	return out
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

func GetArtifact(artifactID string) (*Artifact, error) {
	url, err := config.GetEndpointURL("/api/v2/artifacts/" + artifactID + "/")
	if err != nil {
		return nil, err
	}

	var artifact Artifact

	err = drapi.GetJSON(url, "artifact", &artifact)
	if err != nil {
		return nil, err
	}

	return &artifact, nil
}

type ArtifactList struct {
	Data       []Artifact `json:"data"`
	Count      int        `json:"count"`
	TotalCount int        `json:"totalCount"`
	Next       string     `json:"next"`
	Previous   string     `json:"previous"`
}

type ArtifactCreateRequest struct {
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	Spec        ArtifactCreateSpec `json:"spec"`
}

type ArtifactCreateSpec struct {
	ContainerGroups []ArtifactCreateContainerGroup `json:"containerGroups"`
}

type ArtifactCreateContainerGroup struct {
	Containers []ArtifactCreateContainer `json:"containers"`
}

type ArtifactCreateContainer struct {
	ImageURI        string                         `json:"imageUri,omitempty"`
	Port            int                            `json:"port,omitempty"`
	ResourceRequest *ArtifactCreateResourceRequest `json:"resourceRequest,omitempty"`
	CodeRef         *CodeRef                       `json:"codeRef,omitempty"`
}

type ArtifactCreateResourceRequest struct {
	CPU    int   `json:"cpu"`
	Memory int64 `json:"memory"`
}

// ValidateCreateRequest decodes a user-supplied spec file with DisallowUnknownFields
// against ArtifactCreateRequest and enforces required-field invariants. The original
// bytes are still sent verbatim by the caller; the strict struct never reaches the wire.
func ValidateCreateRequest(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var req ArtifactCreateRequest

	if err := dec.Decode(&req); err != nil {
		return fmt.Errorf("invalid spec: %w", err)
	}

	if req.Name == "" {
		return errors.New("invalid spec: required field 'name' is missing or empty")
	}

	if len(req.Spec.ContainerGroups) == 0 {
		return errors.New("invalid spec: 'spec.containerGroups' must contain at least one entry")
	}

	for i, group := range req.Spec.ContainerGroups {
		if len(group.Containers) == 0 {
			return fmt.Errorf("invalid spec: 'spec.containerGroups[%d].containers' must contain at least one entry", i)
		}
	}

	return nil
}

// CreateArtifact POSTs payload to /api/v2/artifacts/ and returns the parsed artifact.
// payload is typically a json.RawMessage from the spec file, sent verbatim after
// ValidateCreateRequest passed.
func CreateArtifact(payload any) (*Artifact, error) {
	url, err := config.GetEndpointURL("/api/v2/artifacts/")
	if err != nil {
		return nil, err
	}

	var artifact Artifact

	err = drapi.PostJSON(url, "artifact", payload, &artifact)
	if err != nil {
		return nil, err
	}

	return &artifact, nil
}

func ListArtifacts(limit int, status string) ([]Artifact, error) {
	endpoint := "/api/v2/artifacts/?limit=" + strconv.Itoa(limit)

	if status != "" {
		endpoint += "&status=" + status
	}

	pageURL, err := config.GetEndpointURL(endpoint)
	if err != nil {
		return nil, err
	}

	var all []Artifact

	for pageURL != "" {
		var list ArtifactList

		if err := drapi.GetJSON(pageURL, "artifacts", &list); err != nil {
			return nil, err
		}

		all = append(all, list.Data...)

		if len(all) >= limit {
			return all[:limit], nil
		}

		pageURL = list.Next
	}

	return all, nil
}
