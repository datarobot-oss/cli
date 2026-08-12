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
  spec:
    type: service
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
		documentFrom(t, `{"name": "live-app-artifact", "spec": {"type": "service",
			"containerGroups": [{"name": "default", "containers": [
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

func TestRun_BoundWorkloadWithoutAnArtifact(t *testing.T) {
	stubLive(t, documentFrom(t, `{"name": "live-app"}`), nil)

	_, err := Run(headless(t.TempDir(), Answers{WorkloadID: "68b0"}))
	require.Error(t, err)

	assert.Contains(t, err.Error(), "no artifact to copy")
}

// Cancelling writes nothing and says so, so `config && up` stops here.
func TestRun_CancelledWritesNothing(t *testing.T) {
	original := runInteractiveFlowFn
	runInteractiveFlowFn = func(Options, Detected) ([]byte, manifest.Draft, error) {
		return nil, manifest.Draft{}, ErrCancelled
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
	runInteractiveFlowFn = func(Options, Detected) ([]byte, manifest.Draft, error) {
		t.Fatal("the wizard must not run without a terminal")

		return nil, manifest.Draft{}, nil
	}

	t.Cleanup(func() { runInteractiveFlowFn = originalFlow })

	originalTerminal := isStdinTerminalFn
	isStdinTerminalFn = func() bool { return false }

	t.Cleanup(func() { isStdinTerminalFn = originalTerminal })

	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\n")

	_, err := Run(Options{Dir: dir, Answers: Answers{Name: "my-app"}, Stderr: &bytes.Buffer{}})
	require.NoError(t, err)
}
