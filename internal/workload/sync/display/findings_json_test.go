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

package display

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// planDoc renders a plan with the given findings and returns the decoded
// top-level document.
func planDoc(t *testing.T, plan *sync.SyncPlan, f Findings) map[string]any {
	t.Helper()

	var buf bytes.Buffer

	require.NoError(t, RenderSyncJSON(&buf, plan, nil, false, f))

	var doc map[string]any

	require.NoError(t, json.Unmarshal(buf.Bytes(), &doc), "the plan document must decode as JSON")

	return doc
}

// Both findings keys are part of the contract: always present and explicitly
// empty when there is nothing to report, so a script never has to guess
// whether a missing key meant "checked, clean" or "never checked".
func TestRenderSyncJSON_FindingsAlwaysPresent(t *testing.T) {
	for name, plan := range map[string]*sync.SyncPlan{"a plan": {}, "no plan": nil} {
		t.Run(name, func(t *testing.T) {
			doc := planDoc(t, plan, Findings{})

			for _, key := range []string{"divergence", "skippedSymlinks"} {
				v, ok := doc[key]
				require.True(t, ok, "%s must always be emitted", key)
				require.IsType(t, []any{}, v, "%s is an array, never null", key)
				assert.Empty(t, v)
			}

			assert.Contains(t, doc, "locked")
		})
	}
}

func TestRenderSyncJSON_DivergenceEntries(t *testing.T) {
	doc := planDoc(t, &sync.SyncPlan{}, Findings{Divergence: []sync.Divergence{
		{Path: "app.py", Kind: sync.DivergenceHashMismatch, BaseHash: "h-base", RemoteHash: "h-remote"},
		{Path: "gone.py", Kind: sync.DivergenceBaseOnly, BaseHash: "h-gone"},
		{Path: "stray.py", Kind: sync.DivergenceRemoteOnly, RemoteHash: "h-stray"},
	}})

	raw, err := json.Marshal(doc["divergence"])
	require.NoError(t, err)

	var entries []struct {
		Path       string `json:"path"`
		Kind       string `json:"kind"`
		BaseHash   string `json:"baseHash"`
		RemoteHash string `json:"remoteHash"`
	}

	require.NoError(t, json.Unmarshal(raw, &entries))
	require.Len(t, entries, 3)

	assert.Equal(t, "app.py", entries[0].Path)
	assert.Equal(t, "hash_mismatch", entries[0].Kind)
	assert.Equal(t, "h-base", entries[0].BaseHash)
	assert.Equal(t, "h-remote", entries[0].RemoteHash)

	assert.Equal(t, "base_only", entries[1].Kind)
	assert.Equal(t, "h-gone", entries[1].BaseHash)
	assert.Empty(t, entries[1].RemoteHash)

	assert.Equal(t, "remote_only", entries[2].Kind)
	assert.Equal(t, "h-stray", entries[2].RemoteHash)
	assert.Empty(t, entries[2].BaseHash)
}

// A symlink path lives only in skippedSymlinks, never in an action array, and
// the entry says whether a whole subtree was omitted.
func TestRenderSyncJSON_SkippedSymlinkEntries(t *testing.T) {
	doc := planDoc(t,
		&sync.SyncPlan{Uploads: []sync.FileAction{{Path: "real.py"}}},
		Findings{SkippedSymlinks: []sync.SkippedSymlink{
			{Path: "link_to_file.py", IsDir: false},
			{Path: "link_to_dir", IsDir: true},
		}})

	raw, err := json.Marshal(doc["skippedSymlinks"])
	require.NoError(t, err)

	var entries []struct {
		Path  string `json:"path"`
		IsDir bool   `json:"isDir"`
	}

	require.NoError(t, json.Unmarshal(raw, &entries))
	require.Len(t, entries, 2)
	assert.Equal(t, "link_to_file.py", entries[0].Path)
	assert.False(t, entries[0].IsDir)
	assert.Equal(t, "link_to_dir", entries[1].Path)
	assert.True(t, entries[1].IsDir)

	uploads, ok := doc["uploads"].([]any)
	require.True(t, ok)
	require.Len(t, uploads, 1)

	entry, ok := uploads[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "real.py", entry["path"])
}

// A refused document carries the findings too: the refusal is about the plan,
// and the diagnostics were made before it.
func TestRenderRefusedJSON_CarriesFindings(t *testing.T) {
	var buf bytes.Buffer

	require.NoError(t, RenderRefusedJSON(&buf, &sync.SyncPlan{}, false, Findings{
		SkippedSymlinks: []sync.SkippedSymlink{{Path: "link.py"}},
	}))

	var doc map[string]any

	require.NoError(t, json.Unmarshal(buf.Bytes(), &doc))
	assert.Equal(t, true, doc["refused"])
	assert.Len(t, doc["skippedSymlinks"], 1)
}
