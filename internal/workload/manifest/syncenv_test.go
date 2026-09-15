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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mixedManifest is a container whose entries need different things of a sync,
// so one call exercises the whole pass.
const mixedManifest = `name: my-app
artifact:
  name: my-app-artifact
  type: service
  spec:
    containerGroups:
      - name: default
        containers:
          - name: primary
            primary: true
            port: 8080
            imageUri: registry/team/app:v1
            environmentVars:
              # The one the deploy reads first.
              - name: LOG_LEVEL
                value: debug
              - name: REGION
                value: eu-west-1
              - name: RETIRED
                value: yes-really
`

// One call, both halves. The two flags this replaced meant two parses, two
// renders, two validations and two writes for a run that wanted both, with the
// second reading a tree the first had already changed.
func TestSyncEnvVars_AddsAndUpdatesInOnePass(t *testing.T) {
	path := writeManifest(t, t.TempDir(), mixedManifest)

	changes, content, err := SyncEnvVars(path, []EnvVar{
		{Name: "LOG_LEVEL", Value: "trace"},
		{Name: "REGION", Value: "eu-west-1"},
		{Name: "RETIRED", Value: "yes-really"},
		{Name: "TIMEOUT", Value: "30"},
	}, false)
	require.NoError(t, err)

	assert.Equal(t, []string{"LOG_LEVEL"}, names(changes.Updated), "the value that moved")
	assert.Equal(t, []string{"TIMEOUT"}, names(changes.Added), "the name the file did not carry")
	assert.NotEmpty(t, content)

	on := readFile(t, path)
	assert.Contains(t, on, "trace")
	assert.NotContains(t, on, "debug")
	assert.Contains(t, on, "TIMEOUT")
}

// An entry the sync leaves alone keeps everything about itself, which is what
// makes this safe to run against a file people hand-edit. The addition goes on
// the end rather than wherever a dotenv parser happened to yield it.
func TestSyncEnvVars_KeepsCommentsAndOrder(t *testing.T) {
	path := writeManifest(t, t.TempDir(), mixedManifest)

	_, _, err := SyncEnvVars(path, []EnvVar{
		{Name: "LOG_LEVEL", Value: "trace"},
		{Name: "TIMEOUT", Value: "30"},
		{Name: "REGION", Value: "eu-west-1"},
		{Name: "RETIRED", Value: "yes-really"},
	}, false)
	require.NoError(t, err)

	on := readFile(t, path)
	assert.Contains(t, on, "# The one the deploy reads first.", "a comment is not this edit's to take")
	assert.Less(t, strings.Index(on, "REGION"), strings.Index(on, "TIMEOUT"),
		"the file's own order survives, and what is new is appended")
}

// A name the manifest carries and .env no longer mentions is left alone.
// Deleting a line from a developer's own file says nothing about what the
// workload should run, so the sync adds and rewrites and never removes.
func TestSyncEnvVars_LeavesANameEnvNoLongerCarriesAlone(t *testing.T) {
	path := writeManifest(t, t.TempDir(), mixedManifest)

	changes, _, err := SyncEnvVars(path, []EnvVar{
		{Name: "LOG_LEVEL", Value: "debug"},
		{Name: "REGION", Value: "eu-west-1"},
	}, false)
	require.NoError(t, err)

	assert.False(t, changes.Any(), "nothing to add and nothing to move is not an edit")
	assert.Equal(t, mixedManifest, readFile(t, path))
}
