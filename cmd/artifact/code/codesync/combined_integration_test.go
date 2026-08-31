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

package codesync

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests prove the display layer composes all three fixes: when
// symlinks, divergence, and other notices fire simultaneously, stdout stays
// pure JSON (or carries only the plan in human mode), stderr carries every
// notice, and no notice suppresses another. They use the fakeEngine so the
// display layer is what is under test, not the engine's detection logic
// (which is covered by the engine-level combined_integration_test.go).
//
// Fulfills the display-layer portions of VAL-CROSS-004(b), VAL-CROSS-008,
// and VAL-CROSS-009.

// ---------------------------------------------------------------------------
// VAL-CROSS-004(b): Symlink + divergence in JSON mode — pure stdout
// ---------------------------------------------------------------------------

// TestCmd_Combined_JSON_SymlinkAndDivergence_PureStdout proves that a
// --verify --dry-run --output-format json run with both a skipped symlink
// and a divergence produces stdout that decodes as valid JSON to EOF with
// both the skippedSymlinks field (non-empty) and the divergence field
// (naming the path) populated, while stderr carries both notices and no
// prose reaches stdout.
func TestCmd_Combined_JSON_SymlinkAndDivergence_PureStdout(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{
		plan: &sync.SyncPlan{},
		skippedSymlinks: []sync.SkippedSymlink{
			{Path: "link_to_file.py", IsDir: false},
		},
		divergences: []sync.Divergence{
			{Path: "app.py", Kind: sync.DivergenceHashMismatch, BaseHash: "aaaa", RemoteHash: "bbbb"},
		},
	}

	flags := map[string]string{
		"dir":           dir,
		"yes":           "true",
		"dry-run":       "true",
		"verify":        "true",
		"output-format": "json",
	}

	_, stdout, stderr, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err)

	raw := stdout.String()

	// stdout decodes as valid JSON to EOF.
	assertOnlyJSON(t, strings.NewReader(raw))

	// The JSON plan document contains both the skippedSymlinks field
	// (non-empty) and the divergence field (naming the path).
	dec := json.NewDecoder(strings.NewReader(raw))

	var doc struct {
		SkippedSymlinks []struct {
			Path  string `json:"path"`
			IsDir bool   `json:"isDir"`
		} `json:"skippedSymlinks"`
		Divergence []struct {
			Path string `json:"path"`
			Kind string `json:"kind"`
		} `json:"divergence"`
	}

	require.NoError(t, dec.Decode(&doc), "stdout must decode as the plan JSON document")

	require.Len(t, doc.SkippedSymlinks, 1, "the skippedSymlinks field must be non-empty")
	assert.Equal(t, "link_to_file.py", doc.SkippedSymlinks[0].Path)
	assert.False(t, doc.SkippedSymlinks[0].IsDir)

	require.Len(t, doc.Divergence, 1, "the divergence field must be non-empty")
	assert.Equal(t, "app.py", doc.Divergence[0].Path)
	assert.Equal(t, "hash_mismatch", doc.Divergence[0].Kind)

	// stderr carries both the symlink notice and the divergence prose.
	errText := stderr.String()
	assert.Contains(t, errText, "link_to_file.py", "stderr must carry the symlink notice")
	assert.Contains(t, errText, "app.py", "stderr must carry the divergence notice naming the path")
	assert.Contains(t, errText, "divergence", "stderr must carry the divergence prose")

	// No prose reaches stdout.
	assert.NotContains(t, raw, "was not uploaded", "no symlink prose on stdout")
	assert.NotContains(t, raw, "divergence:", "no divergence prose on stdout")
}

// ---------------------------------------------------------------------------
// VAL-CROSS-008: Maximal mixed plan in JSON mode — every field populated
// ---------------------------------------------------------------------------

