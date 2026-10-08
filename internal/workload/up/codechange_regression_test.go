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
	"testing"

	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests guard `dr workload up`'s code-change measurement so the
// upload-integrity fix cannot regress it silently. defaultCodeChange builds
// a dry-run sync engine and counts plan upload + delete rows; the first-deploy
// flag is set when the project is not linked.
//
// The modified-tree and unchanged-tree count tests live in the sync package
// (TestWorkloadUp_CodeChangeCount_*) because defaultCodeChange uses sync.New
// with production deps for linked projects, which cannot be tested without
// staging. The sync engine's Plan() is the measurement's core, and testing it
// with injected fakes verifies the same logic.
//
// Fulfills VAL-REGRESSION-008.

// dockerfileManifestYAML is a minimal manifest with a Dockerfile build mode,
// which is what defaultCodeChange requires to measure code changes.
const dockerfileManifestYAML = `name: my-app
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
            imageBuildConfig:
              dockerfile:
                source: provided
runtime:
  containerGroups:
    - name: default
      replicaCount: 1
      containers:
        - name: primary
          resources:
            cpu: 0.5
            memory: 1Gi
`

// TestDefaultCodeChange_FirstDeploy verifies that defaultCodeChange reports
// the first-deploy flag for an unlinked project. When wapi.Exists returns
// false (no .datarobot/workload or .wapi directory), the measurement returns
// CodeChange{Applies: true, FirstDeploy: true} without calling the sync
// engine — there is nothing to compare against, so every file is new.
func TestDefaultCodeChange_FirstDeploy(t *testing.T) {
	dir := t.TempDir()

	// The project is not linked: no .datarobot/workload or .wapi directory.
	m, err := manifest.Parse([]byte(dockerfileManifestYAML), "")
	require.NoError(t, err)

	require.Equal(t, manifest.BuildModeDockerfile, m.BuildMode(),
		"fixture must have a Dockerfile build mode for code change to apply")

	loaded := Loaded{
		ProjectDir: dir,
		Manifest:   m,
	}

	change, err := defaultCodeChange(loaded, Live{})
	require.NoError(t, err)

	assert.True(t, change.Applies, "Dockerfile build mode means code change applies")
	assert.True(t, change.FirstDeploy,
		"unlinked project must report FirstDeploy=true (nothing to compare against)")
	assert.Equal(t, 0, change.Files,
		"first deploy has no diff count (every file is new, not changed)")
}

// TestDefaultCodeChange_ImageModeDoesNotApply verifies that a manifest with
// an image build mode (not Dockerfile or Generated) does not report a code
// change. A published image has no code to sync.
func TestDefaultCodeChange_ImageModeDoesNotApply(t *testing.T) {
	dir := t.TempDir()

	const imageManifestYAML = `name: my-app
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
            imageUri: registry.example.com/my-app:v1
runtime:
  containerGroups:
    - name: default
      replicaCount: 1
      containers:
        - name: primary
          resources:
            cpu: 0.5
            memory: 1Gi
`

	m, err := manifest.Parse([]byte(imageManifestYAML), "")
	require.NoError(t, err)

	assert.Equal(t, manifest.BuildModeImage, m.BuildMode(),
		"fixture must have an image build mode")

	loaded := Loaded{
		ProjectDir: dir,
		Manifest:   m,
	}

	change, err := defaultCodeChange(loaded, Live{})
	require.NoError(t, err)

	assert.False(t, change.Applies, "image build mode means no code change to measure")
	assert.False(t, change.FirstDeploy, "non-Dockerfile mode must not set FirstDeploy")
}
