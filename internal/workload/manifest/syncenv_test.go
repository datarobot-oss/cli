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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// everyShapeManifest carries one entry of every kind a reconciliation has to
// settle, so a single sync exercises all of them against each other.
const everyShapeManifest = `name: my-app
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
              # hand-written, and worth keeping
              - name: GREETING
                value: hello
              - name: OLD
                value: gone
              - name: WAS_SECRET
                value: dr-credential:68f0cccc0000000000000003/apiToken
              - name: PENDING
                value: dr-credential:PLACEHOLDER/apiToken
              - name: STRUCTURED
                value:
                  nested: thing
runtime:
  containerGroups:
    - name: default
      replicaCount: 1
      containers:
        - name: primary
          resourceAllocation: {cpu: 0.5, memory: 512MB}
`

func syncFixture(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), FileName)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

func syncNames(vars []EnvVar) []string {
	out := make([]string, 0, len(vars))
	for _, v := range vars {
		out = append(out, v.Name)
	}

	return out
}

// .env wins on every category at once, which is the whole contract: the value,
// the form the entry takes, and whether the entry is there at all.
func TestSyncEnvVars_EnvWinsOnEveryCategory(t *testing.T) {
	path := syncFixture(t, everyShapeManifest)

	changes, out, err := SyncEnvVars(path, []EnvVar{
		{Name: "GREETING", Value: "hello there"},
		{Name: "WAS_SECRET", Value: "now-a-literal"},
		{Name: "PENDING", Secret: true, CredentialID: "68f0dddd0000000000000009"},
		{Name: "STRUCTURED", Value: "flat"},
		{Name: "NEW_SECRET", Secret: true, CredentialID: "68f0eeee000000000000000a"},
		{Name: "NEW_PLAIN", Value: "added"},
	}, false)
	require.NoError(t, err)

	assert.Equal(t, []string{"NEW_SECRET", "NEW_PLAIN"}, syncNames(changes.Added))
	assert.Equal(t, []string{"GREETING"}, syncNames(changes.Updated))
	assert.Equal(t, []string{"WAS_SECRET", "PENDING", "STRUCTURED"}, syncNames(changes.Replaced))
	assert.Equal(t, []string{"OLD"}, changes.Removed)

	written := string(out)

	assert.Contains(t, written, "value: hello there", "a literal follows .env")
	assert.Contains(t, written, "value: now-a-literal", "a reference .env now reads as plain becomes one")
	assert.Contains(t, written, "dr-credential:68f0dddd0000000000000009/apiToken", "the placeholder is finished")
	assert.Contains(t, written, "value: flat", "a mapping .env can only hold as a string becomes one")
	assert.NotContains(t, written, "nested: thing")

	assert.NotContains(t, written, "OLD", "a name .env dropped is removed")
}

// A name it does not touch keeps everything about itself, which is what makes
// this safe to run against a file people hand-edit.
func TestSyncEnvVars_KeepsCommentsAndOrder(t *testing.T) {
	path := syncFixture(t, everyShapeManifest)

	_, out, err := SyncEnvVars(path, []EnvVar{
		{Name: "GREETING", Value: "hello there"},
		{Name: "OLD", Value: "gone"},
		{Name: "WAS_SECRET", Secret: true, CredentialID: "68f0cccc0000000000000003"},
		{Name: "PENDING", Secret: true},
		{Name: "STRUCTURED", Value: "flat"},
	}, false)
	require.NoError(t, err)

	written := string(out)

	assert.Contains(t, written, "# hand-written, and worth keeping")
	assert.Less(t, strings.Index(written, "GREETING"), strings.Index(written, "OLD"),
		"surviving entries keep the order the file put them in, not the order .env yields")
}

// A placeholder with still nowhere to point is left exactly as it was, so the
// run reports no change rather than a change that changed nothing.
func TestSyncEnvVars_LeavesAnUnfinishablePlaceholder(t *testing.T) {
	path := syncFixture(t, everyShapeManifest)

	changes, _, err := SyncEnvVars(path, []EnvVar{
		{Name: "GREETING", Value: "hello"},
		{Name: "OLD", Value: "gone"},
		{Name: "WAS_SECRET", Secret: true, CredentialID: "68f0cccc0000000000000003"},
		{Name: "PENDING", Secret: true},
		{Name: "STRUCTURED", Value: "flat"},
	}, false)
	require.NoError(t, err)

	assert.NotContains(t, syncNames(changes.Replaced), "PENDING")
	assert.Contains(t, syncNames(changes.Replaced), "STRUCTURED",
		"a value the file holds as a mapping can never agree with a file that holds only strings")
}

// Nothing to do writes nothing, so a sync is safe to run on every deploy.
func TestSyncEnvVars_AgreementIsANoOp(t *testing.T) {
	// Without the structured entry: .env holds only strings, so a value the
	// manifest keeps as a mapping is drift this can never call settled.
	path := syncFixture(t, strings.Replace(everyShapeManifest,
		"              - name: STRUCTURED\n                value:\n                  nested: thing\n", "", 1))
	before, err := os.ReadFile(path)
	require.NoError(t, err)

	changes, _, err := SyncEnvVars(path, []EnvVar{
		{Name: "GREETING", Value: "hello"},
		{Name: "OLD", Value: "gone"},
		{Name: "WAS_SECRET", Secret: true, CredentialID: "68f0cccc0000000000000003"},
		{Name: "PENDING", Secret: true},
	}, false)
	require.NoError(t, err)

	assert.False(t, changes.Any())

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "the file is byte for byte what it was")
}

