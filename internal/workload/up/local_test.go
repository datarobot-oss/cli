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
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/docker"
	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	localArtifactID = "6901f3a0c2b7e4d1a5f0b9c3"
	localAddress    = "hub.example/artifacts/" + localArtifactID
	localDigest     = "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
)

// unboundLocalManifest names neither an image nor a build: the platform
// issues an address and `up` pushes to it.
const unboundLocalManifest = `name: my-app
importance: low
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
runtime:
  containerGroups:
    - name: default
      replicaCount: 1
      containers:
        - name: primary
          resourceAllocation: {cpu: 0.5, memory: 512MB}
`

func addressedArtifactDoc(string) (workload.Document, error) {
	return workload.Document{
		"id": localArtifactID,
		"spec": map[string]any{"containerGroups": []any{map[string]any{
			"name":       "default",
			"containers": []any{map[string]any{"name": "primary", "primary": true, "imageUri": localAddress}},
		}}},
	}, nil
}

// wiredDocker records the docker calls into tr and answers every push with
// localDigest.
func wiredDocker(t *testing.T, tr *track) {
	t.Helper()

	force(t, &dockerAvailableFn, func(context.Context) error { return nil })
	force(t, &dockerBuildFn, func(_ context.Context, _, tag string, _ func(string)) error {
		tr.steps = append(tr.steps, "docker-build "+tag)

		return nil
	})
	force(t, &dockerPushFn, func(_ context.Context, ref docker.Reference, token string, _ func(string)) (string, error) {
		assert.Equal(t, "tok", token)

		tr.steps = append(tr.steps, "docker-push "+ref.Address())

		return localDigest, nil
	})
	force(t, &apiTokenFn, func() string { return "tok" })
}

func wiredLocal(t *testing.T, tr *track) fakes {
	t.Helper()
	t.Setenv("DATAROBOT_CLI_FEATURE_LOCAL_BUILD", "true")

	wiredDocker(t, tr)

	f := wiredBuild(tr)
	f.code = func(Loaded, Live) (CodeChange, error) {
		return CodeChange{Applies: true, Local: true, FirstDeploy: true, TreeHash: "tree-1"}, nil
	}
	f.newArtifact = func(payload any) (*workload.Artifact, error) {
		tr.steps = append(tr.steps, "create-artifact")
		tr.artifactIn = payload

		return &workload.Artifact{ID: localArtifactID, Status: workload.ArtifactStatusDraft}, nil
	}
	f.artifactD = addressedArtifactDoc
	f.sync = func(string) (*sync.Result, error) {
		t.Fatal("a local build uploads no code")

		return nil, nil
	}

	return f
}

