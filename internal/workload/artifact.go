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
	"net/url"
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
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`

	// Type is what kind of thing this is: a service, an agent. It sits beside
	// the spec rather than in it, because the platform reads the discriminator
	// from the artifact and pops any type sent inside the spec. An artifact
	// repository takes its own type from whichever artifact opened it, so this
	// is what says whether a later version can still join that lineage.
	Type string `json:"type"`

	// ArtifactRepositoryID is the lineage this artifact belongs to. Successive
	// versions of one thing share it, which is the only way to tell them from
	// unrelated artifacts that happen to carry the same name.
	ArtifactRepositoryID string `json:"artifactRepositoryId"`

	// Version numbers this artifact within its repository. A pointer because
	// the platform sends null for a draft and assigns a number only on
	// locking, and an unnumbered draft is a different thing from a version 0
	// that no repository ever issues.
	Version *int `json:"version"`

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
	// Name is the container's identifier within its group. Workload runtime
	// overrides (e.g. resourceAllocation) must reference the container by this
	// name, so it is parsed for callers that build a runtime spec.
	Name             string            `json:"name,omitempty"`
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
// "dr-credential" and carries DRCredentialID/Key instead, so the secret never
// appears in the spec. Source is omitempty because the server defaults plain
// vars to "string".
//
// Key selects which field of the credential to use, not to be confused with
// Name (the env var's own name): one stored credential can bundle several
// fields, as an S3 credential bundles awsAccessKeyId and awsSecretAccessKey.
type EnvironmentVar struct {
	Source         string `json:"source,omitempty"`
	Name           string `json:"name"`
	Value          string `json:"value,omitempty"`
	DRCredentialID string `json:"drCredentialId,omitempty"`
	Key            string `json:"key,omitempty"`
}

// EnvironmentVarSourceDRCredential marks a variable the platform resolves from
// the credential store at container start.
const EnvironmentVarSourceDRCredential = "dr-credential"

// defaultPrimaryContainerName is the name the CLI writes into the specs it
// generates, and the fallback for an unnamed primary container.
const defaultPrimaryContainerName = "primary"

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
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`

	// ArtifactRepositoryID and Version place the artifact in its lineage.
	// Neither is omitempty: the text and JSON forms of a command have to carry
	// the same fields, and a key that comes and goes with the artifact's status
	// is one a script cannot rely on. A draft's version is null rather than 0,
	// which says unnumbered instead of naming a version no repository issues.
	ArtifactRepositoryID string `json:"artifactRepositoryId"`
	Version              *int   `json:"version"`

	CatalogID string `json:"catalogId"`
	VersionID string `json:"versionId"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func NewArtifactOutput(a Artifact) ArtifactOutput {
	out := ArtifactOutput{
		ID:                   a.ID,
		Name:                 a.Name,
		Status:               a.Status,
		ArtifactRepositoryID: a.ArtifactRepositoryID,
		Version:              a.Version,
		CreatedAt:            a.CreatedAt.Format(time.RFC3339),
		UpdatedAt:            a.UpdatedAt.Format(time.RFC3339),
	}

	if codeRef := ExtractCodeRef(a); codeRef != nil {
		out.CatalogID = codeRef.CatalogID
		out.VersionID = codeRef.CatalogVersionID
	}

	return out
}

func (a *Artifact) IsLocked() bool {
	return IsLockedStatus(a.Status)
}

// IsLockedStatus is the same question asked of a status read off a raw
// document, for a caller that already has the artifact as the server sent it
// and should not fetch it again to get a typed struct to ask.
func IsLockedStatus(status string) bool {
	return strings.EqualFold(status, ArtifactStatusLocked)
}

// primaryContainer is the single definition of which container the CLI reads
// per-container fields from: the one flagged "primary": true, falling back to
// containerGroups[0].containers[0], nil when the artifact has neither. Every
// exported Primary* helper goes through it so they cannot disagree. Mirrors
// setPrimaryCodeRefInRawArtifact: once a primary is found it commits rather
// than falling through to a sidecar.
func primaryContainer(artifact Artifact) *Container {
	for i, group := range artifact.Spec.ContainerGroups {
		for j, container := range group.Containers {
			if container.Primary != nil && *container.Primary {
				return &artifact.Spec.ContainerGroups[i].Containers[j]
			}
		}
	}

	if len(artifact.Spec.ContainerGroups) == 0 || len(artifact.Spec.ContainerGroups[0].Containers) == 0 {
		return nil
	}

	return &artifact.Spec.ContainerGroups[0].Containers[0]
}

// ExtractCodeRef returns the primary container's codeRef, or nil when the
// primary has no codeRef (rather than falling through to a sidecar, which
// would surface stale catalog info in display).
func ExtractCodeRef(artifact Artifact) *DatarobotCodeRef {
	container := primaryContainer(artifact)
	if container == nil {
		return nil
	}

	return codeRefFromContainer(*container)
}

func codeRefFromContainer(container Container) *DatarobotCodeRef {
	if container.ImageBuildConfig == nil || container.ImageBuildConfig.CodeRef == nil {
		return nil
	}

	return container.ImageBuildConfig.CodeRef.Datarobot
}

// GetPrimaryContainerImageURI returns the imageUri of the primary container.
// Server-managed: the placeholder from create before a build, the produced
// image after one completes.
func GetPrimaryContainerImageURI(artifact Artifact) string {
	container := primaryContainer(artifact)
	if container == nil {
		return ""
	}

	return container.ImageURI
}

// PrimaryEnvironmentVars returns the environmentVars of the primary container,
// nil when the artifact has no containers.
func PrimaryEnvironmentVars(artifact Artifact) []EnvironmentVar {
	container := primaryContainer(artifact)
	if container == nil {
		return nil
	}

	return container.EnvironmentVars
}

// PrimaryContainerName returns the name of the artifact's primary container.
// Callers building a workload runtime spec need it because resourceAllocation
// overrides are keyed by container name and must match a container in the
// artifact. An unnamed primary falls back to "primary", which is the CLI's own
// convention rather than a guarantee: a hand-written artifact that leaves its
// primary blank will not match, and the server decides what to do with that.
func PrimaryContainerName(artifact Artifact) string {
	container := primaryContainer(artifact)
	if container == nil || container.Name == "" {
		return defaultPrimaryContainerName
	}

	return container.Name
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

// CloneArtifact copies an artifact into a new draft named name.
func CloneArtifact(artifactID, name string) (*Artifact, error) {
	url, err := config.GetEndpointURL("/api/v2/artifacts/" + escapeID(artifactID) + "/clone")
	if err != nil {
		return nil, err
	}

	body := map[string]string{"name": name}

	var artifact Artifact

	if err := drapi.PostJSON(url, "artifact clone", body, &artifact); err != nil {
		return nil, err
	}

	return &artifact, nil
}

// UpdateArtifactSpec writes a draft artifact's spec.
//
// containerGroups is replaced wholesale rather than merged key by key. The
// platform re-injects what it owns on a build-from-source container, so a spec
// that omits imageUri keeps the running image; codeRef is not re-injected,
// which is why the copy path puts it back before writing.
func UpdateArtifactSpec(artifactID string, spec json.RawMessage) error {
	if len(spec) == 0 {
		return errors.New("no artifact spec to update")
	}

	url, err := config.GetEndpointURL("/api/v2/artifacts/" + escapeID(artifactID) + "/")
	if err != nil {
		return err
	}

	body := map[string]json.RawMessage{"spec": spec}

	return drapi.PatchJSON(url, "artifact", body, nil)
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

// SpecWithPrimaryCodeRef returns spec with the primary container pointed at the
// catalog pair given. An update replaces a container wholesale, so the file's
// spec alone would erase the reference.
func SpecWithPrimaryCodeRef(spec json.RawMessage, catalogID, catalogVersionID string) (json.RawMessage, error) {
	var decoded map[string]any

	if err := json.Unmarshal(spec, &decoded); err != nil {
		return nil, fmt.Errorf("cannot read the artifact spec: %w", err)
	}

	// The map is shared rather than copied, so its edits are marshalled back.
	if err := setPrimaryCodeRefInRawArtifact(map[string]any{"spec": decoded}, catalogID, catalogVersionID); err != nil {
		return nil, err
	}

	updated, err := json.Marshal(decoded)
	if err != nil {
		return nil, fmt.Errorf("cannot convert the artifact spec to JSON: %w", err)
	}

	return updated, nil
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

	container := primaryContainerInGroups(groups)
	if container == nil {
		return noPrimaryContainer(groups)
	}

	setImageBuildConfigCodeRef(container, map[string]any{
		keyRawDatarobot: map[string]any{
			keyRawCatalogID:        catalogID,
			keyRawCatalogVersionID: catalogVersionID,
		},
	})

	return nil
}

// noPrimaryContainer says why the search came back empty. Asked only after it
// has failed: checking groups[0] first would refuse an artifact whose flagged
// primary sits in a later group.
func noPrimaryContainer(groups []any) error {
	group, ok := groups[0].(map[string]any)
	if !ok {
		return errors.New("artifact: spec.containerGroups[0] missing or wrong type")
	}

	if len(containersOfGroup(group)) == 0 {
		return errors.New("artifact: spec.containerGroups[0].containers missing or empty")
	}

	return errors.New("artifact: spec.containerGroups[0].containers[0] missing or wrong type")
}

// Keys along the code reference inside a decoded container, spelled once so
// the reader and the writer below cannot drift apart.
const (
	keyRawImageBuildConfig = "imageBuildConfig"
	keyRawCodeRef          = "codeRef"
	keyRawDatarobot        = "datarobot"
	keyRawCatalogID        = "catalogId"
	keyRawCatalogVersionID = "catalogVersionId"
)

// CodeRefInContainer reads the catalog pair a decoded container points at, nil
// when it names none. The read half of setPrimaryCodeRefInRawArtifact.
func CodeRefInContainer(container map[string]any) *DatarobotCodeRef {
	build, _ := container[keyRawImageBuildConfig].(map[string]any)
	ref, _ := build[keyRawCodeRef].(map[string]any)

	datarobot, ok := ref[keyRawDatarobot].(map[string]any)
	if !ok {
		return nil
	}

	catalogID, _ := datarobot[keyRawCatalogID].(string)
	catalogVersionID, _ := datarobot[keyRawCatalogVersionID].(string)

	return &DatarobotCodeRef{CatalogID: catalogID, CatalogVersionID: catalogVersionID}
}

// PrimaryContainerInDocument is primaryContainer for an artifact still in the
// shape the server sent it. Two readings of which container is primary
// disagreeing is how a plan promises an image the run cannot find.
func PrimaryContainerInDocument(raw map[string]any) map[string]any {
	spec, ok := raw["spec"].(map[string]any)
	if !ok {
		return nil
	}

	groups, ok := spec["containerGroups"].([]any)
	if !ok {
		return nil
	}

	return primaryContainerInGroups(groups)
}

// primaryContainerInGroups is primaryContainer's rule for a decoded document:
// the container flagged "primary", else the first.
func primaryContainerInGroups(groups []any) map[string]any {
	if flagged := flaggedPrimaryContainer(groups); flagged != nil {
		return flagged
	}

	return firstContainerInGroups(groups)
}

// flaggedPrimaryContainer is the container that claims to be primary, if any.
func flaggedPrimaryContainer(groups []any) map[string]any {
	for _, g := range groups {
		group, ok := g.(map[string]any)
		if !ok {
			continue
		}

		for _, c := range containersOfGroup(group) {
			if container, ok := c.(map[string]any); ok && isPrimaryContainer(container) {
				return container
			}
		}
	}

	return nil
}

func firstContainerInGroups(groups []any) map[string]any {
	if len(groups) == 0 {
		return nil
	}

	group, ok := groups[0].(map[string]any)
	if !ok {
		return nil
	}

	containers := containersOfGroup(group)
	if len(containers) == 0 {
		return nil
	}

	first, _ := containers[0].(map[string]any)

	return first
}

func containersOfGroup(group map[string]any) []any {
	containers, _ := group["containers"].([]any)

	return containers
}

// setImageBuildConfigCodeRef preserves any existing dockerfile config and
// seeds a "provided" Dockerfile default when imageBuildConfig is absent
// (server requires a dockerfile on the imageBuildConfig).
func setImageBuildConfigCodeRef(container map[string]any, codeRef map[string]any) {
	ibc, ok := container[keyRawImageBuildConfig].(map[string]any)
	if !ok || ibc == nil {
		ibc = map[string]any{
			"dockerfile": map[string]any{
				"source": "provided",
			},
		}
	}

	ibc[keyRawCodeRef] = codeRef
	container[keyRawImageBuildConfig] = ibc
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

// ListArtifacts fetches up to limit artifacts starting at offset, optionally
// filtered by status. It validates its arguments and clamps the page size to
// the server-enforced ceiling, mirroring ListWorkloads.
func ListArtifacts(limit, offset int, status Status) ([]Artifact, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("invalid limit %d: must be positive", limit)
	}

	if offset < 0 {
		return nil, fmt.Errorf("invalid offset %d: must be non-negative", offset)
	}

	query := url.Values{}
	query.Set("limit", strconv.Itoa(min(limit, maxPageSize)))
	query.Set("offset", strconv.Itoa(offset))

	if status != "" {
		query.Set("status", string(status))
	}

	pageURL, err := drapi.EndpointURL("/artifacts/", query)
	if err != nil {
		return nil, err
	}

	return paginateArtifacts(pageURL, limit)
}

// paginateArtifacts follows next-links from pageURL, accumulating artifacts
// until limit is reached or the pages run out. Split from ListArtifacts to
// keep that function's cyclomatic complexity under the linter threshold.
func paginateArtifacts(pageURL string, limit int) ([]Artifact, error) {
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
