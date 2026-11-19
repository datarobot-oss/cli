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

package wizard

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const portableWorkloadDoc = `{"name": "canvas-agent", "importance": "low", "artifactId": "68a1",
	"runtime": {"containerGroups": [{"name": "default", "replicaCount": 1,
		"containers": [{"name": "primary", "resourceAllocation": {"cpu": 0.5, "memory": "512MB"}}]}]}}`

const portableArtifactDoc = `{"name": "canvas-agent-artifact", "type": "service", "artifactRepositoryId": "68r1",
	"spec": {"containerGroups": [{"name": "default", "containers": [{
		"name": "primary", "primary": true, "port": 8080,
		"imageBuildConfig": {"dockerfile": {"source": "provided", "path": "./Dockerfile"}},
		"environmentVars": [
			{"name": "DATAROBOT_ENDPOINT", "source": "string", "value": "https://app.datarobot.com/api/v2"},
			{"name": "DATAROBOT_API_TOKEN", "source": "dr-credential", "drCredentialId": "68f0cccc0000000000000003", "key": "apiToken"}
		]}]}]}}`

func TestPreparePortable_RendersAnUnboundManifestWithoutTheCode(t *testing.T) {
	stubLive(t, documentFrom(t, portableWorkloadDoc), documentFrom(t, portableArtifactDoc))

	var stderr bytes.Buffer

	portable, err := PreparePortable("68b0c1d2e3f4a5b6c7d8e9f0", "68r1", &stderr)
	require.NoError(t, err)

	assert.Equal(t, "canvas-agent", portable.Name)
	assert.Equal(t, 1, portable.CredentialsPending)
	assert.Empty(t, stderr.String(), "the same lineage warns about nothing")

	// A provided build with no Dockerfile on disk yet: the code arrives with
	// the sync that follows, so the file has to be writable before it.
	dir := t.TempDir()

	path, err := portable.Write(dir)
	require.NoError(t, err)
	assert.Equal(t, manifest.Path(dir), path)

	written, err := os.ReadFile(path)
	require.NoError(t, err)

	assert.NotContains(t, string(written), "workloadId")
	assert.NotContains(t, string(written), "68b0c1d2e3f4a5b6c7d8e9f0")
	assert.NotContains(t, string(written), "68f0cccc0000000000000003")
	assert.Contains(t, string(written), manifest.CredentialPlaceholder)
	assert.Contains(t, string(written), "name: canvas-agent")
}

func TestPreparePortable_WarnsWhenTheWorkloadRunsAnotherLineage(t *testing.T) {
	stubLive(t, documentFrom(t, portableWorkloadDoc), documentFrom(t, portableArtifactDoc))

	var stderr bytes.Buffer

	_, err := PreparePortable("68b0c1d2e3f4a5b6c7d8e9f0", "68other", &stderr)
	require.NoError(t, err)

	assert.Contains(t, stderr.String(), "does not run this artifact's lineage")
}

func TestPreparePortable_UnknownWorkloadFails(t *testing.T) {
	stubLiveErr(t, assert.AnError)

	_, err := PreparePortable("68b0c1d2e3f4a5b6c7d8e9f0", "68r1", nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
}

func TestPortableManifest_WriteNeverReplacesAManifest(t *testing.T) {
	stubLive(t, documentFrom(t, portableWorkloadDoc), documentFrom(t, portableArtifactDoc))

	portable, err := PreparePortable("68b0c1d2e3f4a5b6c7d8e9f0", "68r1", nil)
	require.NoError(t, err)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(manifest.Path(dir), []byte("name: mine\n"), 0o600))

	_, err = portable.Write(dir)

	require.ErrorIs(t, err, manifest.ErrExists)

	kept, err := os.ReadFile(manifest.Path(dir))
	require.NoError(t, err)
	assert.Equal(t, "name: mine\n", string(kept))
}

func TestWorkloadsRunning_ReadsEveryPageAndMatchesTheArtifactExactly(t *testing.T) {
	all := make([]workload.Workload, 250)
	for i := range all {
		all[i] = workload.Workload{ID: fmt.Sprintf("w%03d", i), ArtifactID: "art-other"}
	}

	all[5].ArtifactID = "art-1"
	all[205].ArtifactID = "art-1"

	var offsets []int

	t.Cleanup(SetListWorkloadsFnForTest(func(limit, offset int, _ []string, _ string) ([]workload.Workload, error) {
		offsets = append(offsets, offset)

		return all[offset:min(offset+limit, len(all))], nil
	}))

	found, err := WorkloadsRunning("art-1")
	require.NoError(t, err)

	require.Len(t, found, 2)
	assert.Equal(t, []string{"w005", "w205"}, []string{found[0].ID, found[1].ID})
	assert.Equal(t, []int{0, 100, 200}, offsets, "a match on the last page is found")
}

func TestWorkloadsRunning_NoMatchIsNotAnError(t *testing.T) {
	t.Cleanup(SetListWorkloadsFnForTest(func(int, int, []string, string) ([]workload.Workload, error) {
		return []workload.Workload{{ID: "w1", ArtifactID: "art-other"}}, nil
	}))

	found, err := WorkloadsRunning("art-1")

	require.NoError(t, err)
	assert.Empty(t, found)
}

func TestWorkloadsRunning_ListFailureNamesTheArtifact(t *testing.T) {
	t.Cleanup(SetListWorkloadsFnForTest(func(int, int, []string, string) ([]workload.Workload, error) {
		return nil, assert.AnError
	}))

	_, err := WorkloadsRunning("art-1")

	require.ErrorIs(t, err, assert.AnError)
	assert.Contains(t, err.Error(), "art-1")
}
