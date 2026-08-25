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

package up

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
)

// Test seams for the two reads, replaced by this package's tests to keep the
// plan off the network.
var (
	getWorkloadDocFn = workload.GetWorkloadDocument
	getArtifactDocFn = workload.GetArtifactDocument
)

// Keys read off the raw documents. They are read before manifest.NewLive
// cleans the spec and runtime blocks, because the cleaning strips exactly
// these names as server bookkeeping.
const (
	keyStatus     = "status"
	keyEndpoint   = "endpoint"
	keyArtifactID = "artifactId"
	keySpec       = "spec"
	keyType       = "type"

	// keyArtifactRepositoryID is the repository an artifact belongs to. The
	// platform assigns it, so it is read here and never written to the file;
	// a create that sends it back is what makes successive versions land in
	// one repository instead of each opening its own.
	keyArtifactRepositoryID = "artifactRepositoryId"
)

// keyArtifactType is what the plan calls a change of artifact kind. It is
// spelled for a reader of the plan rather than as the API's own field name,
// which is bare "type" on the artifact and would read as nothing in
// particular on a line of its own.
const keyArtifactType = "artifact.type"

// Live is the workload as the platform currently has it: the two cleaned
// documents that drift is measured against, plus the few scalars the command
// branches on or prints.
//
// The embedded manifest.Live is what the setup wizard already uses to bind to
// a running workload, and it is the right shape here for the same reason. Its
// Spec and Runtime are the server's own documents with the server's own
// bookkeeping removed, so a block no release of this CLI has heard of still
// takes part in the comparison instead of being silently dropped.
type Live struct {
	manifest.Live

	// State is what the workload is doing, reduced to the branches that
	// change the deploy.
	State State

	// Status is the platform's own word. Three statuses reduce to
	// StateStopped and only some of them can be started.
	Status string

	// ArtifactID is the artifact currently running, "" when there is none.
	ArtifactID string

	// ArtifactType is what kind of thing that artifact is: a service, an
	// agent. It sits beside the spec rather than in it, because the platform
	// reads the discriminator from the artifact and pops any type sent inside
	// the spec, so the spec walk never sees it.
	ArtifactType string

	// ArtifactRepositoryID is the lineage the running version belongs to, ""
	// when there is no artifact to read it from. A roll hands it to the
	// version it mints, which is what makes the two successive versions of one
	// thing rather than two unrelated artifacts sharing a name.
	ArtifactRepositoryID string

	// Endpoint is the workload's stable URL. The platform assigns it at
	// creation and it survives every rollout, so it is safe to print before
	// the workload is actually serving.
	Endpoint string

	// Locked reports whether the running artifact is immutable. A locked
	// artifact means production, and its successor has to be locked too
	// before the platform will accept a replacement.
	Locked bool
}

// liveArtifactType reads the discriminator off the artifact document, falling
// back to the spec.
//
// The platform hoists a type sent inside the spec and answers with it beside
// the spec, which is what the fallback is for: the file's side is read from
// both placements, and a live side that read only one would leave the two
// halves of the same comparison enforcing different invariants. If a route
// ever answered without hoisting, an agent would read as having no type, be
// defaulted to a service, and drift against a file correctly calling it an
// agent on every run.
func liveArtifactType(artifactDoc workload.Document) string {
	if beside := artifactDoc.String(keyType); beside != "" {
		return beside
	}

	spec, _ := artifactDoc[keySpec].(map[string]any)
	within, _ := spec[keyType].(string)

	return within
}

// Look fetches the live workload and the artifact it runs.
//
// An empty workloadID is not an error: it is a manifest that has never been
// deployed, which reports StateUnbound and nothing else. A workload the
// platform does not have is not an error either, only StateMissing. Both are
// ordinary situations with different right answers, and choosing between them
// belongs to the apply step, which is the part that knows whether it is
// allowed to create something.
func Look(workloadID string) (Live, error) {
	if workloadID == "" {
		return Live{State: StateUnbound}, nil
	}

	workloadDoc, err := getWorkloadDocFn(workloadID)
	if err != nil {
		if isNotFound(err) {
			return Live{State: StateMissing}, nil
		}

		return Live{}, fmt.Errorf("cannot read workload %s: %w", workloadID, err)
	}

	artifactID := workloadDoc.String(keyArtifactID)

	artifactDoc, err := artifactFor(artifactID)
	if err != nil {
		return Live{}, err
	}

	status := workloadDoc.String(keyStatus)

	return Live{
		Live:                 manifest.NewLive(workloadID, workloadDoc, artifactDoc),
		State:                stateFor(status),
		Status:               status,
		ArtifactID:           artifactID,
		ArtifactType:         liveArtifactType(artifactDoc),
		ArtifactRepositoryID: artifactDoc.String(keyArtifactRepositoryID),
		Endpoint:             workloadDoc.String(keyEndpoint),
		Locked:               isLocked(artifactDoc.String(keyStatus)),
	}, nil
}

// artifactFor reads the artifact a workload runs. A workload without one is
// possible only in the moments around creation, and it is not a failure: the
// spec simply compares as absent, which reads as "everything the file says is
// an addition", which is exactly right for something not yet built.
func artifactFor(artifactID string) (workload.Document, error) {
	if artifactID == "" {
		return nil, nil
	}

	doc, err := getArtifactDocFn(artifactID)
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("cannot read artifact %s: %w", artifactID, err)
	}

	return doc, nil
}

// isLocked compares case-insensitively because the platform's own docs
// disagree with themselves about the casing of status enums.
func isLocked(status string) bool {
	return strings.EqualFold(status, workload.ArtifactStatusLocked)
}

// isNotFound reports whether err is the API's 404. The same three lines
// appear at twenty-odd call sites across this repo; retiring them behind a
// drapi helper is worth doing, but not from here.
func isNotFound(err error) bool {
	var httpErr *drapi.HTTPError

	return errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound
}
