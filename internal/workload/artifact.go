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
	ImageBuildConfig *ImageBuildConfig `json:"imageBuildConfig,omitempty"`
	// ImageURI is the resolved container image reference. Server-managed:
	// before a successful build this is the placeholder from create; after
	// build COMPLETED it points at the produced image.
	ImageURI string `json:"imageUri,omitempty"`
	// *bool so an absent value round-trips as nil and omitempty drops it
	// on marshal, instead of re-asserting `false` back to the server.
	Primary         *bool            `json:"primary,omitempty"`
	EnvironmentVars []EnvironmentVar `json:"environmentVars,omitempty"`
}

// EnvironmentVar is one entry in a container's environmentVars array. A plain
// var carries Value directly; a credential-backed var has Source ==
// "dr-credential" and carries DRCredentialID/Key instead, letting the
// platform inject the secret without it ever appearing in the artifact spec.
// Source is omitempty on write: the server defaults plain vars to "string"
// when it is absent, so callers only need to set it for the credential case.
//
// Key selects which field of the DRCredentialID credential to use -- not to
// be confused with Name (the env var's own name). A single stored credential
// can bundle several secret fields (an S3 credential has awsAccessKeyId,
// awsSecretAccessKey, and awsSessionToken, for instance); Key picks one.
type EnvironmentVar struct {
	Source         string `json:"source,omitempty"`
	Name           string `json:"name"`
	Value          string `json:"value,omitempty"`
	DRCredentialID string `json:"drCredentialId,omitempty"`
	Key            string `json:"key,omitempty"`
}

const EnvironmentVarSourceDRCredential = "dr-credential"

type ImageBuildConfig struct {
	CodeRef    *CodeRef    `json:"codeRef,omitempty"`
	Dockerfile *Dockerfile `json:"dockerfile,omitempty"`
}

// Dockerfile flattens workload-api's ProvidedDockerfile / GeneratedDockerfile
// union. Source is the discriminator; the rest are only valid when
// source == "generated".
type Dockerfile struct {
	Source                        string   `json:"source"`
	ExecutionEnvironmentID        string   `json:"executionEnvironmentId,omitempty"`
	ExecutionEnvironmentVersionID string   `json:"executionEnvironmentVersionId,omitempty"`
	Entrypoint                    []string `json:"entrypoint,omitempty"`
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

func (a *Artifact) IsLocked() bool {
	return strings.EqualFold(a.Status, ArtifactStatusLocked)
}

// ExtractCodeRef mirrors the write-side selection in setPrimaryCodeRefInRawArtifact:
// once a primary container is found it commits, returning nil if the primary
// has no codeRef rather than falling through to a sidecar (which would surface
// stale catalog info in display). Falls back to containerGroups[0].containers[0]
// when no container is flagged primary.
func ExtractCodeRef(artifact Artifact) *DatarobotCodeRef {
	for _, group := range artifact.Spec.ContainerGroups {
		for _, container := range group.Containers {
			if container.Primary == nil || !*container.Primary {
				continue
			}

			return codeRefFromContainer(container)
		}
	}

	if len(artifact.Spec.ContainerGroups) == 0 {
		return nil
	}

	if len(artifact.Spec.ContainerGroups[0].Containers) == 0 {
		return nil
	}

	return codeRefFromContainer(artifact.Spec.ContainerGroups[0].Containers[0])
}

func codeRefFromContainer(container Container) *DatarobotCodeRef {
	if container.ImageBuildConfig == nil || container.ImageBuildConfig.CodeRef == nil {
		return nil
	}

	return container.ImageBuildConfig.CodeRef.Datarobot
}

// GetPrimaryContainerImageURI returns the imageUri of the primary container
// (the one flagged "primary": true), falling back to
// containerGroups[0].containers[0] when no primary is marked. Mirrors
// ExtractCodeRef's selection semantics so reads and writes target the same
// container after a build updates the spec server-side.
func GetPrimaryContainerImageURI(artifact Artifact) string {
	for _, group := range artifact.Spec.ContainerGroups {
		for _, container := range group.Containers {
			if container.Primary == nil || !*container.Primary {
				continue
			}

			return container.ImageURI
		}
	}

	if len(artifact.Spec.ContainerGroups) == 0 {
		return ""
	}

	if len(artifact.Spec.ContainerGroups[0].Containers) == 0 {
		return ""
	}

	return artifact.Spec.ContainerGroups[0].Containers[0].ImageURI
}

// PrimaryEnvironmentVars returns the environmentVars of the primary container
// (the one flagged "primary": true), falling back to
// containerGroups[0].containers[0] when no primary is marked. Mirrors
// GetPrimaryContainerImageURI's selection semantics so every per-container
// field this CLI reads picks the same container.
func PrimaryEnvironmentVars(artifact Artifact) []EnvironmentVar {
	for _, group := range artifact.Spec.ContainerGroups {
		for _, container := range group.Containers {
			if container.Primary == nil || !*container.Primary {
				continue
			}

			return container.EnvironmentVars
		}
	}

	if len(artifact.Spec.ContainerGroups) == 0 {
		return nil
	}

	if len(artifact.Spec.ContainerGroups[0].Containers) == 0 {
		return nil
	}

	return artifact.Spec.ContainerGroups[0].Containers[0].EnvironmentVars
}

func GetArtifact(artifactID string) (*Artifact, error) {
	url, err := config.GetEndpointURL("/api/v2/artifacts/" + escapeID(artifactID) + "/")
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
	ImageURI         string            `json:"imageUri,omitempty"`
	Port             int               `json:"port,omitempty"`
	Primary          *bool             `json:"primary,omitempty"`
	ImageBuildConfig *ImageBuildConfig `json:"imageBuildConfig,omitempty"`
}

// ValidateCreateRequest checks the structural invariants of a user-supplied
// spec (required name, non-empty containerGroups, every group has containers)
// and lets the server validate field-level shape. We don't reject unknown
// fields here because the workload-api schema moves faster than this struct;
// the server's 422 carries a JSON-path detail that's clearer than what
// DisallowUnknownFields would produce. The original bytes are sent verbatim.
func ValidateCreateRequest(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))

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

