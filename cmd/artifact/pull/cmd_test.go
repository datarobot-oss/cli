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

package pull

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/docker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	artifactID = "6901f3a0c2b7e4d1a5f0b9c3"
	digest     = "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
)

func artifactRunning(t *testing.T, image string) {
	t.Helper()

	previous := getArtifactFn
	getArtifactFn = func(id string) (*workload.Artifact, error) {
		return &workload.Artifact{ID: id, Spec: workload.Spec{ContainerGroups: []workload.ContainerGroup{{
			Containers: []workload.Container{{Name: "primary", ImageURI: image}},
		}}}}, nil
	}

	t.Cleanup(func() { getArtifactFn = previous })
}

func execute(t *testing.T, args ...string) (string, error) {
	t.Helper()

	cmd := Cmd()
	cmd.PreRunE = nil

	var stdout bytes.Buffer

	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)

	err := cmd.ExecuteContext(context.Background())

	return stdout.String(), err
}

func TestPull_RefusesAnImageNotPushedThroughDataRobot(t *testing.T) {
	artifactRunning(t, "registry.example/team/app:v1")

	_, err := execute(t, artifactID)
	require.ErrorContains(t, err, "nothing to pull through DataRobot")
}

func TestPull_PullsAPinAndReportsIt(t *testing.T) {
	artifactRunning(t, "hub.example/artifacts/"+artifactID+"@"+digest)

	previous, previousToken := pullFn, apiTokenFn

	t.Cleanup(func() { pullFn, apiTokenFn = previous, previousToken })

	apiTokenFn = func() string { return "tok" }
	pullFn = func(_ context.Context, ref docker.Reference, token string, _ func(string)) (string, string, error) {
		assert.Equal(t, "tok", token)
		assert.Equal(t, digest, ref.Digest)

		return ref.Address(), ref.Digest, nil
	}

	out, err := execute(t, artifactID, "--output-format", "json")
	require.NoError(t, err)

	var envelope map[string]pulled

	require.NoError(t, json.Unmarshal([]byte(out), &envelope))
	assert.Equal(t, pulled{ArtifactID: artifactID, Image: "hub.example/artifacts/" + artifactID, Digest: digest},
		envelope["pull"])
}
