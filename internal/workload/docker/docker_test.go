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

package docker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testID     = "6901f3a0c2b7e4d1a5f0b9c3"
	testDigest = "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
)

type call struct {
	args   []string
	config map[string]any
}

// fakeDocker records each call, reads the Docker config it was pointed at
// while that config still exists, and prints output.
func fakeDocker(t *testing.T, output string, fail error) *[]call {
	t.Helper()

	var calls []call

	previous := runFn
	runFn = func(_ context.Context, env []string, out io.Writer, args ...string) error {
		c := call{args: args}

		for _, kv := range env {
			if dir, ok := strings.CutPrefix(kv, "DOCKER_CONFIG="); ok {
				data, err := os.ReadFile(filepath.Join(dir, "config.json"))
				require.NoError(t, err)
				require.NoError(t, json.Unmarshal(data, &c.config))

				c.config["dir"] = dir
			}
		}

		calls = append(calls, c)
		_, _ = io.WriteString(out, output)

		return fail
	}

	t.Cleanup(func() { runFn = previous })

	return &calls
}

func TestParseReference(t *testing.T) {
	ref, ok := ParseReference("hub.example/artifacts/" + testID)
	require.True(t, ok)
	assert.Equal(t, Reference{Host: "hub.example", ArtifactID: testID}, ref)

	ref, ok = ParseReference("hub.example:5000/artifacts/" + testID + "@" + testDigest)
	require.True(t, ok)
	assert.Equal(t, testDigest, ref.Digest)
	assert.Equal(t, "hub.example:5000/artifacts/"+testID, ref.Address())

	for _, uri := range []string{
		"",
		"python:3.12",
		"hub.example/artifacts/" + testID + ":latest",
		"hub.example/artifacts/not-an-id",
		"hub.example/team/artifacts/" + testID,
		"hub.example/artifacts/" + testID + "@sha256:short",
	} {
		_, ok := ParseReference(uri)
		assert.False(t, ok, uri)
	}
}

func TestPushLogsInWithATemporaryConfigAndReturnsTheDigest(t *testing.T) {
	calls := fakeDocker(t, "abc: Pushed\nlatest: digest: "+testDigest+" size: 528\n", nil)

	var seen []string

	digest, err := Push(context.Background(), Reference{Host: "hub.example", ArtifactID: testID}, "tok",
		func(line string) { seen = append(seen, line) })

	require.NoError(t, err)
	assert.Equal(t, testDigest, digest)
	assert.Equal(t, []string{"abc: Pushed", "latest: digest: " + testDigest + " size: 528"}, seen)

	require.Len(t, *calls, 1)

	c := (*calls)[0]
	assert.Equal(t, []string{"push", "hub.example/artifacts/" + testID}, c.args)

	auths := c.config["auths"].(map[string]any)
	auth := auths["hub.example"].(map[string]any)["auth"].(string)
	decoded, err := base64.StdEncoding.DecodeString(auth)
	require.NoError(t, err)
	assert.Equal(t, "api:tok", string(decoded))

	_, statErr := os.Stat(c.config["dir"].(string))
	assert.True(t, os.IsNotExist(statErr), "the config holding the token must be removed")
}

func TestPushWithoutADigestFails(t *testing.T) {
	fakeDocker(t, "Pushed\n", nil)

	_, err := Push(context.Background(), Reference{Host: "h", ArtifactID: testID}, "tok", nil)
	require.ErrorContains(t, err, "reported no digest")
}

func TestPushNeedsAToken(t *testing.T) {
	calls := fakeDocker(t, "", nil)

	_, err := Push(context.Background(), Reference{Host: "h", ArtifactID: testID}, "", nil)
	require.ErrorContains(t, err, "dr auth login")
	assert.Empty(t, *calls)
}

func TestPullOfAPinTagsItWithItsAddress(t *testing.T) {
	calls := fakeDocker(t, "", nil)

	name, digest, err := Pull(context.Background(),
		Reference{Host: "hub.example", ArtifactID: testID, Digest: testDigest}, "tok", nil)

	require.NoError(t, err)
	assert.Equal(t, "hub.example/artifacts/"+testID, name)
	assert.Equal(t, testDigest, digest)
	require.Len(t, *calls, 2)
	assert.Equal(t, []string{"pull", "--platform", Platform, "hub.example/artifacts/" + testID + "@" + testDigest},
		(*calls)[0].args)
	assert.Equal(t, []string{"tag", "hub.example/artifacts/" + testID + "@" + testDigest, "hub.example/artifacts/" + testID},
		(*calls)[1].args)
}

func TestBuildIsForTheClusterPlatform(t *testing.T) {
	calls := fakeDocker(t, "#1 DONE\r#2 DONE", nil)

	var seen []string

	require.NoError(t, Build(context.Background(), "/src", "hub.example/artifacts/"+testID,
		func(line string) { seen = append(seen, line) }))

	assert.Equal(t, []string{
		"build", "--platform", "linux/amd64", "--progress", "plain",
		"-t", "hub.example/artifacts/" + testID, "/src",
	}, (*calls)[0].args)
	assert.Equal(t, []string{"#1 DONE", "#2 DONE"}, seen, "a carriage return ends a line, and the last line is flushed")
}

func TestAvailableNamesTheInstallPage(t *testing.T) {
	fakeDocker(t, "Cannot connect to the Docker daemon\n", errors.New("exit status 1"))

	err := Available(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Cannot connect to the Docker daemon")
	assert.Contains(t, err.Error(), InstallURL)
}

func TestFailedPushKeepsTheCause(t *testing.T) {
	cause := errors.New("exit status 1")
	fakeDocker(t, "denied\n", cause)

	_, err := Push(context.Background(), Reference{Host: "h", ArtifactID: testID}, "tok", nil)
	require.ErrorIs(t, err, cause)
}

func TestPullOfAnAddressReportsTheDigestPulled(t *testing.T) {
	calls := fakeDocker(t, "latest: Pulling from artifacts/x\nDigest: "+testDigest+"\n", nil)

	name, digest, err := Pull(context.Background(), Reference{Host: "hub.example", ArtifactID: testID}, "tok", nil)

	require.NoError(t, err)
	assert.Equal(t, "hub.example/artifacts/"+testID, name)
	assert.Equal(t, testDigest, digest)
	assert.Len(t, *calls, 1, "a tagged pull needs no tag of its own")
}
