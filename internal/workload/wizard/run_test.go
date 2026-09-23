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
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// headless is a run that never prompts, which is what every test here wants:
// the terminal path is exercised through the flow model instead.
func headless(dir string, answers Answers) Options {
	return Options{Dir: dir, NonInteractive: true, Answers: answers, Stderr: &bytes.Buffer{}}
}

// stubLive points the binding path at fixed documents.
func stubLive(t *testing.T, workloadDoc, artifactDoc workload.Document) {
	t.Helper()

	swapLiveFns(t,
		func(string) (workload.Document, error) { return workloadDoc, nil },
		func(string) (workload.Document, error) { return artifactDoc, nil })
}

// stubLiveErr makes the workload fetch fail.
func stubLiveErr(t *testing.T, err error) {
	t.Helper()

	swapLiveFns(t,
		func(string) (workload.Document, error) { return nil, err },
		func(string) (workload.Document, error) { return nil, nil })
}

func swapLiveFns(t *testing.T, getWorkload, getArtifact func(string) (workload.Document, error)) {
	t.Helper()

	originalWorkload, originalArtifact := getWorkloadFn, getArtifactFn

	getWorkloadFn, getArtifactFn = getWorkload, getArtifact

	t.Cleanup(func() { getWorkloadFn, getArtifactFn = originalWorkload, originalArtifact })
}

// Headless there is no screen to explain that a probe is being preserved, so a
// flag that cannot be carried out is an error rather than a run that reports
// success having ignored the one thing it was asked to change.
func TestRun_ProbeFlagOnAnUnmodelledProbeIsRefused(t *testing.T) {
	stubLive(t, documentFrom(t, `{"name": "tcp-app", "artifactId": "68a1",
			"runtime": {"containerGroups": [{"name": "default", "replicaCount": 1,
				"containers": [{"name": "primary", "resourceAllocation": {"cpu": 1, "memory": "2GB"}}]}]}}`),
		documentFrom(t, `{"name": "tcp-app-artifact", "type": "service", "spec": {"containerGroups": [{"name": "default", "containers": [
				{"name": "primary", "primary": true, "port": 9100, "imageUri": "registry/tcp:v1",
				 "readinessProbe": {"tcpSocket": {"port": 9100}, "periodSeconds": 10}}]}]}}`))

	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\n")

	for name, answers := range map[string]Answers{
		"declined": {WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0", NoProbe: true},
		"a path":   {WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0", HealthPath: "/ready"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Run(Options{Dir: dir, NonInteractive: true, Answers: answers})

			require.Error(t, err)
			assert.Contains(t, err.Error(), "readiness probe with no path")
			assert.NoFileExists(t, manifest.Path(dir))
		})
	}

	// Binding without saying anything about the probe still works, and keeps it.
	result, err := Run(Options{
		Dir: dir, NonInteractive: true,
		Answers: Answers{WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0"},
		DryRun:  true,
	})
	require.NoError(t, err)
	assert.Contains(t, string(result.Content), "tcpSocket")
}

func documentFrom(t *testing.T, raw string) workload.Document {
	t.Helper()

	var doc workload.Document

	require.NoError(t, json.Unmarshal([]byte(raw), &doc))

	return doc
}

func TestRun_WritesTheManifest(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\nEXPOSE 3000\n")

	result, err := Run(headless(dir, Answers{Name: "my-app"}))
	require.NoError(t, err)

	assert.Equal(t, ActionCreated, result.Action)
	assert.Equal(t, manifest.Path(dir), result.Path)

	written, err := os.ReadFile(result.Path)
	require.NoError(t, err)
	assert.Equal(t, string(result.Content), string(written))

	// What lands on disk is what the reader accepts: this is the property the
	// whole command exists to hold.
	parsed, err := manifest.Load(result.Path)
	require.NoError(t, err)
	require.NoError(t, parsed.Validate())

	assert.Equal(t, "my-app", parsed.Name())
	assert.Empty(t, parsed.WorkloadID())
	assert.Contains(t, string(written), "port: 3000")
}

func TestRun_DryRunWritesNothing(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\n")

	opts := headless(dir, Answers{Name: "my-app"})
	opts.DryRun = true

	result, err := Run(opts)
	require.NoError(t, err)

	assert.NotEmpty(t, result.Content)
	assert.NoFileExists(t, result.Path)
	// The action is what a script keys on to tell a plan from a file that now
	// exists, so a dry run reporting ActionCreated would be a silent lie.
	assert.Equal(t, ActionPlanned, result.Action)
}

// Setup is a one-time act. A configured project gets a pointer at its file,
// not a second opinion about what should be in it.
func TestRun_ExistingManifestIsLeftAlone(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\n")
	path := manifest.Path(dir)

	original := "name: hand-written\nartifactId: 68b0bbbb0000000000000002\n"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o600))

	result, err := Run(headless(dir, Answers{Name: "something-else"}))
	require.NoError(t, err)

	assert.Equal(t, ActionUnchanged, result.Action)
	assert.Equal(t, path, result.Path)
	assert.Empty(t, result.Content)

	// The answer describes the project as it stands, not as the flags would
	// have configured it.
	assert.Equal(t, "hand-written", result.Draft.Name)

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(after))
}

