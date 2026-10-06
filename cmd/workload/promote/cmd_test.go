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

package promote

import (
	"bytes"
	"errors"
	"net/http"
	"os"
	"testing"

	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/workload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCmd_ArgIsOptional(t *testing.T) {
	cmd := Cmd()

	require.NoError(t, cmd.Args(cmd, []string{"68b0c1d2e3f4a5b6c7d8e9f0"}))
	require.NoError(t, cmd.Args(cmd, nil))
	require.Error(t, cmd.Args(cmd, []string{"a", "b"}))
}

func TestCmd_InvalidOutputFormat(t *testing.T) {
	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"68b0c1d2e3f4a5b6c7d8e9f0", "--output-format", "yaml"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid output format "yaml"`)
}

func swap[T any](t *testing.T, target *T, with T) {
	t.Helper()

	prev := *target
	*target = with

	t.Cleanup(func() { *target = prev })
}

// capture runs fn with stdout redirected and returns what it wrote.
func capture(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)

	prev := os.Stdout
	os.Stdout = w

	defer func() { os.Stdout = prev }()

	fn()
	require.NoError(t, w.Close())

	var buf bytes.Buffer

	_, err = buf.ReadFrom(r)
	require.NoError(t, err)

	return buf.String()
}

func TestCmd_PromotesTheTypedWorkloadAndReportsTheVersion(t *testing.T) {
	version := 3
	promoted := ""

	swap(t, &promoteWorkloadFn, func(id string) (*workload.Workload, error) {
		promoted = id

		return &workload.Workload{ID: id, ArtifactID: "art-1"}, nil
	})
	swap(t, &getArtifactFn, func(id string) (*workload.Artifact, error) {
		assert.Equal(t, "art-1", id)

		return &workload.Artifact{ID: id, Status: workload.ArtifactStatusLocked, Version: &version}, nil
	})

	var stderr bytes.Buffer

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"68b0c1d2e3f4a5b6c7d8e9f0"})

	out := capture(t, func() { require.NoError(t, cmd.Execute()) })

	assert.Equal(t, "68b0c1d2e3f4a5b6c7d8e9f0", promoted)
	assert.Contains(t, out, "artifact art-1 is locked as version 3")
	assert.Empty(t, stderr.String())
}

func TestCmd_JSONCarriesTheVersion(t *testing.T) {
	version := 3

	swap(t, &promoteWorkloadFn, func(id string) (*workload.Workload, error) {
		return &workload.Workload{ID: id, ArtifactID: "art-1"}, nil
	})
	swap(t, &getArtifactFn, func(id string) (*workload.Artifact, error) {
		return &workload.Artifact{ID: id, Version: &version}, nil
	})

	var stderr bytes.Buffer

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"68b0c1d2e3f4a5b6c7d8e9f0", "--output-format", "json"})

	out := capture(t, func() { require.NoError(t, cmd.Execute()) })

	assert.JSONEq(t, `{"workloadId": "68b0c1d2e3f4a5b6c7d8e9f0", "artifactId": "art-1", "version": 3}`, out)
	assert.Empty(t, stderr.String(), "stdout is the document and nothing else is said")
}

func TestCmd_AVersionReadFailureIsOnlyNoted(t *testing.T) {
	swap(t, &promoteWorkloadFn, func(id string) (*workload.Workload, error) {
		return &workload.Workload{ID: id, ArtifactID: "art-1"}, nil
	})
	swap(t, &getArtifactFn, func(string) (*workload.Artifact, error) {
		return nil, errors.New("boom")
	})

	var stderr bytes.Buffer

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"68b0c1d2e3f4a5b6c7d8e9f0", "--output-format", "json"})

	out := capture(t, func() { require.NoError(t, cmd.Execute()) })

	assert.JSONEq(t, `{"workloadId": "68b0c1d2e3f4a5b6c7d8e9f0", "artifactId": "art-1", "version": null}`, out)
	assert.Contains(t, stderr.String(), "version could not be read back")
}

func TestCmd_AlreadyLockedIsThePlatformsRefusal(t *testing.T) {
	swap(t, &promoteWorkloadFn, func(string) (*workload.Workload, error) {
		return nil, &drapi.HTTPError{
			StatusCode: http.StatusUnprocessableEntity,
			Detail:     "Artifact is already locked. Only draft artifacts can be promoted.",
		}
	})

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"68b0c1d2e3f4a5b6c7d8e9f0"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already locked")
}

// This route's 404 also covers an artifact the caller does not own, so the
// usual "not on this instance" wording would send the reader to check their
// endpoint for a permission problem.
func TestCmd_NotFoundNamesOwnershipAsWell(t *testing.T) {
	swap(t, &promoteWorkloadFn, func(string) (*workload.Workload, error) {
		return nil, &drapi.HTTPError{StatusCode: http.StatusNotFound}
	})

	cmd := Cmd()
	cmd.PreRunE = nil
	cmd.SetArgs([]string{"68b0c1d2e3f4a5b6c7d8e9f0"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not yours to lock")
	assert.NotContains(t, err.Error(), "not on this instance")
}
