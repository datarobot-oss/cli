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

package del

import (
	"bytes"
	"net/http"
	"os"
	"testing"

	"github.com/datarobot/cli/cmd/workload/internal/idargs"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// purgeManifest binds a workload and references two credentials: one setup
// minted for this project, one somebody pointed the file at by hand.
const purgeManifest = `workloadId: 68b0c1d2e3f4a5b6c7d8e9f0
name: my-app
artifact:
  name: my-app-artifact
  spec:
    type: service
    containerGroups:
      - name: default
        containers:
          - name: primary
            primary: true
            port: 8080
            imageUri: nginx:latest
            environmentVars:
              - name: API_KEY
                value: dr-credential:68f0cccc0000000000000001/apiToken
              - name: SHARED
                value: dr-credential:68f0cccc0000000000000002/apiToken
`

func readFile(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(content)
}

// purgeFakes wires the four platform calls and records what was deleted.
type purgeFakes struct {
	artifact    *workload.Artifact
	artifactErr error
	deleteErr   error
	credentials map[string]string // id -> name

	deletedArtifacts   []string
	deletedCredentials []string
}

func installPurge(t *testing.T, f *purgeFakes) {
	t.Helper()

	original := [4]any{getArtifactFn, deleteArtifactFn, getCredentialFn, deleteCredentialFn}

	getArtifactFn = func(string) (*workload.Artifact, error) { return f.artifact, f.artifactErr }
	deleteArtifactFn = func(id string) error {
		if f.deleteErr != nil {
			return f.deleteErr
		}

		f.deletedArtifacts = append(f.deletedArtifacts, id)

		return nil
	}
	getCredentialFn = func(id string) (*workload.Credential, error) {
		name, ok := f.credentials[id]
		if !ok {
			return nil, &drapi.HTTPError{StatusCode: http.StatusNotFound}
		}

		return &workload.Credential{CredentialID: id, Name: name}, nil
	}
	deleteCredentialFn = func(id string) error {
		f.deletedCredentials = append(f.deletedCredentials, id)

		return nil
	}

	t.Cleanup(func() {
		getArtifactFn, _ = original[0].(func(string) (*workload.Artifact, error))
		deleteArtifactFn, _ = original[1].(func(string) error)
		getCredentialFn, _ = original[2].(func(string) (*workload.Credential, error))
		deleteCredentialFn, _ = original[3].(func(string) error)
	})
}

// A purge removes exactly the deploy's leftovers: the credential setup
// minted, the draft artifact, the state directory. The credential someone
// pointed the file at by hand is kept and named, and the manifest keeps its
// environment variables.
func TestPurge_RemovesTheDeploysLeftoversAndKeepsWhatIsNotIts(t *testing.T) {
	dir := t.TempDir()
	path := writeManifest(t, dir, purgeManifest)
	require.NoError(t, wapi.Initialize(dir, wapi.InitOptions{ArtifactID: "68a0000000000000000000a1"}))

	f := &purgeFakes{
		artifact: &workload.Artifact{ID: "68a0000000000000000000a1", Status: workload.ArtifactStatusDraft},
		credentials: map[string]string{
			"68f0cccc0000000000000001": "my-app/API_KEY",
			"68f0cccc0000000000000002": "team-shared-key",
		},
	}
	installPurge(t, f)

	var buf bytes.Buffer

	clearStaleBinding(&buf, dir, boundID, true)

	out := buf.String()
	assert.Contains(t, out, "Removed workloadId")
	assert.Contains(t, out, "Removed credential 68f0cccc0000000000000001 (my-app/API_KEY)")
	assert.Contains(t, out, "Removed artifact 68a0000000000000000000a1")
	assert.Contains(t, out, "Removed "+wapi.Dir(dir))
	assert.Contains(t, out, "Kept credential 68f0cccc0000000000000002 (team-shared-key)")
	assert.Contains(t, out, "not minted by this project")
	assert.NotContains(t, out, "still linked to artifact", "the link note is for a delete that keeps it")

	assert.Equal(t, []string{"68f0cccc0000000000000001"}, f.deletedCredentials)
	assert.Equal(t, []string{"68a0000000000000000000a1"}, f.deletedArtifacts)
	assert.False(t, wapi.Exists(dir), "the state directory is gone")

	content := readFile(t, path)
	assert.Contains(t, content, "dr-credential:68f0cccc0000000000000001/apiToken", "the manifest keeps its variables")
	assert.NotContains(t, content, "workloadId")
}

// A locked artifact cannot be deleted, and one another workload still runs
// is left to that workload; both are named with what to do.
func TestPurge_LeavesALockedOrReferencedArtifactAndSaysWhy(t *testing.T) {
	for name, f := range map[string]*purgeFakes{
		"locked": {artifact: &workload.Artifact{ID: "68a0000000000000000000a1", Status: workload.ArtifactStatusLocked}},
		"referenced": {
			artifact:  &workload.Artifact{ID: "68a0000000000000000000a1", Status: workload.ArtifactStatusDraft},
			deleteErr: &drapi.HTTPError{StatusCode: http.StatusConflict},
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeManifest(t, dir, boundManifest)
			require.NoError(t, wapi.Initialize(dir, wapi.InitOptions{ArtifactID: "68a0000000000000000000a1"}))
			installPurge(t, f)

			var buf bytes.Buffer

			clearStaleBinding(&buf, dir, boundID, true)

			assert.Contains(t, buf.String(), "Kept artifact 68a0000000000000000000a1")
			assert.Empty(t, f.deletedArtifacts)
			assert.False(t, wapi.Exists(dir), "the state directory still goes: the link was this project's")
		})
	}
}

