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
	"io"
	"testing"

	"github.com/datarobot/cli/internal/workload"
	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The token in .env is the deploying user's own, so it is declared as the
// platform's key and never carried as a literal or stored as a credential.
func TestManifestVar_DeclaresThePlatformKey(t *testing.T) {
	got := manifestVar(manifest.APIKeyEnvName, "a-real-token", true)

	assert.Equal(t, manifest.EnvVar{Name: manifest.APIKeyEnvName, APIKey: true}, got)
	assert.False(t, got.Secret, "nothing is stored for it, so no credential is minted")
	assert.Empty(t, got.Value, "the token never reaches the file")
}

func TestManifestVar_LeavesEveryOtherNameAlone(t *testing.T) {
	assert.Equal(t, manifest.EnvVar{Name: "GREETING", Value: "hi"}, manifestVar("GREETING", "hi", false))
	assert.Equal(t, manifest.EnvVar{Name: "SLACK_BOT_TOKEN", Value: "x", Secret: true}, manifestVar("SLACK_BOT_TOKEN", "x", true))
}

func TestSyncVars_CarriesThePlatformKeyAsTheKey(t *testing.T) {
	detected := Detected{EnvVars: []EnvVar{
		ClassifyEnv(manifest.APIKeyEnvName, "a-real-token"),
		ClassifyEnv("GREETING", "hi"),
	}}

	got := Answers{}.syncVars(detected)

	assert.Equal(t, []manifest.EnvVar{
		{Name: manifest.APIKeyEnvName, APIKey: true},
		{Name: "GREETING", Value: "hi"},
	}, got)
}

func TestClassifyEnv_SaysThePlatformKeyReplacesTheToken(t *testing.T) {
	got := ClassifyEnv(manifest.APIKeyEnvName, "a-real-token")

	assert.Contains(t, got.Reason, "workload's own key")
}

func TestLiteralNames_SkipsThePlatformKey(t *testing.T) {
	names := LiteralNames([]manifest.EnvVar{
		{Name: manifest.APIKeyEnvName, APIKey: true},
		{Name: "GREETING", Value: "hello"},
	})

	assert.Equal(t, []string{"GREETING"}, names)
}

func TestPlan_NamesThePlatformKey(t *testing.T) {
	assert.Equal(t, "workload's own key", plan(EnvVar{Name: manifest.APIKeyEnvName, Kind: EnvSecret}))
	assert.Equal(t, "credential reference", plan(EnvVar{Name: "TOKEN", Kind: EnvSecret}))
}

// A declared api-key entry with the token in .env is settled, not drift:
// without its case, --sync-env would propose a change on every run.
func TestCompareValues_PlatformKeyAlreadyDeclaredIsNotDrift(t *testing.T) {
	parsed, err := manifest.Parse([]byte(`name: my-app
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
              - name: DATAROBOT_API_TOKEN
                source: api-key
`), "")
	require.NoError(t, err)

	detected := Detected{EnvVars: []EnvVar{ClassifyEnv(manifest.APIKeyEnvName, "a-real-token")}}

	changed, unverifiable := Options{}.compareValues(parsed, detected)

	assert.Empty(t, changed)
	assert.Empty(t, unverifiable)
}

func swapCredentialSeams(t *testing.T, name string) (deleted *[]string) {
	t.Helper()

	getOrig, delOrig := getCredentialFn, deleteCredentialFn

	t.Cleanup(func() { getCredentialFn, deleteCredentialFn = getOrig, delOrig })

	var ids []string

	getCredentialFn = func(string) (*workload.Credential, error) { return &workload.Credential{Name: name}, nil }
	deleteCredentialFn = func(id string) error { ids = append(ids, id); return nil }

	return &ids
}

func replacedByAPIKey(t *testing.T) *manifest.Manifest {
	t.Helper()

	parsed, err := manifest.Parse([]byte(`name: my-app
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
              - name: DATAROBOT_API_TOKEN
                source: dr-credential
                drCredentialId: 68f0cccc0000000000000001
                key: apiToken
`), "")
	require.NoError(t, err)

	return parsed
}

var apiKeyWanted = []manifest.EnvVar{{Name: manifest.APIKeyEnvName, APIKey: true}}

// The credential the CLI minted from the user's token is offered for deletion,
// and goes only when the user says yes.
func TestOfferCredentialCleanup_DeletesOnYes(t *testing.T) {
	deleted := swapCredentialSeams(t, "my-app/DATAROBOT_API_TOKEN")
	asked := ""

	Options{ConfirmDelete: func(name string) bool { asked = name; return true }, Stderr: io.Discard}.
		offerCredentialCleanup(replacedByAPIKey(t), apiKeyWanted)

	assert.Equal(t, "my-app/DATAROBOT_API_TOKEN", asked)
	assert.Equal(t, []string{"68f0cccc0000000000000001"}, *deleted)
}

func TestOfferCredentialCleanup_LeavesItOnNoOrWhenNobodyCanAsk(t *testing.T) {
	deleted := swapCredentialSeams(t, "my-app/DATAROBOT_API_TOKEN")

	Options{ConfirmDelete: func(string) bool { return false }, Stderr: io.Discard}.
		offerCredentialCleanup(replacedByAPIKey(t), apiKeyWanted)
	Options{Stderr: io.Discard}.offerCredentialCleanup(replacedByAPIKey(t), apiKeyWanted)

	assert.Empty(t, *deleted)
}

// A credential this project did not create may be shared, so it is never offered.
func TestOfferCredentialCleanup_NeverTouchesSomeoneElsesCredential(t *testing.T) {
	deleted := swapCredentialSeams(t, "other-app/SHARED")
	asked := false

	Options{ConfirmDelete: func(string) bool { asked = true; return true }, Stderr: io.Discard}.
		offerCredentialCleanup(replacedByAPIKey(t), apiKeyWanted)

	assert.False(t, asked)
	assert.Empty(t, *deleted)
}
