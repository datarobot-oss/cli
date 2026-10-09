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

// sampleSkippedSymlinks is a file symlink plus a directory symlink, so every
// test exercises a notice that carries both kind distinctions.
func sampleSkippedSymlinks() []sync.SkippedSymlink {
	return []sync.SkippedSymlink{
		{Path: "link_to_file.py", IsDir: false},
		{Path: "link_to_dir", IsDir: true},
	}
}

// The symlink notice is a diagnostic, and diagnostics never belong on the
// data stream: it must reach stderr in human mode and stay off stdout.
func TestRunE_SymlinkNotice_ReachesStderrOnly(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{
		plan:            &sync.SyncPlan{},
		skippedSymlinks: sampleSkippedSymlinks(),
	}

	flags := map[string]string{"dir": dir, "yes": "true", "dry-run": "true"}

	_, stdout, stderr, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err)

	errText := stderr.String()

	// The summary names every symlink with its kind.
	assert.Contains(t, errText, "link_to_file.py")
	assert.Contains(t, errText, "link_to_dir")
	assert.Contains(t, errText, "file")
	assert.Contains(t, errText, "directory")

	assert.NotContains(t, stdout.String(), "link_to_file.py",
		"symlink prose must stay off the data stream")
	assert.NotContains(t, stdout.String(), "link_to_dir",
		"symlink prose must stay off the data stream")
}

// Under --output-format json the symlink data flows into the plan document
// as a structured field, and every trace of the prose stays on stderr so
// stdout decodes as JSON to EOF.
func TestRunE_JSON_SkippedSymlinkFieldPopulated(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{
		plan:            &sync.SyncPlan{},
		skippedSymlinks: sampleSkippedSymlinks(),
	}

	flags := map[string]string{"dir": dir, "yes": "true", "dry-run": "true", "output-format": "json"}

	_, stdout, stderr, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err)

	raw := stdout.String()

	assertOnlyJSON(t, strings.NewReader(raw))

	assert.Contains(t, stderr.String(), "link_to_file.py",
		"the symlink notice must remain on stderr in JSON mode")

	dec := json.NewDecoder(strings.NewReader(raw))

	var doc struct {
		SkippedSymlinks []struct {
			Path  string `json:"path"`
			IsDir bool   `json:"isDir"`
		} `json:"skippedSymlinks"`
	}

	require.NoError(t, dec.Decode(&doc), "the first stdout document must be the plan")
	require.Len(t, doc.SkippedSymlinks, 2, "every skipped symlink must be listed in the plan document")

	assert.Equal(t, "link_to_file.py", doc.SkippedSymlinks[0].Path)
	assert.False(t, doc.SkippedSymlinks[0].IsDir)

	assert.Equal(t, "link_to_dir", doc.SkippedSymlinks[1].Path)
	assert.True(t, doc.SkippedSymlinks[1].IsDir)
}

// A project with no symlinks leaves the field explicitly empty rather than
// omitted: a script must be able to tell "no symlinks" from "the field was
// never emitted" (the locked:false precedent).
func TestRunE_JSON_SkippedSymlinksExplicitlyEmptyWhenNone(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{plan: &sync.SyncPlan{}}

	flags := map[string]string{"dir": dir, "yes": "true", "dry-run": "true", "output-format": "json"}

	_, stdout, _, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err)

	raw := stdout.String()

	assertOnlyJSON(t, strings.NewReader(raw))
	assert.Contains(t, raw, `"skippedSymlinks": []`)
}

