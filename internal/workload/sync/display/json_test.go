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

// planDocWithDivergence renders a plan with the given divergence findings and
// returns the decoded top-level document.
func planDocWithDivergence(t *testing.T, plan *sync.SyncPlan, divergences []sync.Divergence) map[string]any {
	t.Helper()

	var buf bytes.Buffer

	require.NoError(t, RenderPlanJSON(&buf, plan, false, divergences, nil))

	var doc map[string]any

	require.NoError(t, json.Unmarshal(buf.Bytes(), &doc), "the plan document must decode as JSON")

	return doc
}

// The divergence field is part of the contract, not a bonus: always present
// and explicitly empty when there is no divergence, so a script reading the
// document never has to guess whether a missing key meant "checked, clean"
// or "never checked" (the locked:false precedent).
func TestRenderPlanJSON_DivergenceAlwaysPresent(t *testing.T) {
	doc := planDocWithDivergence(t, &sync.SyncPlan{}, nil)

	div, ok := doc["divergence"]
	require.True(t, ok, "the divergence key must always be emitted")

	require.IsType(t, []any{}, div)
	assert.Empty(t, div, "no divergence must render as an explicit empty array, not null and not an omitted key")
	assert.Contains(t, doc, "locked")
}

func TestRenderPlanJSON_DivergenceEntries(t *testing.T) {
	divergences := []sync.Divergence{
		{Path: "app.py", Kind: sync.DivergenceHashMismatch, BaseHash: "h-base", RemoteHash: "h-remote"},
		{Path: "gone.py", Kind: sync.DivergenceBaseOnly, BaseHash: "h-gone"},
		{Path: "stray.py", Kind: sync.DivergenceRemoteOnly, RemoteHash: "h-stray"},
	}

	doc := planDocWithDivergence(t, &sync.SyncPlan{}, divergences)

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

	assert.Equal(t, "gone.py", entries[1].Path)
	assert.Equal(t, "base_only", entries[1].Kind)
	assert.Equal(t, "h-gone", entries[1].BaseHash)
	assert.Empty(t, entries[1].RemoteHash)

	assert.Equal(t, "stray.py", entries[2].Path)
	assert.Equal(t, "remote_only", entries[2].Kind)
	assert.Equal(t, "h-stray", entries[2].RemoteHash)
	assert.Empty(t, entries[2].BaseHash)
}

// An empty plan (Up to date.) can still carry findings: divergence is
// detected in Phase 2 and reported regardless of what the plan does with the
// paths, so a nil plan must not drop the field.
func TestRenderPlanJSON_NilPlan_KeepsDivergence(t *testing.T) {
	divergences := []sync.Divergence{
		{Path: "app.py", Kind: sync.DivergenceHashMismatch, BaseHash: "h-base", RemoteHash: "h-remote"},
	}

	doc := planDocWithDivergence(t, nil, divergences)

	div, ok := doc["divergence"]
	require.True(t, ok, "the divergence key must be emitted even for a nil plan")

	require.IsType(t, []any{}, div)
	assert.Len(t, div, 1)
}

// planDocWithSymlinks renders a plan with the given skipped symlinks and
// returns the decoded top-level document.
func planDocWithSymlinks(t *testing.T, plan *sync.SyncPlan, symlinks []sync.SkippedSymlink) map[string]any {
	t.Helper()

	var buf bytes.Buffer

	require.NoError(t, RenderPlanJSON(&buf, plan, false, nil, symlinks))

	var doc map[string]any

	require.NoError(t, json.Unmarshal(buf.Bytes(), &doc), "the plan document must decode as JSON")

	return doc
}

// The skippedSymlinks field is part of the contract: always present and
// explicitly empty when there are none (the locked:false precedent).
func TestRenderPlanJSON_SkippedSymlinksAlwaysPresent(t *testing.T) {
	doc := planDocWithSymlinks(t, &sync.SyncPlan{}, nil)

	sym, ok := doc["skippedSymlinks"]
	require.True(t, ok, "the skippedSymlinks key must always be emitted")

	require.IsType(t, []any{}, sym)
	assert.Empty(t, sym, "no skipped symlinks must render as an explicit empty array, not null and not an omitted key")
}

func TestRenderPlanJSON_SkippedSymlinkEntries(t *testing.T) {
	symlinks := []sync.SkippedSymlink{
		{Path: "link_to_file.py", IsDir: false},
		{Path: "link_to_dir", IsDir: true},
	}

	doc := planDocWithSymlinks(t, &sync.SyncPlan{}, symlinks)

	raw, err := json.Marshal(doc["skippedSymlinks"])
	require.NoError(t, err)

	var entries []struct {
		Path  string `json:"path"`
		IsDir bool   `json:"isDir"`
	}

	require.NoError(t, json.Unmarshal(raw, &entries))
	require.Len(t, entries, 2)

	assert.Equal(t, "link_to_file.py", entries[0].Path)
	assert.False(t, entries[0].IsDir, "file symlink must report isDir false")

	assert.Equal(t, "link_to_dir", entries[1].Path)
	assert.True(t, entries[1].IsDir, "directory symlink must report isDir true")
}

// A nil plan can still carry findings: skipped symlinks are detected in Phase 2
// and reported regardless of what the plan does with the paths.
func TestRenderPlanJSON_NilPlan_KeepsSkippedSymlinks(t *testing.T) {
	symlinks := []sync.SkippedSymlink{
		{Path: "link.py", IsDir: false},
	}

	doc := planDocWithSymlinks(t, nil, symlinks)

	sym, ok := doc["skippedSymlinks"]
	require.True(t, ok, "the skippedSymlinks key must be emitted even for a nil plan")

	require.IsType(t, []any{}, sym)
	assert.Len(t, sym, 1)
}
