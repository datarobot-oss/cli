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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/workload"
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
// A fresh directory per case, so a refusal that went missing shows up as its
// own failure rather than as "already has one" for the rest.
func TestRun_SpecFileRefusesTheFlagsItAnswers(t *testing.T) {
	for name, answers := range map[string]Answers{
		"--image":       {Name: "x", Image: "other:1"},
		"--build-mode":  {Name: "x", BuildMode: manifest.BuildModeDockerfile},
		"--port":        {Name: "x", Port: 8080},
		"--type":        {Name: "x", Type: manifest.TypeAgent},
		"--workload-id": {WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()

			_, err := Run(withSpec(dir, writeSpec(t, dir, artifactSpec), answers))
			require.Error(t, err)
			assert.Contains(t, err.Error(), name)
			assert.Contains(t, err.Error(), "--spec-file")
			assert.NoFileExists(t, manifest.Path(dir))
		})
	}

	t.Run("--sync-env", func(t *testing.T) {
		dir := t.TempDir()
		opts := withSpec(dir, writeSpec(t, dir, artifactSpec), Answers{Name: "x"})
		opts.SyncEnv = true

		_, err := Run(opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--sync-env cannot be combined with --spec-file")
	})
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

// A spec whose spec block has no container passes the shape check but has
// nowhere to write the answers, so it is refused up front with the message
// the loader promises rather than by Apply, naming a file that does not
// exist yet.
func TestLoadSpec_RefusesASpecWithNoPrimaryContainer(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadSpec(writeSpec(t, dir, "name: hollow\nspec:\n  containerGroups: []\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "primary container")
}

// The kind is the file's answer too.
func TestRun_SpecFileRefusesTheKindFlags(t *testing.T) {
	dir := t.TempDir()
	spec := writeSpec(t, dir, artifactSpec)

	for name, answers := range map[string]Answers{
		"type": {Name: "my-app", Type: manifest.TypeAgent},
		"a2a":  {Name: "my-app", A2AEnabled: true},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Run(withSpec(dir, spec, answers))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "cannot be combined with --spec-file")
		})
	}
}

// A build the directory cannot support is refused before anything is asked,
// on a terminal as headless: the source screen that would have said so is
// the one the file skips.
func TestFlow_PreparedSpecRefusesABuildTheDirectoryCannotMake(t *testing.T) {
	for name, c := range map[string]struct {
		build string
		want  string
	}{
		"dockerfile build with no Dockerfile": {
			build: "imageBuildConfig: {dockerfile: {source: provided}}",
			want:  "has none",
		},
		"generated build with nothing to build from": {
			build: "imageBuildConfig: {dockerfile: {source: generated, executionEnvironmentId: a, executionEnvironmentVersionId: b, entrypoint: [python, app.py]}}",
			want:  "cannot be made from",
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			spec := strings.Replace(artifactSpec, "imageUri: registry/app:v7", c.build, 1)

			prepared, err := LoadSpec(writeSpec(t, dir, spec))
			require.NoError(t, err)

			model := newFlow(Detect(dir), nil, Answers{}).withPrepared(prepared)
			require.Error(t, model.failed)
			assert.Contains(t, model.failed.Error(), c.want)

			_, err = Run(withSpec(dir, writeSpec(t, dir, spec), Answers{Name: "my-app"}))
			require.Error(t, err, "headless says the same")
			assert.Contains(t, err.Error(), c.want)
		})
	}
}

// The confirm screen speaks of a new workload, not of one running: a spec
// file is as new as a fresh setup, so there is no diff and no "(running)".
func TestFlow_PreparedSpecConfirmReadsAsAFreshSetup(t *testing.T) {
	dir := t.TempDir()
	prepared, err := LoadSpec(writeSpec(t, dir, workloadSpec))
	require.NoError(t, err)

	model := newFlow(Detect(dir), nil, Answers{}).withPrepared(prepared)
	require.Equal(t, screenConfirm, model.at)

	view := model.View()
	assert.Contains(t, view, "creates a new workload")
	assert.NotContains(t, view, "(running)")
	assert.NotContains(t, view, "no change to the running workload")
	assert.Empty(t, model.diff)
}

