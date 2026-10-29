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
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/datarobot/cli/internal/usecase"
	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	latestUC = "68b0aa11bb22cc33dd44ee55"
	otherUC  = "68b0ffffffffffffffffffff"
)

// stubUseCases wires the three lookups: the latest Use Case, the one a
// workload is linked to, and whether a Use Case has Enclaves.
func stubUseCases(t *testing.T, latest, linked *workload.UseCase, enclaves bool) {
	t.Helper()

	prevLatest, prevLinked, prevEnclaves := latestUseCaseFn, workloadUseCaseFn, useCaseEnclavesFn

	latestUseCaseFn = func() (*workload.UseCase, error) { return latest, nil }
	workloadUseCaseFn = func(string) (*workload.UseCase, error) { return linked, nil }
	useCaseEnclavesFn = func(usecase.ID) (bool, error) { return enclaves, nil }

	t.Cleanup(func() { latestUseCaseFn, workloadUseCaseFn, useCaseEnclavesFn = prevLatest, prevLinked, prevEnclaves })
}

func parsedUseCase(t *testing.T, content []byte) (string, string) {
	t.Helper()

	m, err := manifest.Parse(content, "")
	require.NoError(t, err)

	compiled, err := m.Compile()
	require.NoError(t, err)

	policy, _ := documentFrom(t, string(compiled.Payload))["runtime"].(map[string]any)["enclaveSelectionPolicy"].(string)

	return m.UseCaseID(), policy
}

func TestUseCase_NewWorkloadTakesTheLatest(t *testing.T) {
	stubUseCases(t, &workload.UseCase{ID: latestUC, Name: "Sales"}, nil, false)

	var stderr bytes.Buffer

	opts := headless(writeDockerfile(t, t.TempDir(), "FROM scratch\n"), Answers{Name: "my-app"})
	opts.DryRun, opts.Stderr = true, &stderr

	result, err := Run(opts)
	require.NoError(t, err)

	id, policy := parsedUseCase(t, result.Content)
	assert.Equal(t, latestUC, id)
	assert.Equal(t, latestUC, result.UseCaseID)
	assert.Empty(t, policy, "a Use Case with no Enclaves leaves the workload on the Hub")
	assert.Contains(t, stderr.String(), "Sales ("+latestUC+"), the most recently updated")
}

// The create endpoint refuses a Use Case with Enclaves and no policy, so the
// file has to carry one for the first up to succeed.
func TestUseCase_NewWorkloadOnAnEnclaveUseCaseGetsAvailability(t *testing.T) {
	stubUseCases(t, &workload.UseCase{ID: latestUC}, nil, true)

	result, err := Run(headless(writeDockerfile(t, t.TempDir(), "FROM scratch\n"), Answers{Name: "my-app"}))
	require.NoError(t, err)

	id, policy := parsedUseCase(t, result.Content)
	assert.Equal(t, latestUC, id)
	assert.Equal(t, "availability", policy)
}

func TestUseCase_FlagWinsOverTheLatest(t *testing.T) {
	stubUseCases(t, &workload.UseCase{ID: latestUC}, nil, false)

	result, err := Run(headless(writeDockerfile(t, t.TempDir(), "FROM scratch\n"),
		Answers{Name: "my-app", UseCaseID: " " + otherUC + " "}))
	require.NoError(t, err)

	id, _ := parsedUseCase(t, result.Content)
	assert.Equal(t, otherUC, id)
}