// The notice must fire in every mode — dry-run, diff, and a real sync — and
// even when the plan is empty. The exit code must be 0 in all of these.
func TestRunE_SymlinkNotice_AllModes_ExitZero(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags map[string]string
	}{
		{name: "dry-run", flags: map[string]string{"yes": "true", "dry-run": "true"}},
		{name: "diff", flags: map[string]string{"yes": "true", "diff": "true"}},
		{name: "real-sync", flags: map[string]string{"yes": "true"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			linkProject(t, dir)

			fe := &fakeEngine{
				plan:            &sync.SyncPlan{},
				skippedSymlinks: sampleSkippedSymlinks(),
				result:          &sync.Result{NewVersion: "v1"},
			}

			flags := map[string]string{"dir": dir}
			for k, v := range tc.flags {
				flags[k] = v
			}

			_, _, stderr, err := runWithDeps(t, fakeEngineDeps(fe), flags)
			require.NoError(t, err, "exit code must be 0 when the project contains skipped symlinks")

			assert.Contains(t, stderr.String(), "link_to_file.py",
				"the symlink notice must fire in %s mode", tc.name)
		})
	}
}

// The notice must fire even when the plan is empty / "Up to date.".
func TestRunE_SymlinkNotice_EmptyPlan(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{
		plan:            &sync.SyncPlan{},
		skippedSymlinks: sampleSkippedSymlinks(),
	}

	flags := map[string]string{"dir": dir, "yes": "true", "dry-run": "true"}

	_, _, stderr, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err)

	assert.Contains(t, stderr.String(), "link_to_file.py",
		"the symlink notice must fire even when the plan is empty")
}

// On a locked artifact containing a file symlink, --dry-run --yes emits both
// the symlink notice and the locked notice on stderr and prints the plan on
// stdout.
func TestRunE_SymlinkNotice_FiresAlongsideLockedNotice(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{
		plan: &sync.SyncPlan{
			Uploads: []sync.FileAction{{Path: "a.py"}},
		},
		lockedNote:      "Artifact art-1 is locked, so this is a preview only.",
		skippedSymlinks: []sync.SkippedSymlink{{Path: "link.py", IsDir: false}},
	}

	flags := map[string]string{"dir": dir, "yes": "true", "dry-run": "true"}

	_, stdout, stderr, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err)

	assert.Contains(t, stderr.String(), "is locked", "the locked notice must appear on stderr")
	assert.Contains(t, stderr.String(), "link.py", "the symlink notice must also appear on stderr")
	assert.Contains(t, stdout.String(), "a.py", "the plan must appear on stdout")
}

// When more than SymlinkNoticeBound symlinks are skipped, the stderr prose
// lists the first 5 in deterministic order followed by a count of the
// remainder, while the JSON field still lists every one.
func TestRunE_SymlinkNotice_BoundedProse_CompleteJSON(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	// Create 7 symlinks so the bound (5) is exceeded.
	symlinks := make([]sync.SkippedSymlink, 7)
	for i := range symlinks {
		symlinks[i] = sync.SkippedSymlink{
			Path:  "link" + string(rune('a'+i)) + ".py",
			IsDir: false,
		}
	}

	fe := &fakeEngine{
		plan:            &sync.SyncPlan{},
		skippedSymlinks: symlinks,
	}

	flags := map[string]string{"dir": dir, "yes": "true", "dry-run": "true", "output-format": "json"}

	_, stdout, stderr, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err)

	raw := stdout.String()

	assertOnlyJSON(t, strings.NewReader(raw))

	// The JSON field lists every symlink.
	dec := json.NewDecoder(strings.NewReader(raw))

	var doc struct {
		SkippedSymlinks []struct {
			Path  string `json:"path"`
			IsDir bool   `json:"isDir"`
		} `json:"skippedSymlinks"`
	}

	require.NoError(t, dec.Decode(&doc))
	require.Len(t, doc.SkippedSymlinks, 7, "the JSON field must list every symlink even when prose is bounded")

	// The stderr prose is bounded: mentions the first 5 and the count of the remainder.
	errText := stderr.String()
	assert.Contains(t, errText, "linka.py")
	assert.Contains(t, errText, "linkb.py")
	assert.Contains(t, errText, "linkc.py")
	assert.Contains(t, errText, "linkd.py")
	assert.Contains(t, errText, "linke.py")
	assert.NotContains(t, errText, "linkf.py", "the 6th symlink must not appear in the bounded prose")
	assert.NotContains(t, errText, "linkg.py", "the 7th symlink must not appear in the bounded prose")
	assert.Contains(t, errText, "2 more", "the count of the remainder must appear")
}

