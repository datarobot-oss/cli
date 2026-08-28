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

package doctor

import (
	"context"
	"testing"

	core "github.com/datarobot/cli/internal/doctor"
	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDivergenceCheck_PointerMatrix(t *testing.T) {
	tests := []struct {
		name              string
		cfgVersionID      string
		manifestVersionID string
		want              core.Status
	}{
		{"both null", "", "", core.StatusOK},
		{"both set and equal", testVersionID, testVersionID, core.StatusOK},
		{"config set, manifest null", testVersionID, "", core.StatusFAIL},
		{"config null, manifest set", "", testVersionID, core.StatusFAIL},
		{"both set but different", testVersionID, "ffffffffffffffffffffffff", core.StatusFAIL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			initStateDir(t, dir)

			require.NoError(t, wapi.SaveConfig(dir, validConfig(testCatalogID, tt.cfgVersionID)))

			require.NoError(t, wapi.SaveManifest(dir, validManifest(tt.manifestVersionID)))

			res := (&divergenceCheck{projectDir: dir}).Run(context.Background())

			assert.Equal(t, tt.want, res.Status)

			if tt.want == core.StatusFAIL {
				assert.Equal(t, RemedyDivergence, res.Remedy)

				assert.True(t, res.Fixable)
			}
		})
	}
}

func TestDivergenceCheck_SKIP_ConfigUnreadable(t *testing.T) {
	dir := t.TempDir()

	initStateDir(t, dir)

	writeStateFile(t, dir, "manifest.json", `{"version":1,"syncedAt":null,"syncedVersionId":null,"files":{}}`)

	res := (&divergenceCheck{projectDir: dir}).Run(context.Background())

	assert.Equal(t, core.StatusSKIP, res.Status)
}

func TestDivergenceCheck_SKIP_ManifestUnreadable(t *testing.T) {
	dir := t.TempDir()

	initStateDir(t, dir)

	require.NoError(t, wapi.SaveConfig(dir, validConfig(testCatalogID, testVersionID)))

	res := (&divergenceCheck{projectDir: dir}).Run(context.Background())

	assert.Equal(t, core.StatusSKIP, res.Status)
}

func TestDivergenceCheck_SKIP_NotLinked(t *testing.T) {
	res := (&divergenceCheck{projectDir: t.TempDir()}).Run(context.Background())

	assert.Equal(t, core.StatusSKIP, res.Status)

	assert.Contains(t, res.Summary, "no linked state")
}

func TestNormalizeStringPtr(t *testing.T) {
	assert.Nil(t, normalizeStringPtr(nil))

	assert.Nil(t, normalizeStringPtr(strPtr("")))

	assert.Equal(t, strPtr(testVersionID), normalizeStringPtr(strPtr(testVersionID)))
}