// Renaming on the way back through the name screen renames the artifact too:
// the first derivation must not stick to the shared spec.
func TestFlow_PreparedArtifactSpecFollowsARename(t *testing.T) {
	dir := t.TempDir()
	prepared, err := LoadSpec(writeSpec(t, dir, strings.Replace(artifactSpec, "name: prepared-artifact\n", "", 1)))
	require.NoError(t, err)

	model := newFlow(Detect(dir), nil, Answers{}).withPrepared(prepared)
	model = press(t, pastName(t, model), "enter")
	require.Equal(t, screenConfirm, model.at)
	assert.Contains(t, string(model.content), "name: test-app"+manifest.ArtifactNameSuffix)

	model = press(t, model, "esc", "esc")
	require.Equal(t, screenName, model.at)

	model = press(t, press(t, typeInto(t, model, "renamed-app"), "enter"), "enter")
	require.Equal(t, screenConfirm, model.at)
	assert.Contains(t, string(model.content), "name: renamed-app"+manifest.ArtifactNameSuffix)
	assert.NotContains(t, string(model.content), "test-app")
}

// Setup run from the parent of the project still offers the directory first.
// Choosing the project keeps the file's answers: the build, the port and the
// name are the spec's, not the chosen directory's.
func TestFlow_PreparedSpecSurvivesTheDirectoryScreen(t *testing.T) {
	parent := t.TempDir()
	app := filepath.Join(parent, "my-app")
	require.NoError(t, os.MkdirAll(app, 0o755))
	writeDockerfile(t, app, "FROM scratch\nEXPOSE 3000\n")

	prepared, err := LoadSpec(writeSpec(t, parent, workloadSpec))
	require.NoError(t, err)

	model := newFlow(Detect(parent), nil, Answers{}).withPrepared(prepared)
	require.Equal(t, screenDirectory, model.at)

	model = press(t, model, "enter")
	require.NoError(t, model.failed)
	assert.Equal(t, app, model.detected.Dir)
	assert.Equal(t, "prepared-app", model.draft.Name, "the file's name, not the directory's")
	assert.Equal(t, manifest.BuildModeImage, model.draft.Build.Mode, "the file's image, not the Dockerfile found")
	assert.Equal(t, 9090, model.draft.Port, "the file's port, not the EXPOSE")
	require.Equal(t, screenConfirm, model.at)
	assert.Contains(t, string(model.content), "imageUri: registry/app:v7")
	assert.NotContains(t, string(model.content), "source: provided")
}

// A secret-looking literal in the file is named as the file's, since no
// workload declared it and nothing is being bound.
func TestRun_SpecFileWarnsAboutItsOwnSecretLiterals(t *testing.T) {
	dir := t.TempDir()
	spec := strings.Replace(workloadSpec, "            imageUri: registry/app:v7\n",
		"            imageUri: registry/app:v7\n            environmentVars:\n"+
			"              - name: OPENAI_API_KEY\n                value: sk-abcdefghijklmnopqrstuvwxyz012345\n", 1)
	path := writeSpec(t, dir, spec)

	opts := withSpec(dir, path, Answers{})

	_, err := Run(opts)
	require.NoError(t, err)

	stderr := opts.Stderr.(*bytes.Buffer).String()
	assert.Contains(t, stderr, "the spec file "+path+" declares 1 variable")
	assert.Contains(t, stderr, "the manifest copies it")
	assert.NotContains(t, stderr, "binding copies it")
}

// From a deploy the bare "pass --name" was a dead end: up refuses --name.
// The refusal names the command that takes it, with the file.
func TestRun_SpecFileWithoutANameNamesTheConfigCommand(t *testing.T) {
	dir := t.TempDir()
	spec := writeSpec(t, dir, artifactSpec)

	_, err := Run(withSpec(dir, spec, Answers{}))
	require.ErrorIs(t, err, ErrSpecFileUnnamed)
	assert.Contains(t, err.Error(), "dr workload config --spec-file "+spec+" --name <name>")
}