func TestUseCase_FlagMustBeAnID(t *testing.T) {
	_, err := Run(headless(writeDockerfile(t, t.TempDir(), "FROM scratch\n"),
		Answers{Name: "my-app", UseCaseID: "sales"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --use-case-id")
}

// A Use Case cannot be swapped under a workload, so binding writes the one it
// has and refuses a flag naming another.
func TestUseCase_BoundWorkloadKeepsItsLink(t *testing.T) {
	stubBoundLive(t)
	stubUseCases(t, &workload.UseCase{ID: latestUC}, &workload.UseCase{ID: otherUC, Name: "Ops"}, false)

	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\n")

	result, err := Run(Options{
		Dir: dir, NonInteractive: true, DryRun: true,
		Answers: Answers{WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0"},
	})
	require.NoError(t, err)

	id, _ := parsedUseCase(t, result.Content)
	assert.Equal(t, otherUC, id)

	_, err = Run(Options{
		Dir: dir, NonInteractive: true, DryRun: true,
		Answers: Answers{WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0", UseCaseID: latestUC},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be changed")
}

// An unlinked bound workload takes the latest for up to link, and keeps the
// placement it runs with: the Enclave check is for a create only.
func TestUseCase_UnlinkedBoundWorkloadTakesTheLatest(t *testing.T) {
	stubBoundLive(t)
	stubUseCases(t, &workload.UseCase{ID: latestUC}, nil, true)

	result, err := Run(Options{
		Dir: writeDockerfile(t, t.TempDir(), "FROM scratch\n"), NonInteractive: true,
		DryRun: true, Answers: Answers{WorkloadID: "68b0c1d2e3f4a5b6c7d8e9f0"},
	})
	require.NoError(t, err)

	id, policy := parsedUseCase(t, result.Content)
	assert.Equal(t, latestUC, id)
	assert.Empty(t, policy)
}

// A tenant where Use Cases cannot be listed still gets a manifest: a
// workload off any Enclave needs none.
func TestUseCase_ListingFailureWritesNone(t *testing.T) {
	stubUseCases(t, nil, nil, false)

	latestUseCaseFn = func() (*workload.UseCase, error) { return nil, errors.New("403 Forbidden") }

	var stderr bytes.Buffer

	opts := headless(writeDockerfile(t, t.TempDir(), "FROM scratch\n"), Answers{Name: "my-app"})
	opts.Stderr = &stderr

	result, err := Run(opts)
	require.NoError(t, err)

	id, _ := parsedUseCase(t, result.Content)
	assert.Empty(t, id)
	assert.Contains(t, stderr.String(), "Could not list Use Cases")
}

func stubBoundLive(t *testing.T) {
	t.Helper()

	stubLive(t, documentFrom(t, `{"name": "app", "artifactId": "68a1",
			"runtime": {"containerGroups": [{"name": "default", "replicaCount": 1,
				"containers": [{"name": "primary", "resourceAllocation": {"cpu": 1, "memory": "2GB"}}]}]}}`),
		documentFrom(t, `{"name": "app-artifact", "type": "service", "spec": {"containerGroups": [{"name": "default",
				"containers": [{"name": "primary", "primary": true, "port": 8080, "imageUri": "registry/app:v1"}]}]}}`))
}

// The rendered manifest drops top-level keys it does not model, so the spec
// file's own useCaseId is read from the file, and gets the same Enclave check.
func TestUseCase_SpecFileKeepsItsOwn(t *testing.T) {
	stubUseCases(t, &workload.UseCase{ID: latestUC}, nil, true)

	dir := writeDockerfile(t, t.TempDir(), "FROM scratch\n")
	spec := filepath.Join(t.TempDir(), "workload.json")
	require.NoError(t, os.WriteFile(spec, []byte(`{"name": "my-app", "useCaseId": "`+otherUC+`",
		"artifact": {"name": "a", "spec": {"containerGroups": [{"name": "default", "containers": [
			{"name": "primary", "primary": true, "port": 8080, "imageUri": "registry/app:v1"}]}]}}}`), 0o600))

	opts := headless(dir, Answers{})
	opts.SpecFile = spec

	result, err := Run(opts)
	require.NoError(t, err)

	id, policy := parsedUseCase(t, result.Content)
	assert.Equal(t, otherUC, id)
	assert.Equal(t, "availability", policy)

	require.NoError(t, os.Remove(manifest.Path(dir)))

	opts.Answers.UseCaseID = latestUC

	_, err = Run(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disagrees")
}
