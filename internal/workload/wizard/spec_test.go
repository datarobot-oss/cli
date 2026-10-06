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
	"os"
	"path/filepath"
	"testing"

	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// artifactSpec is what `dr artifact create --spec-file` takes: the artifact
// alone, with a prebuilt image so nothing in the directory is needed.
const artifactSpec = `name: prepared-artifact
spec:
  containerGroups:
    - name: default
      containers:
        - name: primary
          primary: true
          port: 9090
          imageUri: registry/app:v7
          readinessProbe:
            path: /healthz
            port: 9090
`

// workloadSpec is what `dr workload create --spec-file` takes: name, the
// artifact inline, and a runtime block.
const workloadSpec = `name: prepared-app
importance: high
artifact:
  name: prepared-app-artifact
  type: service
  spec:
    containerGroups:
      - name: default
        containers:
          - name: primary
            primary: true
            port: 9090
            imageUri: registry/app:v7
runtime:
  containerGroups:
    - name: default
      replicaCount: 3
      containers:
        - name: primary
          resourceAllocation: {cpu: 2, memory: 4GB}
`

func writeSpec(t *testing.T, dir, content string) string {
	t.Helper()

	path := filepath.Join(dir, "spec.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

func withSpec(dir, spec string, answers Answers) Options {
	opts := headless(dir, answers)
	opts.SpecFile = spec

	return opts
}

// An artifact spec plus a name is a whole setup: the artifact block comes
// from the file, the runtime is the default, and the spec file is untouched.
func TestRun_SpecFileArtifactSpecWritesTheManifest(t *testing.T) {
	dir := t.TempDir()
	spec := writeSpec(t, dir, artifactSpec)

	result, err := Run(withSpec(dir, spec, Answers{Name: "my-app"}))
	require.NoError(t, err)
	assert.Equal(t, ActionCreated, result.Action)

	content := string(result.Content)
	assert.Contains(t, content, "name: my-app\n")
	assert.Contains(t, content, "name: prepared-artifact")
	assert.Contains(t, content, "imageUri: registry/app:v7")
	assert.Contains(t, content, "path: /healthz")
	assert.Contains(t, content, "replicaCount: 1", "the default runtime is written")
	assert.NotContains(t, content, "workloadId")

	parsed, err := manifest.Load(result.Path)
	require.NoError(t, err)
	require.NoError(t, parsed.Validate())

	again, err := os.ReadFile(spec)
	require.NoError(t, err)
	assert.Equal(t, artifactSpec, string(again), "the spec file is input, not the manifest")
}

// A workload spec is taken as it is: its name, importance and runtime block
// reach the manifest, and a sizing flag still wins over the file.
func TestRun_SpecFileWorkloadSpecIsTakenAsItIs(t *testing.T) {
	dir := t.TempDir()
	spec := writeSpec(t, dir, workloadSpec)

	result, err := Run(withSpec(dir, spec, Answers{}))
	require.NoError(t, err)

	content := string(result.Content)
	assert.Contains(t, content, "name: prepared-app\n")
	assert.Contains(t, content, "importance: high")
	assert.Contains(t, content, "replicaCount: 3")
	assert.Contains(t, content, "memory: 4GB")

	fresh := t.TempDir()
	opts := withSpec(fresh, spec, Answers{Replicas: 5})
	opts.DryRun = true

	result, err = Run(opts)
	require.NoError(t, err)
	assert.Contains(t, string(result.Content), "replicaCount: 5", "an explicit sizing flag wins")
	assert.NoFileExists(t, manifest.Path(fresh))
}

// What the file leaves open has to come from somewhere headless.
func TestRun_SpecFileWithoutANameIsRefusedHeadless(t *testing.T) {
	dir := t.TempDir()
	spec := writeSpec(t, dir, artifactSpec)

	_, err := Run(withSpec(dir, spec, Answers{}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--name")
	assert.NoFileExists(t, manifest.Path(dir))
}

// The file is the build answer, so the flags that would contradict it are
// refused, and so is binding, since a prepared spec is a workload to create.
func TestRun_SpecFileRefusesTheFlagsItAnswers(t *testing.T) {
	dir := t.TempDir()
	spec := writeSpec(t, dir, artifactSpec)

	for name, answers := range map[string]Answers{
		"--image":       {Name: "x", Image: "other:1"},
		"--build-mode":  {Name: "x", BuildMode: manifest.BuildModeDockerfile},
		"--port":        {Name: "x", Port: 8080},
		"--workload-id": {WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Run(withSpec(dir, spec, answers))
			require.Error(t, err)
			assert.Contains(t, err.Error(), name)
		})
	}
}

// A file that is neither shape, or binds an artifact by id, has nothing to
// prepare from.
func TestLoadSpec_RefusesWhatItCannotPrepareFrom(t *testing.T) {
	dir := t.TempDir()

	for name, content := range map[string]string{
		"artifactId": "name: x\nartifactId: 68b0bbbb0000000000000002\n",
		"no spec":    "name: x\nimportance: low\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadSpec(writeSpec(t, dir, content))
			require.Error(t, err)
		})
	}
}

// Once a manifest exists the file is the source, so the flag is refused
// rather than silently ignored.
func TestRun_SpecFileIsRefusedOverAnExistingManifest(t *testing.T) {
	dir := t.TempDir()
	spec := writeSpec(t, dir, artifactSpec)
	require.NoError(t, os.WriteFile(manifest.Path(dir), []byte("name: here\nartifactId: 68b0bbbb0000000000000002\n"), 0o600))

	_, err := Run(withSpec(dir, spec, Answers{Name: "x"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already has one")
}

// On a terminal a workload spec with a name and a runtime block goes straight
// to confirm, and the manifest it produces is the file plus nothing.
func TestFlow_PreparedWorkloadSpecGoesStraightToConfirm(t *testing.T) {
	dir := t.TempDir()
	prepared, err := LoadSpec(writeSpec(t, dir, workloadSpec))
	require.NoError(t, err)

	model := newFlow(Detect(dir), nil, Answers{}).withPrepared(prepared)
	require.Equal(t, screenConfirm, model.at)
	require.NoError(t, model.failed)
	assert.Contains(t, string(model.content), "name: prepared-app\n")
	assert.Contains(t, string(model.content), "replicaCount: 3")

	model = press(t, model, "enter")
	assert.True(t, model.done)
}

// An artifact spec leaves the name and the sizing open: the name screen
// first, then the settings with the file's port as the default, then confirm.
func TestFlow_PreparedArtifactSpecAsksOnlyWhatItLeavesOpen(t *testing.T) {
	dir := t.TempDir()
	prepared, err := LoadSpec(writeSpec(t, dir, artifactSpec))
	require.NoError(t, err)

	model := newFlow(Detect(dir), nil, Answers{}).withPrepared(prepared)
	require.Equal(t, screenName, model.at)

	model = pastName(t, model)
	require.Equal(t, screenSettings, model.at, "the build screens are skipped; the sizing is open")
	assert.Equal(t, 9090, model.draft.Port, "the file's port is the default")

	model = press(t, model, "enter")
	require.Equal(t, screenConfirm, model.at)
	require.NoError(t, model.failed)
	assert.Contains(t, string(model.content), "name: test-app\n")
	assert.Contains(t, string(model.content), "name: prepared-artifact")

	// Back from confirm lands on the settings, and back from there on the
	// name: the skipped screens are skipped both ways.
	model = press(t, model, "esc", "esc")
	assert.Equal(t, screenName, model.at)
}

// A .env is still asked about: the file says nothing about it.
func TestFlow_PreparedSpecStillAsksAboutTheEnvFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, EnvFileName), []byte("LOG_LEVEL=debug\n"), 0o600))

	prepared, err := LoadSpec(writeSpec(t, dir, workloadSpec))
	require.NoError(t, err)

	model := newFlow(Detect(dir), nil, Answers{}).withPrepared(prepared)
	assert.Equal(t, screenEnv, model.at)
}