// The build check set on entry is cleared by the first keystroke, so it is
// asked again on the way to confirm: typing a name and pressing Enter through
// the screens must not write a manifest the platform cannot build.
func TestFlow_PreparedSpecBuildCheckSurvivesTheKeystrokes(t *testing.T) {
	dir := t.TempDir()
	spec := strings.Replace(artifactSpec, "imageUri: registry/app:v7",
		"imageBuildConfig: {dockerfile: {source: generated, executionEnvironmentId: a, executionEnvironmentVersionId: b, entrypoint: [python, app.py]}}", 1)

	prepared, err := LoadSpec(writeSpec(t, dir, spec))
	require.NoError(t, err)

	model := newFlow(Detect(dir), nil, Answers{}).withPrepared(prepared)
	require.Error(t, model.failed, "said on entry")

	model = press(t, pastName(t, model), "enter")
	require.Equal(t, screenConfirm, model.at)
	require.Error(t, model.failed, "and said again on confirm, where it counts")
	assert.Contains(t, model.failed.Error(), "cannot be made from")

	model = press(t, model, "enter")
	assert.False(t, model.done, "a build the platform cannot make is not confirmed")
}

// On a terminal the flag checks headless applies are applied before the
// wizard opens: the screens that would show a bad importance are the ones
// the file skips.
func TestRunInteractiveFlow_SpecFileRefusesTheFlagsHeadlessRefuses(t *testing.T) {
	dir := t.TempDir()

	_, _, _, err := runInteractiveFlow(
		Options{Dir: dir, SpecFile: writeSpec(t, dir, workloadSpec), Answers: Answers{Importance: "bogus"}},
		Detect(dir))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `--importance "bogus"`)
}

// A spec that fails the ledger is refused before any secret is stored: a
// credential created for a file that is then refused would outlive the run
// with nothing pointing at it.
func TestRun_SpecFileIsValidatedBeforeSecretsAreStored(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, EnvFileName), []byte("OPENAI_API_KEY=sk-abcdefghijklmnopqrstuvwxyz012345\n"), 0o600))

	original := createCredentialFn
	createCredentialFn = func(name, _ string) (*workload.Credential, error) {
		t.Fatalf("credential %s was stored for a spec the run then refused", name)

		return nil, nil
	}

	t.Cleanup(func() { createCredentialFn = original })

	spec := writeSpec(t, dir, strings.Replace(artifactSpec, "port: 9090", "port: 80", 1))

	_, err := Run(withSpec(dir, spec, Answers{Name: "my-app"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "port")
	assert.NoFileExists(t, manifest.Path(dir))
}

// The spec file has no name to print, so the probe lines say whose probe it
// is in words rather than leading with an empty name.
func TestFlow_PreparedSpecProbeLinesNameTheFile(t *testing.T) {
	dir := t.TempDir()
	spec := strings.Replace(artifactSpec, "            path: /healthz\n", "", 1)

	prepared, err := LoadSpec(writeSpec(t, dir, spec))
	require.NoError(t, err)

	model := newFlow(Detect(dir), nil, Answers{}).withPrepared(prepared)
	model = press(t, pastName(t, model), "enter")
	require.Equal(t, screenConfirm, model.at)

	assert.Contains(t, model.View(), "Readiness: the spec file's own probe")
	assert.NotContains(t, model.View(), "Readiness: 's own probe")
}

// An image build syncs nothing, so the warning about everything in a suspect
// directory being uploaded does not apply to an image spec.
func TestRun_ImageSpecInAnEmptyDirectoryDoesNotWarnAboutTheUpload(t *testing.T) {
	dir := t.TempDir()
	opts := withSpec(dir, writeSpec(t, dir, artifactSpec), Answers{Name: "my-app"})

	_, err := Run(opts)
	require.NoError(t, err)
	assert.NotContains(t, opts.Stderr.(*bytes.Buffer).String(), "would be uploaded")
}