// Reporting an existing manifest means reading it, so a bound project is not
// reported as one that will create a workload on the first up.
func TestRun_ExistingManifestIsReadBack(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\n")

	first, err := Run(headless(dir, Answers{Name: "my-app"}))
	require.NoError(t, err)
	require.NoError(t, manifest.WriteWorkloadID(first.Path, "68b0ffff0000000000000009"))

	again, err := Run(headless(dir, Answers{}))
	require.NoError(t, err)

	assert.Equal(t, ActionUnchanged, again.Action)
	assert.Equal(t, "my-app", again.Draft.Name)
	assert.Equal(t, "68b0ffff0000000000000009", again.Draft.WorkloadID)
	assert.Equal(t, manifest.BuildModeDockerfile, again.Draft.Build.Mode)
}

// A manifest that no longer validates is the news, not the fact that a file
// exists: up would fail on it too, and this way the line number arrives now.
func TestRun_ExistingManifestThatDoesNotValidate(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\n")

	require.NoError(t, os.WriteFile(manifest.Path(dir), []byte(`name: broken
artifact:
  name: broken-artifact
  type: service
  spec:
    containerGroups:
      - name: default
        containers:
          - name: primary
            primary: true
            port: 80
            imageUri: nginx:latest
`), 0o600))

	_, err := Run(headless(dir, Answers{}))
	require.Error(t, err)

	assert.Contains(t, err.Error(), "line 11")
	assert.Contains(t, err.Error(), "must listen on 1024 or above")
}

// Two manifests on one upward walk mean a deploy reads a different file
// depending on where it was run, so the shadowed one is named.
func TestRun_WarnsAboutAManifestAbove(t *testing.T) {
	parent := t.TempDir()
	require.NoError(t, os.WriteFile(manifest.Path(parent), []byte("name: parent-app\nartifactId: 68b0\n"), 0o600))

	child := filepath.Join(parent, "service")
	require.NoError(t, os.Mkdir(child, 0o755))
	writeDockerfile(t, child, "FROM scratch\n")

	stderr := &bytes.Buffer{}
	opts := headless(child, Answers{Name: "child-app"})
	opts.Stderr = stderr

	_, err := Run(opts)
	require.NoError(t, err)

	assert.Contains(t, stderr.String(), manifest.Path(parent))
	assert.Contains(t, stderr.String(), "reads a different file")
}

// A parent that is not a directory has no manifest in it, so there is nothing
// to warn about. Locate refuses to start the walk and names the ancestor it was
// handed, so the warning this used to print ended with a path the reader never
// typed while opening with the one they did.
func TestWarnShadowedManifest_SaysNothingWhenTheWalkCannotStart(t *testing.T) {
	stderr := &bytes.Buffer{}

	warnShadowedManifest(stderr, filepath.Join(t.TempDir(), "missing", "project"))

	assert.Empty(t, stderr.String())
}

