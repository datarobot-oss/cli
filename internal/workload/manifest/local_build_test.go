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

package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const localBuildGateEnv = "DATAROBOT_CLI_FEATURE_LOCAL_BUILD"

func dirWithDockerfile(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, dockerfileName), []byte("FROM scratch\n"), 0o600))

	return dir
}

func TestValidate_LocalBuildOnlyBehindTheGate(t *testing.T) {
	dir := dirWithDockerfile(t)
	content := serviceWithContainerBody("")

	requireFindings(t, validateString(t, dir, content), []FieldError{
		{Line: 9, Path: "artifact.spec.containerGroups[0].containers[0]", Msg: "must set either imageUri or imageBuildConfig"},
	})

	t.Setenv(localBuildGateEnv, "true")

	require.NoError(t, validateString(t, dir, content))

	m, err := Parse([]byte(content), dir)
	require.NoError(t, err)
	assert.Equal(t, BuildModeLocal, m.BuildMode())
}

func TestValidate_LocalBuildNeedsDockerfile(t *testing.T) {
	t.Setenv(localBuildGateEnv, "true")

	requireFindings(t, validateString(t, t.TempDir(), serviceWithContainerBody("")), []FieldError{
		{Line: 9, Path: "artifact.spec.containerGroups[0].containers[0]", Msg: "no Dockerfile exists in"},
	})
}

// Only the primary container receives a push; the platform refuses a second.
func TestValidate_LocalBuildIsForThePrimaryOnly(t *testing.T) {
	t.Setenv(localBuildGateEnv, "true")

	content := serviceWithContainerBody(`          - name: sidecar
            primary: false
`)

	requireFindings(t, validateString(t, dirWithDockerfile(t), content), []FieldError{
		{Line: 12, Path: "artifact.spec.containerGroups[0].containers[1]", Msg: "must set either imageUri or imageBuildConfig"},
	})
}

func TestRender_LocalBuild(t *testing.T) {
	t.Setenv(localBuildGateEnv, "true")

	draft := serviceDraft()
	draft.Build = Build{Mode: BuildModeLocal}

	out, err := draft.Render()
	require.NoError(t, err)

	assert.NotContains(t, string(out), keyImageURI)
	assert.NotContains(t, string(out), keyImageBuildConfig)
	assert.Contains(t, string(out), "primary: true # "+localBuildComment)

	m, err := Parse(out, dirWithDockerfile(t))
	require.NoError(t, err)
	require.NoError(t, m.Validate())
	assert.Equal(t, BuildModeLocal, m.BuildMode())
}

// A workload whose image was pushed binds as a local build: its reference is
// issued per artifact and must not be copied into the file.
func TestLiveBuild_PushedImageIsALocalBuild(t *testing.T) {
	for _, uri := range []string{
		"hub.example/artifacts/6901f3a0c2b7e4d1a5f0b9c3",
		"hub.example/artifacts/6901f3a0c2b7e4d1a5f0b9c3@sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
	} {
		assert.Equal(t, Build{Mode: BuildModeLocal}, liveBuild(map[string]any{keyImageURI: uri}), uri)
	}

	assert.Equal(t, Build{Mode: BuildModeImage, ImageURI: "nginx:latest"},
		liveBuild(map[string]any{keyImageURI: "nginx:latest"}))
}