// A symlink path never appears in any action list; it exists only in the
// skipped-symlink notice/field.
func TestRunE_JSON_SymlinkPathNotInActionLists(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	fe := &fakeEngine{
		plan: &sync.SyncPlan{
			Uploads: []sync.FileAction{{Path: "real.py"}},
		},
		skippedSymlinks: []sync.SkippedSymlink{{Path: "link.py", IsDir: false}},
	}

	flags := map[string]string{"dir": dir, "yes": "true", "dry-run": "true", "output-format": "json"}

	_, stdout, _, err := runWithDeps(t, fakeEngineDeps(fe), flags)
	require.NoError(t, err)

	raw := stdout.String()

	dec := json.NewDecoder(strings.NewReader(raw))

	var doc struct {
		Uploads         []map[string]any `json:"uploads"`
		Downloads       []map[string]any `json:"downloads"`
		Deletes         []map[string]any `json:"deletes"`
		Conflicts       []map[string]any `json:"conflicts"`
		SkippedSymlinks []map[string]any `json:"skippedSymlinks"`
	}

	require.NoError(t, dec.Decode(&doc))

	// The symlink path is only in skippedSymlinks.
	require.Len(t, doc.SkippedSymlinks, 1)
	assert.Equal(t, "link.py", doc.SkippedSymlinks[0]["path"])

	// It is not in any action list.
	for _, u := range doc.Uploads {
		assert.NotEqual(t, "link.py", u["path"], "symlink path must not appear in uploads")
	}

	for _, d := range doc.Deletes {
		assert.NotEqual(t, "link.py", d["path"], "symlink path must not appear in deletes")
	}

	for _, d := range doc.Downloads {
		assert.NotEqual(t, "link.py", d["path"], "symlink path must not appear in downloads")
	}

	for _, c := range doc.Conflicts {
		assert.NotEqual(t, "link.py", c["path"], "symlink path must not appear in conflicts")
	}
}

// Two runs against an unchanged project produce the same set, order and
// notice text (deterministic across runs).
func TestRunE_SymlinkNotice_DeterministicAcrossRuns(t *testing.T) {
	dir := t.TempDir()
	linkProject(t, dir)

	symlinks := []sync.SkippedSymlink{
		{Path: "z_link.py", IsDir: false},
		{Path: "a_link.py", IsDir: false},
		{Path: "m_link", IsDir: true},
	}

	makeEngine := func() *fakeEngine {
		return &fakeEngine{
			plan:            &sync.SyncPlan{},
			skippedSymlinks: symlinks,
		}
	}

	flags := map[string]string{"dir": dir, "yes": "true", "dry-run": "true"}

	_, _, stderr1, err := runWithDeps(t, fakeEngineDeps(makeEngine()), flags)
	require.NoError(t, err)

	_, _, stderr2, err := runWithDeps(t, fakeEngineDeps(makeEngine()), flags)
	require.NoError(t, err)

	// The notice text is identical across runs (sorted by path).
	assert.Equal(t, stderr1.String(), stderr2.String(),
		"two runs against an unchanged project must produce the same notice text")

	// The sorted order is a_link.py, m_link, z_link.py.
	errText := stderr1.String()
	idxA := strings.Index(errText, "a_link.py")
	idxM := strings.Index(errText, "m_link")
	idxZ := strings.Index(errText, "z_link.py")
	require.True(t, idxA >= 0 && idxM >= 0 && idxZ >= 0, "all three symlinks must appear")
	assert.True(t, idxA < idxM && idxM < idxZ, "symlinks must be listed in sorted path order")
}
