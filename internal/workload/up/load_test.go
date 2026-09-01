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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/datarobot/cli/internal/workload/wizard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// boundManifest is a complete, valid file carrying both credential value
// forms, so compilation has something real to expand.
const boundManifest = `workloadId: 68b0aaaa0000000000000001
name: my-app
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
            imageUri: nginx:latest
            environmentVars:
              - name: LOG_LEVEL
                value: debug
              - name: OPENAI_API_KEY
                value: dr-credential:68f0cccc0000000000000003/apiToken
runtime:
  containerGroups:
    - name: default
      replicaCount: 2
      containers:
        - name: primary
          resourceAllocation:
            cpu: 0.5
            memory: 512MB
`

// writeManifest drops content at dir/.datarobot.yaml and returns the path.
func writeManifest(t *testing.T, dir, content string) string {
	t.Helper()

	path := filepath.Join(dir, manifest.FileName)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

func TestLoad_ReadsValidatesAndCompiles(t *testing.T) {
	dir := t.TempDir()
	path := writeManifest(t, dir, boundManifest)

	loaded, err := Load(dir)
	require.NoError(t, err)

	assert.Equal(t, path, loaded.Path)
	assert.Equal(t, dir, loaded.ProjectDir)
	assert.Equal(t, "68b0aaaa0000000000000001", loaded.WorkloadID())
	assert.Equal(t, "my-app", loaded.Manifest.Name())
	require.Len(t, loaded.Compiled.CredentialRefs, 1)
	assert.Equal(t, "68f0cccc0000000000000003", loaded.Compiled.CredentialRefs[0].CredentialID)
}

// TestLoad_WalksUpward is why ProjectDir exists as a separate field. Running
// `up` from a subdirectory has to deploy the whole project, not the fragment
// the shell happened to be in.
func TestLoad_WalksUpward(t *testing.T) {
	root := t.TempDir()
	writeManifest(t, root, boundManifest)

	nested := filepath.Join(root, "services", "api")
	require.NoError(t, os.MkdirAll(nested, 0o750))

	loaded, err := Load(nested)
	require.NoError(t, err)
	assert.Equal(t, root, loaded.ProjectDir, "the project root is where the manifest is, not where up was run")
}

// TestLoad_MissingIsASentinel keeps the wizard-or-error decision with the
// command. Returning a plain error here would force every caller to match on
// a message.
func TestLoad_MissingIsASentinel(t *testing.T) {
	_, err := Load(t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNoManifest)
}

// TestLoad_InvalidCarriesLineNumbers is the whole reason validation runs
// before anything touches the network: the user gets the file's own
// coordinates, in milliseconds, instead of a server rejection after a build.
func TestLoad_InvalidCarriesLineNumbers(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, `name: my-app
artifact:
  name: my-app-artifact
  spec:
    type: service
    containerGroups:
      - name: default
        containers:
          - name: primary
            primary: true
            port: 80
            imageUri: nginx:latest
runtime:
  containerGroups:
    - name: default
      replicaCount: 1
      containers:
        - name: primary
          resourceAllocation: {cpu: 0.5, memory: 512MB}
`)

	_, err := Load(dir)
	require.Error(t, err)

	var invalid *manifest.ValidationError

	require.ErrorAs(t, err, &invalid, "a bad manifest must arrive as the line-numbered type")
	require.NotEmpty(t, invalid.Errors)
	assert.Positive(t, invalid.Errors[0].Line, "every finding names the line it sits on")
}

func TestLoad_ForeignFileIsRejected(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "version: 3\nservices:\n  web:\n    image: nginx\n")

	_, err := Load(dir)
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNoManifest, "a file that is there but wrong is not the same as no file")
}

// TestLoaded_SpecAndRuntimeComeFromTheCompiledPayload is the pairing that
// makes drift detection fair. Reading the spec off the raw file would leave
// the credential shorthand unexpanded, and every bound workload would then
// report a permanent, unfixable difference on that one variable.
func TestLoaded_SpecAndRuntimeComeFromTheCompiledPayload(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, boundManifest)

	loaded, err := Load(dir)
	require.NoError(t, err)

	spec, err := loaded.Spec()
	require.NoError(t, err)
	assert.NotContains(t, spec, "type",
		"the live spec never carries a type, so comparing one reports drift no deploy can settle")

	kind, err := loaded.ArtifactType()
	require.NoError(t, err)
	assert.Equal(t, "service", kind, "the file said it inside the spec, which is where it has to be read from")

	groups, _ := spec["containerGroups"].([]any)
	require.Len(t, groups, 1)

	group, _ := groups[0].(map[string]any)
	containers, _ := group["containers"].([]any)
	require.Len(t, containers, 1)

	container, _ := containers[0].(map[string]any)
	vars, _ := container["environmentVars"].([]any)
	require.Len(t, vars, 2)

	secret, _ := vars[1].(map[string]any)
	assert.Equal(t, "dr-credential", secret["source"])
	assert.Equal(t, "68f0cccc0000000000000003", secret["drCredentialId"])
	assert.Equal(t, "apiToken", secret["key"])
	assert.NotContains(t, secret, "value", "the shorthand is replaced, not kept alongside")

	runtime, err := loaded.Runtime()
	require.NoError(t, err)

	runtimeGroups, _ := runtime["containerGroups"].([]any)
	require.Len(t, runtimeGroups, 1)
}

