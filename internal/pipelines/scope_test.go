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

package pipelines

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intPtr(v int) *int { return &v }

func TestResolveScope(t *testing.T) {
	tests := []struct {
		name        string
		scope       string
		version     *int
		wantScope   Scope
		wantVersion *int
		wantErr     string
	}{
		{
			name:      "no scope, no version -> draft",
			wantScope: ScopeDraft,
		},
		{
			name:        "no scope, version=2 -> locked v2 (auto)",
			version:     intPtr(2),
			wantScope:   ScopeLocked,
			wantVersion: intPtr(2),
		},
		{
			name:      "scope=draft, no version -> draft",
			scope:     "draft",
			wantScope: ScopeDraft,
		},
		{
			name:    "scope=draft + version -> error",
			scope:   "draft",
			version: intPtr(1),
			wantErr: "draft cannot be combined",
		},
		{
			name:    "scope=locked, no version -> error",
			scope:   "locked",
			wantErr: "requires --version",
		},
		{
			name:        "scope=locked + version=3",
			scope:       "locked",
			version:     intPtr(3),
			wantScope:   ScopeLocked,
			wantVersion: intPtr(3),
		},
		{
			name:    "invalid scope",
			scope:   "weird",
			wantErr: "invalid --scope",
		},
		{
			name:      "case-insensitive scope",
			scope:     "DRAFT",
			wantScope: ScopeDraft,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scope, version, err := ResolveScope(tc.scope, tc.version)

			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantScope, scope)

			if tc.wantVersion == nil {
				assert.Nil(t, version)
			} else {
				require.NotNil(t, version)
				assert.Equal(t, *tc.wantVersion, *version)
			}
		})
	}
}

func TestPipelinePath(t *testing.T) {
	t.Run("draft scope", func(t *testing.T) {
		got, err := PipelinePath("abc", ScopeDraft, nil, "inputs")
		require.NoError(t, err)
		assert.Equal(t, "/api/v2/pipelines/abc/inputs", got)
	})

	t.Run("locked scope with version", func(t *testing.T) {
		got, err := PipelinePath("abc", ScopeLocked, intPtr(2), "schedules")
		require.NoError(t, err)
		assert.Equal(t, "/api/v2/pipelines/abc/versions/2/schedules", got)
	})

	t.Run("nested suffix", func(t *testing.T) {
		got, err := PipelinePath("abc", ScopeLocked, intPtr(2), "dispatches/xyz/status")
		require.NoError(t, err)
		assert.Equal(t, "/api/v2/pipelines/abc/versions/2/dispatches/xyz/status", got)
	})

	t.Run("empty suffix returns base", func(t *testing.T) {
		got, err := PipelinePath("abc", ScopeDraft, nil, "")
		require.NoError(t, err)
		assert.Equal(t, "/api/v2/pipelines/abc", got)
	})

	t.Run("leading slash on suffix is tolerated", func(t *testing.T) {
		got, err := PipelinePath("abc", ScopeDraft, nil, "/inputs")
		require.NoError(t, err)
		assert.Equal(t, "/api/v2/pipelines/abc/inputs", got)
	})

	t.Run("locked without version is an error", func(t *testing.T) {
		_, err := PipelinePath("abc", ScopeLocked, nil, "inputs")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "version")
	})

	t.Run("missing pipeline id is an error", func(t *testing.T) {
		_, err := PipelinePath("", ScopeDraft, nil, "inputs")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "pipeline id")
	})
}