func TestRun_NoWarningWithoutAManifestAbove(t *testing.T) {
	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\n")

	stderr := &bytes.Buffer{}
	opts := headless(dir, Answers{Name: "my-app"})
	opts.Stderr = stderr

	_, err := Run(opts)
	require.NoError(t, err)

	assert.Empty(t, stderr.String())
}

func TestRun_ReportsMissingAnswers(t *testing.T) {
	dir := t.TempDir()

	_, err := Run(headless(dir, Answers{Name: "my-app"}))
	require.Error(t, err)

	assert.Contains(t, err.Error(), "--build-mode")
	assert.NoFileExists(t, manifest.Path(dir))
}

// Binding headlessly is the same operation the picker performs: the live spec
// is downloaded and the file is written from it.
func TestRun_BindsToALiveWorkload(t *testing.T) {
	stubLive(t,
		documentFrom(t, `{"name": "live-app", "importance": "high", "artifactId": "68a1",
			"runtime": {"containerGroups": [{"name": "default", "autoscaling": {"enabled": true, "maxReplicaCount": 9},
				"containers": [{"name": "primary", "resourceAllocation": {"cpu": 2, "memory": "4GB"}}]}]}}`),
		documentFrom(t, `{"name": "live-app-artifact", "type": "service", "spec": {"containerGroups": [{"name": "default", "containers": [
				{"name": "primary", "primary": true, "port": 8000, "imageUri": "registry/live:v9"}]}]}}`))

	dir := t.TempDir()

	result, err := Run(headless(dir, Answers{WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0"}))
	require.NoError(t, err)

	written := string(result.Content)

	assert.Contains(t, written, "workloadId: 68b0c1d2e3f4a5b6c7d8e9f0")
	assert.Contains(t, written, "name: live-app")
	assert.Contains(t, written, "importance: high")
	// The sizing the wizard has no question for survives the round trip.
	assert.Contains(t, written, "maxReplicaCount: 9")
	assert.Contains(t, written, "memory: 4GB")

	parsed, err := manifest.Load(result.Path)
	require.NoError(t, err)
	require.NoError(t, parsed.Validate())
	assert.Equal(t, "68b0c1d2e3f4a5b6c7d8e9f0", parsed.WorkloadID())
}

// A workload id that does not resolve fails at setup, which is the whole
// point of resolving it here instead of at deploy time.
func TestRun_UnknownWorkloadID(t *testing.T) {
	stubLiveErr(t, errors.New("404 not found"))

	_, err := Run(headless(t.TempDir(), Answers{WorkloadID: "nope"}))
	require.Error(t, err)

	assert.Contains(t, err.Error(), "cannot bind to workload nope")
}

// boundWithLiveEnv stubs a workload whose artifact declares the given
// environmentVars JSON, which is the block binding copies verbatim.
func boundWithLiveEnv(t *testing.T, envVars string) {
	t.Helper()

	stubLive(t,
		documentFrom(t, `{"name": "live-app", "importance": "high", "artifactId": "68a1",
			"runtime": {"containerGroups": [{"name": "default", "replicaCount": 1,
				"containers": [{"name": "primary", "resourceAllocation": {"cpu": 1, "memory": "1GB"}}]}]}}`),
		documentFrom(t, `{"name": "live-app-artifact", "type": "service", "spec": {"containerGroups": [{"name": "default", "containers": [
				{"name": "primary", "primary": true, "port": 8000, "imageUri": "registry/live:v9",
				 "environmentVars": `+envVars+`}]}]}}`))
}

// boundRun binds headlessly and returns what reached stderr.
func boundRun(t *testing.T) string {
	t.Helper()

	stderr := &bytes.Buffer{}
	opts := headless(t.TempDir(), Answers{WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0"})
	opts.Stderr = stderr

	_, err := Run(opts)
	require.NoError(t, err)

	return stderr.String()
}

// Binding preserves the live spec, so a secret the platform holds in the clear
// is copied into a file meant to be committed. Nothing can fix that
// automatically, and headlessly no screen shows the file before it is written,
// so the warning is the only thing standing between the two.
func TestRun_BoundWorkloadWarnsAboutLiveSecretLiterals(t *testing.T) {
	boundWithLiveEnv(t, `[{"name": "LOG_LEVEL", "value": "debug"},
		{"name": "OPENAI_API_KEY", "value": "sk-live-51H8xQvZmNpKdRt7YwLbG3JfA9"},
		{"name": "DATABASE_URL", "value": "postgres://carol:hunter2@db.example.com:5432/app"}]`)

	stderr := boundRun(t)

	assert.Contains(t, stderr, "Warning: live-app declares 2 variables whose value looks like a secret")
	assert.Contains(t, stderr, "OPENAI_API_KEY (looks like an OpenAI-style API key)")
	assert.Contains(t, stderr, "DATABASE_URL (a URL with carol's password in it)")
	assert.Contains(t, stderr, manifest.CredentialShorthandPrefix)

	// The warning exists because these values must not spread, so it cannot be
	// the thing that spreads them.
	assert.NotContains(t, stderr, "sk-live-51H8xQvZmNpKdRt7YwLbG3JfA9")
	assert.NotContains(t, stderr, "hunter2")

	// An ordinary setting is not a finding: a warning that fires on LOG_LEVEL
	// is one the reader learns to skip.
	assert.NotContains(t, stderr, "LOG_LEVEL")
}

// The name test and the entropy test are deliberately not used here. Both are
// calibrated for a screen that can be corrected in one keystroke, and this
// warning has no such screen behind it.
func TestRun_BoundWorkloadDoesNotGuessAtLiveValues(t *testing.T) {
	boundWithLiveEnv(t, `[{"name": "API_URL", "value": "https://api.example.com/v2"},
		{"name": "BUILD_ID", "value": "a7f3c9d1e5b2f8a4c6d0"},
		{"name": "SESSION_TOKEN", "value": "short"}]`)

	assert.NotContains(t, boundRun(t), "looks like a secret")
}

// A value the platform already keeps in the credential store is not in the
// clear, in either spelling, so neither is a finding.
func TestRun_BoundWorkloadIgnoresCredentialReferences(t *testing.T) {
	boundWithLiveEnv(t, `[{"name": "SHORTHAND_KEY", "value": "dr-credential:68f0cccc0000000000000003/apiToken"},
		{"name": "OBJECT_KEY", "source": "dr-credential",
		 "drCredentialId": "68f0cccc0000000000000003", "key": "apiToken"}]`)

	assert.NotContains(t, boundRun(t), "looks like a secret")
}

func TestRun_BoundWorkloadWithoutAnArtifact(t *testing.T) {
	stubLive(t, documentFrom(t, `{"name": "live-app"}`), nil)

	_, err := Run(headless(t.TempDir(), Answers{WorkloadID: "68b0"}))
	require.Error(t, err)

	assert.Contains(t, err.Error(), "no artifact to copy")
}

// Cancelling writes nothing and says so, so `config && up` stops here.
func TestRun_CancelledWritesNothing(t *testing.T) {
	original := runInteractiveFlowFn
	runInteractiveFlowFn = func(Options, Detected) ([]byte, manifest.Draft, string, error) {
		return nil, manifest.Draft{}, "", ErrCancelled
	}

	t.Cleanup(func() { runInteractiveFlowFn = original })

	originalTerminal := isStdinTerminalFn
	isStdinTerminalFn = func() bool { return true }

	t.Cleanup(func() { isStdinTerminalFn = originalTerminal })

	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\n")

	_, err := Run(Options{Dir: dir, Stderr: &bytes.Buffer{}})
	require.ErrorIs(t, err, ErrCancelled)

	assert.NoFileExists(t, manifest.Path(dir))
}

// Without a terminal nothing prompts, even when --yes was not passed.
func TestRun_NoTerminalNeverPrompts(t *testing.T) {
	originalFlow := runInteractiveFlowFn
	runInteractiveFlowFn = func(Options, Detected) ([]byte, manifest.Draft, string, error) {
		t.Fatal("the wizard must not run without a terminal")

		return nil, manifest.Draft{}, "", nil
	}

	t.Cleanup(func() { runInteractiveFlowFn = originalFlow })

	originalTerminal := isStdinTerminalFn
	isStdinTerminalFn = func() bool { return false }

	t.Cleanup(func() { isStdinTerminalFn = originalTerminal })

	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\n")

	_, err := Run(Options{Dir: dir, Answers: Answers{Name: "my-app"}, Stderr: &bytes.Buffer{}})
	require.NoError(t, err)
}

// writeEnvFile puts a .env in dir and hands back the directory, so a test
// reads as one line of setup.
func writeEnvFile(t *testing.T, dir, content string) string {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, EnvFileName), []byte(content), 0o600))

	return dir
}

// liveGeneratedAgent is a running agent built from an execution environment,
// which is the case where binding has live values worth keeping.
func liveGeneratedAgent(t *testing.T) {
	t.Helper()
	stubLive(t,
		documentFrom(t, `{"name": "live-agent", "importance": "low", "artifactId": "68a1",
			"runtime": {"containerGroups": [{"name": "default", "replicaCount": 1,
				"containers": [{"name": "primary", "resourceAllocation": {"cpu": 1, "memory": "2GB"}}]}]}}`),
		documentFrom(t, `{"name": "live-agent-artifact", "type": "agent", "spec": {"containerGroups": [{"name": "default", "containers": [
				{"name": "primary", "primary": true, "port": 8000,
				 "imageBuildConfig": {"dockerfile": {"source": "generated",
				   "entrypoint": ["python", "old.py"],
				   "executionEnvironmentId": "68e1", "executionEnvironmentVersionId": "68e2"}}}]}]}}`))
}

// Staying on the mode the workload already uses is an edit to that build, so
// the flags that were not passed keep their live values. Demanding
// --execution-environment again to change the entrypoint would contradict the
// rest of binding, where an omitted field means "leave it alone".
func TestRun_BoundBuildKeepsWhatTheFlagsDidNotMention(t *testing.T) {
	liveGeneratedAgent(t)

	result, err := Run(headless(t.TempDir(), Answers{
		WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0",
		Entrypoint: "python new.py",
	}))
	require.NoError(t, err)

	written := string(result.Content)

	assert.Contains(t, written, "- new.py")
	assert.NotContains(t, written, "old.py")
	assert.Contains(t, written, `executionEnvironmentId: "68e1"`)
	assert.Contains(t, written, `executionEnvironmentVersionId: "68e2"`)
	assert.Contains(t, written, "source: generated")
}

// Switching modes is the other thing entirely: nothing in the old build
// answers for the new one, so the new mode's own flags are required.
func TestRun_BoundBuildModeSwitchNeedsItsOwnFlags(t *testing.T) {
	liveGeneratedAgent(t)

	_, err := Run(headless(t.TempDir(), Answers{WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0", BuildMode: "image"}))
	require.Error(t, err)

	assert.Contains(t, err.Error(), "--image is required")
}

func TestRun_BoundBuildModeSwitchReplacesTheBuild(t *testing.T) {
	liveGeneratedAgent(t)

	result, err := Run(headless(t.TempDir(), Answers{
		WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0",
		BuildMode:  "image",
		Image:      "registry/new:v1",
	}))
	require.NoError(t, err)

	written := string(result.Content)

	assert.Contains(t, written, "imageUri: registry/new:v1")
	assert.NotContains(t, written, "imageBuildConfig")
}

// --a2a-enabled is judged against the kind that will be written. For a bound
// agent that is the live kind, so the caller does not have to repeat --type.
func TestRun_BoundAgentTakesA2AWithoutRepeatingTheType(t *testing.T) {
	liveGeneratedAgent(t)

	result, err := Run(headless(t.TempDir(), Answers{
		WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0",
		A2AEnabled: true,
	}))
	require.NoError(t, err)

	assert.Contains(t, string(result.Content), "a2aEnabled: true")
}

// A service is still a service, bound or not.
func TestRun_BoundServiceRejectsA2A(t *testing.T) {
	stubLive(t,
		documentFrom(t, `{"name": "live-app", "importance": "low", "artifactId": "68a1",
			"runtime": {"containerGroups": [{"name": "default", "replicaCount": 1,
				"containers": [{"name": "primary", "resourceAllocation": {"cpu": 1, "memory": "2GB"}}]}]}}`),
		documentFrom(t, `{"name": "live-app-artifact", "type": "service", "spec": {"containerGroups": [{"name": "default", "containers": [
				{"name": "primary", "primary": true, "port": 8000, "imageUri": "registry/live:v9"}]}]}}`))

	_, err := Run(headless(t.TempDir(), Answers{WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0", A2AEnabled: true}))
	require.Error(t, err)

	assert.Contains(t, err.Error(), "--a2a-enabled applies to --type agent")
}

// The summary counts what reached the file. A name the workload already
// declares keeps its running value, so claiming it was added would describe a
// run that did not happen.
func TestRun_BoundEnvCountsOnlyWhatWasAdded(t *testing.T) {
	stubLive(t,
		documentFrom(t, `{"name": "live-app", "importance": "low", "artifactId": "68a1",
			"runtime": {"containerGroups": [{"name": "default", "replicaCount": 1,
				"containers": [{"name": "primary", "resourceAllocation": {"cpu": 1, "memory": "2GB"}}]}]}}`),
		documentFrom(t, `{"name": "live-app-artifact", "type": "service", "spec": {"containerGroups": [{"name": "default", "containers": [
				{"name": "primary", "primary": true, "port": 8000, "imageUri": "registry/live:v9",
				 "environmentVars": [{"name": "LOG_LEVEL", "value": "info"},
				                     {"name": "OPENAI_API_KEY", "value": "live"}]}]}]}}`))

	dir := writeEnvFile(t, t.TempDir(),
		"LOG_LEVEL=debug\nOPENAI_API_KEY=sk-abcdefghijklmnopqrstuvwxyz012345\nFEATURE_X=on\n")

	result, err := Run(headless(dir, Answers{WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0"}))
	require.NoError(t, err)

	assert.Equal(t, 1, result.EnvKeysListed)
	assert.Equal(t, 0, result.EnvSecretsPending)

	written := string(result.Content)

	assert.Contains(t, written, "FEATURE_X")
	// The live values stand: .env is the developer's copy and may be stale.
	assert.Contains(t, written, "value: info")
	assert.Contains(t, written, "value: live")
}

// A .env that cannot be parsed is the one case where saying nothing is worst:
// the manifest looks complete and the container starts without its
// configuration.
func TestRun_WarnsAboutAnUnreadableEnvFile(t *testing.T) {
	dir := writeEnvFile(t, writeDockerfile(t, t.TempDir(), "FROM scratch\n"),
		"LOG_LEVEL=debug\n\"unterminated\n")

	stderr := &bytes.Buffer{}
	opts := headless(dir, Answers{Name: "my-app"})
	opts.Stderr = stderr

	result, err := Run(opts)
	require.NoError(t, err)

	assert.Contains(t, stderr.String(), EnvFileName)
	assert.Contains(t, stderr.String(), "Nothing from it was imported")
	assert.Equal(t, 0, result.EnvKeysListed)
	assert.NotContains(t, string(result.Content), "environmentVars")
}

// Non-finite is not a quantity of cores. Both parse as float64, and neither
// compares as negative, so the sizing check has to name them.
func TestRun_RejectsNonFiniteCPU(t *testing.T) {
	for name, cpu := range map[string]float64{
		"NaN":  math.NaN(),
		"+Inf": math.Inf(1),
		"-Inf": math.Inf(-1),
	} {
		t.Run(name, func(t *testing.T) {
			dir := writeDockerfile(t, t.TempDir(), "FROM scratch\n")

			_, err := Run(headless(dir, Answers{Name: "my-app", CPU: cpu}))
			require.Error(t, err)

			assert.Contains(t, err.Error(), "--cpu")
			assert.NoFileExists(t, manifest.Path(dir))
		})
	}
}

// Declining the import means the file is not the wizard's business, broken or
// not.
func TestRun_SkipEnvSilencesTheUnreadableEnvWarning(t *testing.T) {
	dir := writeEnvFile(t, writeDockerfile(t, t.TempDir(), "FROM scratch\n"),
		"LOG_LEVEL=debug\n\"unterminated\n")

	stderr := &bytes.Buffer{}
	opts := headless(dir, Answers{Name: "my-app", SkipEnv: true})
	opts.Stderr = stderr

	_, err := Run(opts)
	require.NoError(t, err)

	assert.Empty(t, stderr.String())
}

// Headless can warn but not offer: the directory check becomes a stderr line
// naming the subdirectories that look right, and never a refusal — a valid
// project cannot be reliably recognized, so a wrong guess must cost nothing.
func TestRun_HeadlessWarnsAboutASuspectDirectory(t *testing.T) {
	dir := t.TempDir()
	app := filepath.Join(dir, "web")
	require.NoError(t, os.MkdirAll(app, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(app, "package.json"), []byte("{}"), 0o644))

	var stderr bytes.Buffer

	// Dockerfile mode syncs local code, which is what the warning is about —
	// and a suspect directory genuinely has no Dockerfile, so the run then
	// fails on its own terms. The warning must land first: it names the fix
	// (--dir web) for the failure that follows.
	_, err := Run(Options{
		Dir: dir, NonInteractive: true, Stderr: &stderr,
		Answers: Answers{Name: "my-app", BuildMode: manifest.BuildModeDockerfile},
	})
	require.Error(t, err, "no Dockerfile here; the warning explains why")

	warning := stderr.String()
	assert.Contains(t, warning, "none of the usual project files")
	assert.Contains(t, warning, "web", "the offer names what looks right")
	assert.Contains(t, warning, "--dir", "and the flag that takes it")
}

// An image-mode run never syncs local directory contents, so the suspect-dir
// warning would describe a risk that cannot happen — it stays quiet.
func TestRun_HeadlessImageModeSkipsTheSuspectDirWarning(t *testing.T) {
	dir := t.TempDir()
	app := filepath.Join(dir, "web")
	require.NoError(t, os.MkdirAll(app, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(app, "package.json"), []byte("{}"), 0o644))

	var stderr bytes.Buffer

	_, err := Run(Options{
		Dir: dir, NonInteractive: true, Stderr: &stderr,
		Answers: Answers{Name: "my-app", BuildMode: manifest.BuildModeImage, Image: "registry/team/app:v1"},
	})
	require.NoError(t, err)
	assert.NotContains(t, stderr.String(), "none of the usual project files")
}

// A directory that carries a project file gets no warning: the check is about
// the parent-of-the-app mistake, not about lecturing every project.
func TestRun_HeadlessStaysQuietAboutAProjectRoot(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644))

	var stderr bytes.Buffer

	_, err := Run(Options{
		Dir: dir, NonInteractive: true, Stderr: &stderr,
		Answers: Answers{Name: "my-app", BuildMode: manifest.BuildModeImage, Image: "registry/team/app:v1"},
	})
	require.NoError(t, err)
	assert.NotContains(t, stderr.String(), "usual project files")
}

// The manifest lands where the flow ended up. The interactive directory
// question can move the project, and writing to the directory setup was
// started from would put the file beside the app instead of inside it.
func TestRun_WritesWhereTheFlowEndedUp(t *testing.T) {
	parent := t.TempDir()
	app := filepath.Join(parent, "app")
	require.NoError(t, os.MkdirAll(app, 0o755))

	restoreFlow := SetInteractiveFlowForTest(
		func(Options, Detected) ([]byte, manifest.Draft, string, error) {
			draft := defaultDraft(Detect(app))
			draft.Name = "moved"
			draft.Build = manifest.Build{Mode: manifest.BuildModeImage, ImageURI: "registry/team/app:v1"}

			content, err := draft.Render()

			return content, draft, app, err
		})
	defer restoreFlow()

	restoreTerminal := SetStdinTerminalForTest(true)
	defer restoreTerminal()

	var stderr bytes.Buffer

	result, err := Run(Options{Dir: parent, Stderr: &stderr})
	require.NoError(t, err)

	assert.Equal(t, manifest.Path(app), result.Path)
	assert.FileExists(t, manifest.Path(app), "written where the flow chose")
	assert.NoFileExists(t, manifest.Path(parent), "not where setup was started")
}