// TestCmd_Combined_JSON_MaximalMixedPlan proves that a --verify --yes
// --output-format json run with every action type and both diagnostic types
// produces stdout that is pure JSON with every field populated, stderr
// carrying every notice, and exit code 0.
func TestCmd_Combined_JSON_MaximalMixedPlan(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{
		plan: &sync.SyncPlan{
			Uploads:   []sync.FileAction{{Path: "upload_me.py"}},
			Downloads: []sync.FileAction{{Path: "download_me.py"}},
			Deletes:   []sync.FileAction{{Path: "delete_me.py"}},
			Conflicts: []sync.FileAction{{Path: "conflict_me.py"}},
		},
		result: &sync.Result{
			NewVersion:      "ver-new",
			UploadedCount:   1,
			DownloadedCount: 1,
			DeletedCount:    1,
			ConflictCount:   1,
		},
		skippedSymlinks: []sync.SkippedSymlink{
			{Path: "link_to_file.py", IsDir: false},
			{Path: "link_to_dir", IsDir: true},
		},
		divergences: []sync.Divergence{
			{Path: "diverge_me.py", Kind: sync.DivergenceHashMismatch, BaseHash: "aaaa", RemoteHash: "bbbb"},
		},
	}

	flags := map[string]string{
		"dir":           dir,
		"yes":           "true",
		"verify":        "true",
		"output-format": "json",
	}

	_, stdout, stderr, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err, "exit code must be 0 for a successful sync with diagnostics")

	raw := stdout.String()

	// stdout is pure JSON to EOF (two documents: plan + result).
	assertOnlyJSON(t, strings.NewReader(raw))

	// Decode the first document (the plan).
	dec := json.NewDecoder(strings.NewReader(raw))

	var planDoc struct {
		Uploads         []map[string]any `json:"uploads"`
		Downloads       []map[string]any `json:"downloads"`
		Deletes         []map[string]any `json:"deletes"`
		Conflicts       []map[string]any `json:"conflicts"`
		SkippedSymlinks []map[string]any `json:"skippedSymlinks"`
		Divergence      []map[string]any `json:"divergence"`
	}

	require.NoError(t, dec.Decode(&planDoc), "the first document must be the plan")

	// Every JSON field is populated.
	assert.NotEmpty(t, planDoc.Uploads, "uploads field must be populated")
	assert.NotEmpty(t, planDoc.Downloads, "downloads field must be populated")
	assert.NotEmpty(t, planDoc.Deletes, "deletes field must be populated")
	assert.NotEmpty(t, planDoc.Conflicts, "conflicts field must be populated")
	assert.NotEmpty(t, planDoc.SkippedSymlinks, "skippedSymlinks field must be populated")
	assert.NotEmpty(t, planDoc.Divergence, "divergence field must be populated")

	// Decode the second document (the result).
	var resultDoc map[string]any
	require.NoError(t, dec.Decode(&resultDoc), "the second document must be the result")
	assert.Equal(t, "ver-new", resultDoc["newVersion"])

	// stderr carries the symlink, divergence, and conflict-resolution notices.
	errText := stderr.String()
	assert.Contains(t, errText, "link_to_file.py", "stderr must carry the file symlink notice")
	assert.Contains(t, errText, "link_to_dir", "stderr must carry the directory symlink notice")
	assert.Contains(t, errText, "divergence", "stderr must carry the divergence notice")
	assert.Contains(t, errText, "diverge_me.py", "stderr must name the divergent path")

	// No prose reaches stdout.
	assert.NotContains(t, raw, "was not uploaded", "no symlink prose on stdout")
	assert.NotContains(t, raw, "divergence:", "no divergence prose on stdout")
}

// ---------------------------------------------------------------------------
// VAL-CROSS-009: All notices at once — JSON and human mode
// ---------------------------------------------------------------------------