// A preview renders the result and leaves the file alone, which is what lets
// the caller show a plan and take an answer before anything is stored.
func TestSyncEnvVars_PreviewWritesNothing(t *testing.T) {
	path := syncFixture(t, everyShapeManifest)
	before, err := os.ReadFile(path)
	require.NoError(t, err)

	changes, out, err := SyncEnvVars(path, []EnvVar{{Name: "GREETING", Value: "hello there"}}, true)
	require.NoError(t, err)

	assert.True(t, changes.Any())
	assert.Contains(t, string(out), "hello there")

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
}

// Removing an entry another container reads through an anchor would change
// that container too, in a place no diff of this one would show. Refused
// rather than skipped: the caller asked for a reconciliation and has to be
// told it cannot have one.
func TestSyncEnvVars_RefusesToDropASharedEntry(t *testing.T) {
	path := syncFixture(t, `name: shared
artifact:
  name: shared-artifact
  type: service
  spec:
    containerGroups:
      - name: default
        containers:
          - name: primary
            primary: true
            port: 8080
            imageUri: registry/team/app:v1
            environmentVars: &sharedEnv
              - name: LOG_LEVEL
                value: debug
          - name: sidecar
            imageUri: registry/team/side:v1
            environmentVars: *sharedEnv
runtime:
  containerGroups:
    - name: default
      replicaCount: 1
      containers:
        - name: primary
          resourceAllocation: {cpu: 0.5, memory: 512MB}
        - name: sidecar
          resourceAllocation: {cpu: 0.5, memory: 512MB}
`)

	_, _, err := SyncEnvVars(path, []EnvVar{{Name: "REGION", Value: "eu-west-1"}}, false)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSharedEnvVars)
}

// The sharing can sit below the entry: a value anchored for another entry to
// read through goes out with its entry, and the alias is left pointing at
// nothing, in a file that then no longer parses. The rewrite already refuses
// that shape, and a removal loses more.
func TestSyncEnvVars_RefusesToDropAnEntryWhoseValueIsAnchored(t *testing.T) {
	path := syncFixture(t, container(`            environmentVars:
              - name: REGION
                value: &region eu-west-1
              - name: BACKUP_REGION
                value: *region
`))

	_, _, err := SyncEnvVars(path, []EnvVar{{Name: "BACKUP_REGION", Value: "eu-west-1"}}, false)
	require.ErrorIs(t, err, ErrSharedEnvVars)
}

// A change of form rebuilds the entry, which drops what was anchored inside
// the old one just as a removal would.
func TestSyncEnvVars_RefusesToRebuildAnEntryWhoseValueIsAnchored(t *testing.T) {
	path := syncFixture(t, container(`            environmentVars:
              - name: API_KEY
                value: &key fixture-not-a-real-key-1a2b3c4d
              - name: API_KEY_COPY
                value: *key
`))

	_, _, err := SyncEnvVars(path, []EnvVar{
		{Name: "API_KEY", Secret: true, CredentialID: "68f0cccc0000000000000003"},
		{Name: "API_KEY_COPY", Value: "fixture-not-a-real-key-1a2b3c4d"},
	}, false)
	require.ErrorIs(t, err, ErrSharedEnvVars)
}

// A comment is the reader's note about a variable, and it is as true of the
// new form as of the old. The entry is rebuilt because a literal and a
// reference share no structure, but a comment is not structure, and losing it
// because the value moved takes something that was never this edit's to take.
func TestSyncEnvVars_KeepsCommentsAcrossAFormChange(t *testing.T) {
	path := syncFixture(t, container(`            environmentVars:
              # billing key, rotate quarterly
              - name: API_KEY
                value: plain-old
`))

	_, out, err := SyncEnvVars(path, []EnvVar{
		{Name: "API_KEY", Secret: true, CredentialID: "68f0cccc0000000000000003"},
	}, false)
	require.NoError(t, err)

	written := string(out)

	assert.Contains(t, written, "# billing key, rotate quarterly")
	assert.Contains(t, written, "dr-credential:68f0cccc0000000000000003/apiToken")
	assert.NotContains(t, written, "plain-old")
}

// An entry that spells out no value at all is replaced whole, so whoever
// builds the table the user agrees to has to be able to see that coming.
// Reading it as `value: ""` made it agree with an empty .env line, and the
// edit then landed with no row and nothing asked.
func TestDeclaredEnvVars_MarksAnEntryWithNoWrittenValue(t *testing.T) {
	parsed, err := Parse([]byte(container(`            environmentVars:
              - name: GREETING
                source: api-key
              - name: LOG_LEVEL
                value: ""
`)), FileName)
	require.NoError(t, err)

	declared := parsed.DeclaredEnvVars()
	require.Len(t, declared, 2)

	assert.True(t, declared[0].Unwritten, "no value key at all")
	assert.False(t, declared[1].Unwritten, "an empty value is a value")
}