func PatchArtifactCodeRef(artifactID, catalogID, catalogVersionID string) error {
	url, err := config.GetEndpointURL("/api/v2/artifacts/" + escapeID(artifactID) + "/")
	if err != nil {
		return err
	}

	var raw map[string]any

	if err := drapi.GetJSON(url, "artifact", &raw); err != nil {
		return fmt.Errorf("fetch artifact for codeRef update: %w", err)
	}

	if err := setPrimaryCodeRefInRawArtifact(raw, catalogID, catalogVersionID); err != nil {
		return err
	}

	body := map[string]any{"spec": raw["spec"]}

	return drapi.PatchJSON(url, "artifact", body, nil)
}

func setPrimaryCodeRefInRawArtifact(raw map[string]any, catalogID, catalogVersionID string) error {
	spec, ok := raw["spec"].(map[string]any)
	if !ok {
		return errors.New("artifact: spec missing or wrong type")
	}

	groups, ok := spec["containerGroups"].([]any)
	if !ok || len(groups) == 0 {
		return errors.New("artifact: spec.containerGroups missing or empty")
	}

	codeRef := map[string]any{
		"datarobot": map[string]any{
			"catalogId":        catalogID,
			"catalogVersionId": catalogVersionID,
		},
	}

	if found := assignToPrimaryContainer(groups, codeRef); found {
		return nil
	}

	// Mirror ExtractCodeRef's [0][0] fallback when no container is flagged primary.
	return assignToFirstContainer(groups, codeRef)
}

func assignToPrimaryContainer(groups []any, codeRef map[string]any) bool {
	for _, g := range groups {
		group, ok := g.(map[string]any)
		if !ok {
			continue
		}

		containers, ok := group["containers"].([]any)
		if !ok {
			continue
		}

		for _, c := range containers {
			container, ok := c.(map[string]any)
			if !ok {
				continue
			}

			if isPrimaryContainer(container) {
				setImageBuildConfigCodeRef(container, codeRef)

				return true
			}
		}
	}

	return false
}

func assignToFirstContainer(groups []any, codeRef map[string]any) error {
	firstGroup, ok := groups[0].(map[string]any)
	if !ok {
		return errors.New("artifact: spec.containerGroups[0] missing or wrong type")
	}

	containers, ok := firstGroup["containers"].([]any)
	if !ok || len(containers) == 0 {
		return errors.New("artifact: spec.containerGroups[0].containers missing or empty")
	}

	firstContainer, ok := containers[0].(map[string]any)
	if !ok {
		return errors.New("artifact: spec.containerGroups[0].containers[0] missing or wrong type")
	}

	setImageBuildConfigCodeRef(firstContainer, codeRef)

	return nil
}

// setImageBuildConfigCodeRef preserves any existing dockerfile config and
// seeds a "provided" Dockerfile default when imageBuildConfig is absent
// (server requires a dockerfile on the imageBuildConfig).
func setImageBuildConfigCodeRef(container map[string]any, codeRef map[string]any) {
	ibc, ok := container["imageBuildConfig"].(map[string]any)
	if !ok || ibc == nil {
		ibc = map[string]any{
			"dockerfile": map[string]any{
				"source": "provided",
			},
		}
	}

	ibc["codeRef"] = codeRef
	container["imageBuildConfig"] = ibc
}

func isPrimaryContainer(container map[string]any) bool {
	primary, ok := container["primary"].(bool)

	return ok && primary
}

// LockArtifact promotes a draft artifact to locked via PATCH {"status": "locked"}
// and returns the updated artifact (status locked, version assigned). Locking is
// one-way. The server replies 403 when the artifact is already locked, 404 when
// it does not exist, and 422 when a source-built container is incomplete (missing
// codeRef or unbuilt imageUri); the error detail names the missing piece.
func LockArtifact(artifactID string) (*Artifact, error) {
	url, err := config.GetEndpointURL("/api/v2/artifacts/" + escapeID(artifactID) + "/")
	if err != nil {
		return nil, err
	}

	body := map[string]string{"status": ArtifactStatusLocked}

	var artifact Artifact

	err = drapi.PatchJSON(url, "artifact", body, &artifact)
	if err != nil {
		return nil, err
	}

	return &artifact, nil
}

// DeleteArtifact deletes a draft artifact. The server replies 409 when the
// artifact is locked (locking is one-way; locked artifacts can never be
// deleted) or still referenced by a workload's proton(s); the 409 detail
// names the blocking workload IDs.
func DeleteArtifact(artifactID string) error {
	url, err := config.GetEndpointURL("/api/v2/artifacts/" + escapeID(artifactID) + "/")
	if err != nil {
		return err
	}

	return drapi.DeleteJSON(url, "artifact", nil, nil)
}

func ListArtifacts(limit int, status Status) ([]Artifact, error) {
	endpoint := "/api/v2/artifacts/?limit=" + strconv.Itoa(limit)

	if status != "" {
		endpoint += "&status=" + string(status)
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

		if list.Next == "" {
			break
		}

		if err := drapi.AssertNextOnSameHost(list.Next); err != nil {
			return nil, err
		}

		pageURL = list.Next
	}

	return all, nil
}