// TestCmd_Combined_AllNotices_JSONMode proves that in JSON mode, every notice
// type (state migration, .wapiignore shadow warning, symlinks, divergence)
// reaches stderr while stdout stays pure JSON. The fakeEngine carries all
// four notice types simultaneously, so the display layer is what is under
// test: no notice suppresses another, and stdout carries only JSON.
func TestCmd_Combined_AllNotices_JSONMode(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{
		plan:          &sync.SyncPlan{Uploads: []sync.FileAction{{Path: "a.py"}}},
		migrationNote: "Moved local state from .wapi/ to .datarobot/workload/.",
		ignoreNotice:  "Using .wapiignore for sync ignore patterns.",
		skippedSymlinks: []sync.SkippedSymlink{
			{Path: "link_to_file.py", IsDir: false},
			{Path: "link_to_dir", IsDir: true},
		},
		divergences: []sync.Divergence{
			{Path: "app.py", Kind: sync.DivergenceHashMismatch, BaseHash: "aaaa", RemoteHash: "bbbb"},
		},
	}

	flags := map[string]string{
		"dir":           dir,
		"yes":           "true",
		"dry-run":       "true",
		"verify":        "true",
		"output-format": "json",
	}

	_, stdout, stderr, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err)

	raw := stdout.String()

	// stdout is pure JSON to EOF.
	assertOnlyJSON(t, strings.NewReader(raw))

	// stderr carries every notice — no notice suppresses another.
	errText := stderr.String()
	assert.Contains(t, errText, "Moved local state", "state migration notice must reach stderr")
	assert.Contains(t, errText, ".wapiignore", "ignore file notice must reach stderr")
	assert.Contains(t, errText, "link_to_file.py", "file symlink notice must reach stderr")
	assert.Contains(t, errText, "link_to_dir", "directory symlink notice must reach stderr")
	assert.Contains(t, errText, "divergence", "divergence notice must reach stderr")
	assert.Contains(t, errText, "app.py", "divergence notice must name the path")

	// No notice leaks onto stdout.
	assert.NotContains(t, raw, "Moved local state", "no migration prose on stdout")
	assert.NotContains(t, raw, ".wapiignore", "no ignore prose on stdout")
	assert.NotContains(t, raw, "was not uploaded", "no symlink prose on stdout")
	assert.NotContains(t, raw, "divergence:", "no divergence prose on stdout")

	// stdout is independently reconstructable: it contains only the plan JSON.
	dec := json.NewDecoder(strings.NewReader(raw))

	var doc struct {
		Uploads         []map[string]any `json:"uploads"`
		SkippedSymlinks []map[string]any `json:"skippedSymlinks"`
		Divergence      []map[string]any `json:"divergence"`
	}

	require.NoError(t, dec.Decode(&doc))
	assert.NotEmpty(t, doc.Uploads, "the plan must carry the upload row")
	assert.Len(t, doc.SkippedSymlinks, 2, "the plan must carry both symlinks")
	assert.Len(t, doc.Divergence, 1, "the plan must carry the divergence")

	// stderr is independently reconstructable: every notice is present and
	// none is missing. The plan is NOT on stderr.
	assert.NotContains(t, errText, "a.py", "the plan must not leak onto stderr")
}

// TestCmd_Combined_AllNotices_HumanMode proves that in human mode, stdout
// carries only the plan and stderr carries every notice, so each stream is
// independently reconstructable.
func TestCmd_Combined_AllNotices_HumanMode(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{
		plan:          &sync.SyncPlan{Uploads: []sync.FileAction{{Path: "a.py"}}},
		migrationNote: "Moved local state from .wapi/ to .datarobot/workload/.",
		ignoreNotice:  "Using .wapiignore for sync ignore patterns.",
		skippedSymlinks: []sync.SkippedSymlink{
			{Path: "link_to_file.py", IsDir: false},
			{Path: "link_to_dir", IsDir: true},
		},
		divergences: []sync.Divergence{
			{Path: "app.py", Kind: sync.DivergenceHashMismatch, BaseHash: "aaaa", RemoteHash: "bbbb"},
		},
	}

	flags := map[string]string{
		"dir":     dir,
		"yes":     "true",
		"dry-run": "true",
		"verify":  "true",
	}

	_, stdout, stderr, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err)

	outText := stdout.String()
	errText := stderr.String()

	// stdout carries only the plan (the upload row). The negative checks pin
	// the actual command-summary substrings (not the per-phase prose the
	// fakeEngine never emits), so a StateNotice misrouted to stdout fails.
	assert.Contains(t, outText, "a.py", "stdout must carry the plan")
	assert.NotContains(t, outText, "Moved local state", "no migration notice on stdout")
	assert.NotContains(t, outText, ".wapiignore", "no ignore notice on stdout")
	assert.NotContains(t, outText, "were not uploaded or synced", "no symlink summary on stdout")
	assert.NotContains(t, outText, "--verify found", "no divergence summary on stdout")

	// stderr carries every notice — no notice suppresses another.
	assert.Contains(t, errText, "Moved local state", "state migration notice must reach stderr")
	assert.Contains(t, errText, ".wapiignore", "ignore file notice must reach stderr")
	assert.Contains(t, errText, "link_to_file.py", "file symlink notice must reach stderr")
	assert.Contains(t, errText, "link_to_dir", "directory symlink notice must reach stderr")
	assert.Contains(t, errText, "divergence", "divergence notice must reach stderr")
	assert.Contains(t, errText, "app.py", "divergence notice must name the path")

	// The plan is NOT on stderr.
	assert.NotContains(t, errText, "a.py", "the plan must not leak onto stderr")

	// Each stream is independently reconstructable: stdout has the plan,
	// stderr has every notice. Together they carry the full picture, but
	// neither duplicates the other.
}
