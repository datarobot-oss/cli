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
	"testing"

	"github.com/datarobot/cli/internal/workload/manifest"
	"github.com/stretchr/testify/assert"
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