// An artifact already gone is not a failure: the link to it is what is
// removed, and the message says the artifact was already gone.
func TestPurge_AnArtifactAlreadyGoneIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, boundManifest)
	require.NoError(t, wapi.Initialize(dir, wapi.InitOptions{ArtifactID: "68a0000000000000000000a1"}))
	installPurge(t, &purgeFakes{artifactErr: &drapi.HTTPError{StatusCode: http.StatusNotFound}})

	var buf bytes.Buffer

	clearStaleBinding(&buf, dir, boundID, true)

	assert.Contains(t, buf.String(), "already gone")
	assert.False(t, wapi.Exists(dir))
}

// Without a manifest naming the workload there is nothing to tie leftovers
// to, so a purge removes nothing beyond the workload and says so.
func TestPurge_WithoutAMatchingManifestNothingIsPurged(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "workloadId: 68b0c1d2e3f4a5b6c7d8e9ff\nname: other\nartifactId: 68b0bbbb0000000000000002\n")
	require.NoError(t, wapi.Initialize(dir, wapi.InitOptions{ArtifactID: "68a0000000000000000000a1"}))

	f := &purgeFakes{artifact: &workload.Artifact{ID: "68a0000000000000000000a1", Status: workload.ArtifactStatusDraft}}
	installPurge(t, f)

	var buf bytes.Buffer

	clearStaleBinding(&buf, dir, boundID, true)

	assert.Contains(t, buf.String(), "Nothing purged")
	assert.Empty(t, f.deletedArtifacts)
	assert.True(t, wapi.Exists(dir), "the state directory belongs to another workload's project")
}

// The confirmation names what a purge removes, so --yes to it is consent to
// all of it; a plain delete asks the same question as before.
func TestDeleteQuestion_PurgeNamesWhatElseGoes(t *testing.T) {
	ref := idargs.Ref{ID: "wl-1"}

	assert.NotContains(t, deleteQuestion(ref, false), "--purge")
	assert.Contains(t, deleteQuestion(ref, true), "the credentials this project minted")
	assert.Contains(t, deleteQuestion(ref, true), "the local state directory")
}