func TestRun_LocalFirstDeployBuildsAndPushesThenCreates(t *testing.T) {
	var tr track

	install(t, wiredLocal(t, &tr))

	result, stderr, err := runIn(t, unboundLocalManifest, Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(t, []string{
		"create-artifact", "link", "docker-build " + localAddress, "docker-push " + localAddress, "create-workload",
	}, tr.steps)
	assert.Equal(t, localAddress, result.Image)
	assert.Empty(t, result.BuildID, "no platform build ran")
	assert.Contains(t, stderr, "Pushed "+localAddress+"@"+localDigest)

	container := decodePayload(t, tr.artifactIn)["spec"].(map[string]any)["containerGroups"].([]any)[0].(map[string]any)["containers"].([]any)[0].(map[string]any)
	assert.NotContains(t, container, "imageUri", "leaving it out is what asks for an address")
	assert.Equal(t, localArtifactID, decodePayload(t, tr.workloadIn)["artifactId"])
}

// Without Docker the run stops before the platform is touched.
func TestRun_LocalWithoutDockerCreatesNothing(t *testing.T) {
	var tr track

	install(t, wiredLocal(t, &tr))
	force(t, &dockerAvailableFn, func(context.Context) error { return errors.New("no docker") })

	_, _, err := runIn(t, unboundLocalManifest, Options{NonInteractive: true})
	require.ErrorContains(t, err, "no docker")
	assert.Empty(t, tr.steps)
}

func TestPushLocally_SkipsTheSameTreeToTheSameArtifact(t *testing.T) {
	var tr track

	wiredDocker(t, &tr)
	force(t, &getArtifactDocFn, addressedArtifactDoc)
	force(t, &projectLinkedFn, func(string) bool { return true })

	pushedTo, tree := localArtifactID, "tree-1"
	cfg := wapi.Config{ArtifactID: localArtifactID, LastPushedArtifactID: &pushedTo, LastPushedTreeHash: &tree}

	force(t, &loadProjectFn, func(string) (wapi.Config, error) { return cfg, nil })
	force(t, &saveProjectFn, func(_ string, c wapi.Config) error {
		cfg = c

		return nil
	})

	report := newReporter(&bytes.Buffer{}, false)
	made := version{ID: localArtifactID}

	image, err := pushLocally(t.Context(), "/p", made, CodeChange{Local: true, TreeHash: "tree-1"}, Options{}, report)
	require.NoError(t, err)
	assert.Equal(t, localAddress, image)
	assert.Empty(t, tr.steps, "the image is already there")

	// --force-build pushes anyway, and a changed tree does too; both record it.
	_, err = pushLocally(t.Context(), "/p", made, CodeChange{Local: true, TreeHash: "tree-1"}, Options{ForceBuild: true}, report)
	require.NoError(t, err)

	_, err = pushLocally(t.Context(), "/p", made, CodeChange{Local: true, TreeHash: "tree-2"}, Options{}, report)
	require.NoError(t, err)

	assert.Len(t, tr.steps, 4)
	assert.Equal(t, "tree-2", deref(cfg.LastPushedTreeHash))
	assert.Equal(t, localDigest, deref(cfg.LastPushedDigest))
}

// A new version has its own address, and nothing has been pushed to it yet,
// so the same tree is pushed again.
func TestPushLocally_PushesTheSameTreeToANewVersion(t *testing.T) {
	var tr track

	wiredDocker(t, &tr)
	force(t, &projectLinkedFn, func(string) bool { return true })

	other, tree := "6901f3a0c2b7e4d1a5f0b9c4", "tree-1"

	force(t, &loadProjectFn, func(string) (wapi.Config, error) {
		return wapi.Config{ArtifactID: localArtifactID, LastPushedArtifactID: &other, LastPushedTreeHash: &tree}, nil
	})
	force(t, &saveProjectFn, func(string, wapi.Config) error { return nil })

	_, err := pushLocally(t.Context(), "/p", version{ID: localArtifactID, ImageURI: localAddress},
		CodeChange{Local: true, TreeHash: "tree-1"}, Options{}, newReporter(&bytes.Buffer{}, false))
	require.NoError(t, err)
	assert.Len(t, tr.steps, 2)
}

func TestPushTarget_RefusesWhatAPushCannotReach(t *testing.T) {
	_, err := pushTarget(version{ID: localArtifactID, ImageURI: "nginx:latest"})
	require.ErrorContains(t, err, "no address to push its image to")

	_, err = pushTarget(version{ID: localArtifactID, ImageURI: localAddress + "@" + localDigest})
	require.ErrorContains(t, err, "a push to it would not change")
}

func TestLocalCodeChange(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.py"), []byte("print(1)\n"), 0o600))

	force(t, &treeHashFn, treeHash)
	force(t, &projectLinkedFn, func(string) bool { return false })

	loaded := Loaded{ProjectDir: dir}

	first, err := localCodeChange(loaded, Live{})
	require.NoError(t, err)
	assert.True(t, first.FirstDeploy)
	assert.True(t, first.Changed())

	pending := localArtifactID
	cfg := wapi.Config{
		ArtifactID: localArtifactID, LastPushedTreeHash: &first.TreeHash,
		LastPushedArtifactID: &pending, PendingRolloutArtifactID: &pending,
	}

	force(t, &projectLinkedFn, func(string) bool { return true })
	force(t, &loadProjectFn, func(string) (wapi.Config, error) { return cfg, nil })

	same, err := localCodeChange(loaded, Live{ArtifactID: localArtifactID})
	require.NoError(t, err)
	assert.False(t, same.Changed(), "the tree was pushed to the version that is running")

	// Pushed, marked pending and never served: the next run rolls it out.
	unserved, err := localCodeChange(loaded, Live{ArtifactID: "6901f3a0c2b7e4d1a5f0b9c4"})
	require.NoError(t, err)
	assert.True(t, unserved.Unserved)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.py"), []byte("print(2)\n"), 0o600))

	edited, err := localCodeChange(loaded, Live{ArtifactID: localArtifactID})
	require.NoError(t, err)
	assert.True(t, edited.TreeChanged)
}

func TestTreeHash_FollowsTheSyncEnginesWalk(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.py"), []byte("x\n"), 0o600))

	before, err := treeHash(dir)
	require.NoError(t, err)

	// The CLI's own state is never part of the tree.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".datarobot", "workload"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".datarobot", "workload", "config.json"), []byte("{}"), 0o600))

	after, err := treeHash(dir)
	require.NoError(t, err)
	assert.Equal(t, before, after)

	if runtime.GOOS == "windows" {
		return
	}

	require.NoError(t, os.Chmod(filepath.Join(dir, "app.py"), 0o700))

	executable, err := treeHash(dir)
	require.NoError(t, err)
	assert.NotEqual(t, before, executable, "Docker copies the mode, so it is part of the image")
}
