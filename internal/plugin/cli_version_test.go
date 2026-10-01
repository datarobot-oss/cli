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

package plugin

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompatibleCLIVersion(t *testing.T) {
	tests := []struct {
		name       string
		cliVersion string
		minBound   string
		maxBound   string
		wantOK     bool
		wantErr    bool
	}{
		{
			name:       "no bounds declared",
			cliVersion: "1.0.0",
			minBound:   "",
			maxBound:   "",
			wantOK:     true,
		},
		{
			name:       "min-only bound satisfied above minimum",
			cliVersion: "1.5.0",
			minBound:   "1.0.0",
			maxBound:   "",
			wantOK:     true,
		},
		{
			name:       "min-only bound satisfied at inclusive boundary",
			cliVersion: "1.0.0",
			minBound:   "1.0.0",
			maxBound:   "",
			wantOK:     true,
		},
		{
			name:       "min-only bound violated below minimum",
			cliVersion: "1.9.0",
			minBound:   "2.0.0",
			maxBound:   "",
			wantOK:     false,
		},
		{
			name:       "max-only bound satisfied at inclusive boundary",
			cliVersion: "2.3.0",
			minBound:   "",
			maxBound:   "2.3.0",
			wantOK:     true,
		},
		{
			name:       "max-only bound violated above maximum",
			cliVersion: "2.3.1",
			minBound:   "",
			maxBound:   "2.3.0",
			wantOK:     false,
		},
		{
			name:       "both bounds satisfied",
			cliVersion: "1.5.0",
			minBound:   "1.0.0",
			maxBound:   "2.0.0",
			wantOK:     true,
		},
		{
			name:       "prerelease CLI version compared by core version",
			cliVersion: "1.2.0-rc.1",
			minBound:   "",
			maxBound:   "1.2.0",
			wantOK:     true,
		},
		{
			name:       "dev CLI version bypasses well-formed bounds",
			cliVersion: "dev",
			minBound:   "2.0.0",
			maxBound:   "",
			wantOK:     true,
		},
		{
			name:       "unparseable CLI version bypasses well-formed bounds",
			cliVersion: "not-a-semver",
			minBound:   "",
			maxBound:   "1.0.0",
			wantOK:     true,
		},
		{
			name:       "malformed min bound is always a skip",
			cliVersion: "1.5.0",
			minBound:   "1.x",
			maxBound:   "",
			wantOK:     false,
			wantErr:    true,
		},
		{
			name:       "malformed max bound is always a skip",
			cliVersion: "1.5.0",
			minBound:   "",
			maxBound:   "2.x",
			wantOK:     false,
			wantErr:    true,
		},
		{
			name:       "malformed bound takes precedence over dev CLI bypass",
			cliVersion: "dev",
			minBound:   "1.x",
			maxBound:   "",
			wantOK:     false,
			wantErr:    true,
		},
		{
			name:       "malformed bound takes precedence over unparseable CLI bypass",
			cliVersion: "garbage",
			minBound:   "",
			maxBound:   "2.x",
			wantOK:     false,
			wantErr:    true,
		},
		{
			name:       "v-prefixed bounds are accepted",
			cliVersion: "1.5.0",
			minBound:   "v1.0.0",
			maxBound:   "v2.0.0",
			wantOK:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, err := compatibleCLIVersion(tt.cliVersion, tt.minBound, tt.maxBound)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.wantOK, ok)
		})
	}
}

func TestCliVersionSkip(t *testing.T) {
	t.Run("no bounds returns nil", func(t *testing.T) {
		restore := setCurrentCLIVersionForTest(t, "1.5.0")
		defer restore()

		manifest := &PluginManifest{BasicPluginManifest: BasicPluginManifest{Name: "widget"}}

		assert.Nil(t, cliVersionSkip(manifest, "/usr/local/bin/dr-widget"))
	})

	t.Run("below minimum returns a version-incompatible conflict naming the upgrade path", func(t *testing.T) {
		restore := setCurrentCLIVersionForTest(t, "1.9.0")
		defer restore()

		manifest := &PluginManifest{
			BasicPluginManifest: BasicPluginManifest{Name: "widget"},
			MinCLIVersion:       "2.0.0",
		}

		conflict := cliVersionSkip(manifest, "/usr/local/bin/dr-widget")

		require.NotNil(t, conflict)
		assert.Equal(t, "widget", conflict.Name)
		assert.Equal(t, "/usr/local/bin/dr-widget", conflict.Path)
		assert.Equal(t, SkipReasonVersionIncompatible, conflict.Reason)
		assert.Contains(t, conflict.Detail, "2.0.0")
		assert.Contains(t, conflict.Detail, "1.9.0")
		assert.Contains(t, conflict.Detail, "dr self update")
	})

	t.Run("above maximum returns a version-incompatible conflict naming the plugin update", func(t *testing.T) {
		restore := setCurrentCLIVersionForTest(t, "1.6.0")
		defer restore()

		manifest := &PluginManifest{
			BasicPluginManifest: BasicPluginManifest{Name: "widget"},
			MaxCLIVersion:       "1.5.0",
		}

		conflict := cliVersionSkip(manifest, "/usr/local/bin/dr-widget")

		require.NotNil(t, conflict)
		assert.Equal(t, SkipReasonVersionIncompatible, conflict.Reason)
		assert.Contains(t, conflict.Detail, "1.5.0")
		assert.Contains(t, conflict.Detail, "1.6.0")
		assert.Contains(t, conflict.Detail, "update the plugin")
	})

	t.Run("malformed bound returns a version-incompatible conflict naming the field", func(t *testing.T) {
		restore := setCurrentCLIVersionForTest(t, "1.6.0")
		defer restore()

		manifest := &PluginManifest{
			BasicPluginManifest: BasicPluginManifest{Name: "widget"},
			MaxCLIVersion:       "1.x",
		}

		conflict := cliVersionSkip(manifest, "/usr/local/bin/dr-widget")

		require.NotNil(t, conflict)
		assert.Equal(t, SkipReasonVersionIncompatible, conflict.Reason)
		assert.Contains(t, conflict.Detail, "maxCLIVersion")
		assert.Contains(t, conflict.Detail, "1.x")
	})
}

// setCurrentCLIVersionForTest overrides the currentCLIVersion seam for the
// duration of a test and returns a func that restores the prior value.
// version.Version is "dev" under `go test`, which would otherwise bypass
// every bound check and make these assertions vacuous.
func setCurrentCLIVersionForTest(t *testing.T, v string) func() {
	t.Helper()

	prev := currentCLIVersion
	currentCLIVersion = v

	return func() { currentCLIVersion = prev }
}
