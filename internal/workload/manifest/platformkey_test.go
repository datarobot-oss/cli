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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The platform's own key is declared, not stored: a token held as a credential
// becomes `source: api-key`, and once it is, the entry agrees with .env.
func TestSyncEnvVars_DeclaresThePlatformKey(t *testing.T) {
	path := syncFixture(t, container(`            environmentVars:
              - name: DATAROBOT_API_TOKEN
                value: dr-credential:68f0cccc0000000000000001/apiToken
`))
	vars := []EnvVar{{Name: APIKeyEnvName, APIKey: true}}

	changes, out, err := SyncEnvVars(path, vars, false)
	require.NoError(t, err)

	assert.Equal(t, []string{APIKeyEnvName}, syncNames(changes.Replaced))
	assert.Contains(t, string(out), "source: api-key")
	assert.NotContains(t, string(out), "dr-credential", "the stored copy of the token is no longer referenced")

	again, _, err := SyncEnvVars(path, vars, false)
	require.NoError(t, err)
	assert.False(t, again.Any(), "an entry already declaring the key is final")
}

// A name .env has and the manifest lacks is written as the key, with no value
// beside it: a literal would carry the user's token into git.
func TestSyncEnvVars_AddsThePlatformKeyWithoutAValue(t *testing.T) {
	path := syncFixture(t, container("            environmentVars:\n              - name: GREETING\n                value: hello\n"))

	changes, out, err := SyncEnvVars(path, []EnvVar{
		{Name: "GREETING", Value: "hello"},
		{Name: APIKeyEnvName, APIKey: true},
	}, false)
	require.NoError(t, err)

	assert.Equal(t, []string{APIKeyEnvName}, syncNames(changes.Added))
	assert.Contains(t, string(out), "- name: DATAROBOT_API_TOKEN\n                source: api-key\n")
}

// An api-key entry has no value on purpose, so it is not one a reconciliation
// would have to rebuild for lacking one.
func TestDeclaredEnvVars_MarksThePlatformKey(t *testing.T) {
	parsed, err := Parse([]byte(container(`            environmentVars:
              - name: DATAROBOT_API_TOKEN
                source: api-key
`)), FileName)
	require.NoError(t, err)

	declared := parsed.DeclaredEnvVars()
	require.Len(t, declared, 1)

	assert.True(t, declared[0].APIKey)
	assert.False(t, declared[0].Unwritten)
	assert.False(t, declared[0].Secret())
}

// A .env that lacks the token is not a request to drop the workload's own key.
func TestSyncEnvVars_KeepsThePlatformKeyWhenEnvLacksIt(t *testing.T) {
	path := syncFixture(t, container(`            environmentVars:
              - name: DATAROBOT_API_TOKEN
                source: api-key
              - name: GREETING
                value: hello
`))

	changes, _, err := SyncEnvVars(path, []EnvVar{{Name: "GREETING", Value: "hello"}}, false)
	require.NoError(t, err)

	assert.False(t, changes.Any(), "nothing is removed, so there is nothing to write")
}

// An entry carrying both source and value is not settled: which one the
// platform honours is unclear, so it is rewritten to the clean form.
func TestSyncEnvVars_RewritesAPlatformKeyThatAlsoHoldsAValue(t *testing.T) {
	path := syncFixture(t, container(`            environmentVars:
              - name: DATAROBOT_API_TOKEN
                source: api-key
                value: dr-credential:68f0cccc0000000000000001/apiToken
`))

	changes, out, err := SyncEnvVars(path, []EnvVar{{Name: APIKeyEnvName, APIKey: true}}, false)
	require.NoError(t, err)

	assert.Equal(t, []string{APIKeyEnvName}, syncNames(changes.Replaced))
	assert.NotContains(t, string(out), "dr-credential")
}

// Binding a live workload declares the key as the platform's, never as an
// empty literal that would deploy the container without a token.
func TestLive_ApplyDeclaresThePlatformKey(t *testing.T) {
	live := liveFixture(t)

	draft := live.Defaults()
	draft.EnvVars = []EnvVar{{Name: APIKeyEnvName, APIKey: true}}

	applied, err := live.Apply(draft)
	require.NoError(t, err)

	rendered, err := applied.Render()
	require.NoError(t, err)

	assert.Contains(t, string(rendered), "source: api-key")
	assert.NotContains(t, string(rendered), `value: ""`)
}

// The platform defaults a nameless api-key entry to DATAROBOT_API_TOKEN and
// rejects a second one, so neither bind nor sync may append a duplicate.
func TestSyncEnvVars_TreatsANamelessPlatformKeyAsDeclared(t *testing.T) {
	path := syncFixture(t, container(`            environmentVars:
              - source: api-key
`))

	changes, _, err := SyncEnvVars(path, []EnvVar{{Name: APIKeyEnvName, APIKey: true}}, false)
	require.NoError(t, err)

	assert.False(t, changes.Any(), "the entry already answers to the name")
}

func TestLive_ApplyDoesNotDuplicateANamelessPlatformKey(t *testing.T) {
	existing := []any{map[string]any{keySource: sourceAPIKey}}

	assert.True(t, declaredEnvNames(existing)[APIKeyEnvName])
}
