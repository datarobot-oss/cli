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
	"encoding/json"
	"testing"

	"github.com/datarobot/cli/internal/workload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// namedRoll is a running workload whose file names another artifact by id.
func namedRoll(tr *track, repository string, locked bool) fakes {
	f := wiredRoll(tr)
	f.artifactD = func(string) (workload.Document, error) {
		d := docOf(liveImageArtifactJSON)
		d["artifactRepositoryId"] = "repo-1"

		return d, nil
	}
	f.getArtifact = func(id string) (*workload.Artifact, error) {
		status := workload.ArtifactStatusDraft
		if locked {
			status = workload.ArtifactStatusLocked
		}

		return &workload.Artifact{ID: id, Status: status, ArtifactRepositoryID: repository}, nil
	}

	return f
}

const namedManifest = "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0\n" + boundArtifactManifest

// The platform swaps a workload only onto a version of its own repository,
// and used to say so at apply time, after a dry run had said the roll would
// work. The plan says it now, before anything is sent.
func TestRun_NamedArtifactFromAnotherRepositoryIsRefused(t *testing.T) {
	var tr track

	f := namedRoll(&tr, "repo-2", false)
	f.replace = func(string, string, json.RawMessage) (*workload.Replacement, error) {
		t.Fatal("a swap the platform refuses is not sent")

		return nil, nil
	}

	install(t, f)

	for name, dryRun := range map[string]bool{"a deploy": false, "a dry run": true} {
		t.Run(name, func(t *testing.T) {
			result, _, err := runIn(t, namedManifest, Options{NonInteractive: true, DryRun: dryRun})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "repository repo-2")
			assert.Contains(t, err.Error(), "repository repo-1")
			assert.Contains(t, result.Plan.Incompatible, "repository")
		})
	}
}

// A draft workload cannot take a locked version. The other direction is a
// promotion the deploy makes work by locking the draft first, so only this
// one is refused.
func TestRun_LockedArtifactNamedByADraftWorkloadIsRefused(t *testing.T) {
	var tr track

	install(t, namedRoll(&tr, "repo-1", true))

	_, _, err := runIn(t, namedManifest, Options{NonInteractive: true, DryRun: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a draft workload cannot take a locked version")
}

// A swap onto an artifact that exists reads as one: nothing is minted and
// nothing is built, and the plan must not say "new version".
func TestRun_NamedArtifactPlanReadsAsASwap(t *testing.T) {
	var tr track

	install(t, namedRoll(&tr, "repo-1", false))

	result, stderr, err := runIn(t, namedManifest, Options{NonInteractive: true, DryRun: true})
	require.NoError(t, err)
	assert.Empty(t, result.Plan.Incompatible)
	assert.Contains(t, stderr, "swaps to 68b0bbbb0000000000000002")
	assert.Contains(t, stderr, "nothing is built")
	assert.NotContains(t, stderr, "new version")
}

// A create on a named artifact comes up on it; "its first artifact" would
// claim one is made.
func TestRun_CreateOnANamedArtifactSaysSo(t *testing.T) {
	install(t, fakes{
		getArtifact: func(id string) (*workload.Artifact, error) {
			return &workload.Artifact{ID: id, Status: workload.ArtifactStatusDraft}, nil
		},
	})

	_, stderr, err := runIn(t, boundArtifactManifest, Options{NonInteractive: true, DryRun: true})
	require.NoError(t, err)
	assert.Contains(t, stderr, "on artifact 68b0bbbb0000000000000002")
	assert.NotContains(t, stderr, "first artifact")
}

// The artifact the file names is read once for the plan; a read that fails
// stops the run, since the swap was going to fail on it anyway.
func TestRun_NamedArtifactThatCannotBeReadStopsTheRun(t *testing.T) {
	var tr track

	f := namedRoll(&tr, "repo-1", false)
	f.getArtifact = func(string) (*workload.Artifact, error) { return nil, assert.AnError }

	install(t, f)

	_, _, err := runIn(t, namedManifest, Options{NonInteractive: true, DryRun: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot read artifact 68b0bbbb0000000000000002")
}