// TestLoaded_WorkloadIDIsStrippedFromThePayload: the binding is the CLI's own
// key and the platform rejects it, so it has to be lifted out of the request
// while staying readable on Loaded.
func TestLoaded_WorkloadIDIsStrippedFromThePayload(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, boundManifest)

	loaded, err := Load(dir)
	require.NoError(t, err)

	assert.Equal(t, "68b0aaaa0000000000000001", loaded.WorkloadID())
	assert.NotContains(t, string(loaded.Compiled.Payload), "workloadId")
}

func TestLoaded_UnboundManifestHasNoWorkloadID(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, `name: my-app
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
runtime:
  containerGroups:
    - name: default
      replicaCount: 1
      containers:
        - name: primary
          resourceAllocation: {cpu: 0.5, memory: 512MB}
`)

	loaded, err := Load(dir)
	require.NoError(t, err)
	assert.Empty(t, loaded.WorkloadID())
}

func TestLoaded_PayloadDecodeFailureIsReported(t *testing.T) {
	broken := Loaded{Compiled: &manifest.Compiled{Payload: []byte("{not json")}}

	_, err := broken.Spec()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compiled manifest")

	_, err = broken.Runtime()
	require.Error(t, err)
}

// TestLoad_ADirectoryAtThePathIsNotAManifest documents a surprise worth
// knowing about: Locate tests for a regular file, so a directory with the
// manifest's name is stepped over and the walk carries on upward. The effect
// is that `up` reports no manifest rather than an unreadable one. That is the
// manifest package's choice and the safer of the two, since it keeps a stray
// directory from breaking a project whose real manifest is one level up.
func TestLoad_ADirectoryAtThePathIsNotAManifest(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, manifest.FileName), 0o750))

	_, err := Load(dir)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNoManifest)
}

// The flag exists because the first run of this command already reads .env:
// with no manifest `up` is the wizard. The edit has to happen before the file
// is read, or the deploy would carry the version from before it.
func TestLoad_ImportEnvEditsTheManifestBeforeReadingIt(t *testing.T) {
	dir := t.TempDir()
	path := writeManifest(t, dir, boundManifest)

	swap(t, &runWizardFn, func(opts wizard.Options) (wizard.Result, error) {
		assert.True(t, opts.ImportEnv)
		assert.Equal(t, dir, opts.Dir, "the edit lands next to the manifest being deployed")

		// What the real wizard does: the variable is in the file by the time
		// it returns.
		edited := strings.Replace(boundManifest,
			"              - name: LOG_LEVEL",
			"              - name: REGION\n                value: eu-west-1\n              - name: LOG_LEVEL", 1)
		require.NoError(t, os.WriteFile(path, []byte(edited), 0o600))

		return wizard.Result{Path: path, Action: wizard.ActionUpdated, EnvKeysListed: 1}, nil
	})

	loaded, err := load(dir, Options{ImportEnv: true})
	require.NoError(t, err)

	assert.Equal(t, 1, loaded.Env.KeysAdded)
	assert.Contains(t, loaded.Manifest.EnvVarNames(), "REGION",
		"the deploy plans from the file as edited, not as it was")
}

// A rotation writes to the credential store and leaves the file alone, so the
// count is the only trace of it: the plan below has nothing to show.
func TestLoad_UpdateEnvCarriesTheRotationCount(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, boundManifest)

	swap(t, &runWizardFn, func(opts wizard.Options) (wizard.Result, error) {
		assert.True(t, opts.UpdateEnv)

		return wizard.Result{Action: wizard.ActionUnchanged, EnvSecretsRotated: 2}, nil
	})

	loaded, err := load(dir, Options{UpdateEnv: true})
	require.NoError(t, err)

	assert.Equal(t, 2, loaded.Env.SecretsRotated)
	assert.Equal(t, 0, loaded.Env.KeysAdded)
}

// Without the flags nothing reads .env, which is what keeps a deploy a
// function of the committed repo: a fresh CI clone has no .env at all.
func TestLoad_WithoutTheFlagsTheWizardIsNotRun(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, boundManifest)

	swap(t, &runWizardFn, func(wizard.Options) (wizard.Result, error) {
		t.Fatal("the deploy read .env without being asked to")

		return wizard.Result{}, nil
	})

	_, err := load(dir, Options{})
	require.NoError(t, err)
}

// A dry run promises the file is not touched, and an import that minted a
// credential and rewrote the manifest would have changed two things the plan
// then says it is not going to do.
func TestLoad_DryRunPreviewsTheEdit(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, boundManifest)

	swap(t, &runWizardFn, func(opts wizard.Options) (wizard.Result, error) {
		assert.True(t, opts.DryRun, "the preview must reach the wizard, or it writes")

		return wizard.Result{Action: wizard.ActionPlanned, EnvKeysListed: 1}, nil
	})

	loaded, err := load(dir, Options{ImportEnv: true, DryRun: true})
	require.NoError(t, err)

	assert.Equal(t, 1, loaded.Env.KeysAdded)
	assert.NotContains(t, loaded.Manifest.EnvVarNames(), "REGION")
}
