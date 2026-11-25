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

package sync

import (
	"strings"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/workload/wapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A push-only plan keeps what goes up and sets aside what would come down:
// downloads and remote deletes move to Skipped, local deletes still reach the
// remote, and a conflict stays where the command can refuse it.
func TestSyncPlan_PushOnlyLeavesRemoteChangesAlone(t *testing.T) {
	plan := &SyncPlan{
		Uploads:   []FileAction{{Path: "mine.py", Action: ActUploadModify}},
		Downloads: []FileAction{{Path: "theirs.py", Action: ActDownloadModify}, {Path: "new.py", Action: ActDownloadAdd}},
		Deletes:   []FileAction{{Path: "gone-here.py", Action: ActUploadDelete}, {Path: "gone-there.py", Action: ActDownloadDelete}},
		Conflicts: []FileAction{{Path: "both.py", Action: ActConflictCopy}},
	}

	plan.pushOnly()

	assert.Equal(t, []FileAction{{Path: "mine.py", Action: ActUploadModify}}, plan.Uploads)
	assert.Empty(t, plan.Downloads)
	assert.Equal(t, []FileAction{{Path: "gone-here.py", Action: ActUploadDelete}}, plan.Deletes)
	assert.Equal(t, []FileAction{{Path: "both.py", Action: ActConflictCopy}}, plan.Conflicts)
	assert.ElementsMatch(t, []string{"theirs.py", "new.py", "gone-there.py"}, paths(plan.Skipped))
	assert.False(t, plan.IsEmpty(), "the upload still counts")
}

// The diff phase applies the option, so every caller of Plan sees the same
// shape; without it the plan is unchanged.
func TestPhase3Diff_PushOnlyOption(t *testing.T) {
	base := BaseManifest{"theirs.py": {Hash: "aa", Size: 1}, "mine.py": {Hash: "bb", Size: 1}}
	local := LocalManifest{"theirs.py": {Hash: "aa", Size: 1}, "mine.py": {Hash: "cc", Size: 2}}
	remote := RemoteManifest{"theirs.py": {Hash: "dd", Size: 3}, "mine.py": {Hash: "bb", Size: 1}}

	plain := &Engine{base: base, local: local, remote: remote}
	require.NoError(t, phase3Diff(plain))
	assert.Equal(t, []string{"theirs.py"}, paths(plain.plan.Downloads))
	assert.Empty(t, plain.plan.Skipped)

	pushOnly := &Engine{opts: Options{PushOnly: true}, base: base, local: local, remote: remote}
	require.NoError(t, phase3Diff(pushOnly))
	assert.Empty(t, pushOnly.plan.Downloads)
	assert.Equal(t, []string{"theirs.py"}, paths(pushOnly.plan.Skipped))
	assert.Equal(t, []string{"mine.py"}, paths(pushOnly.plan.Uploads))
}

// A skipped file keeps its old base entry. Were the base to take the remote
// hash, the untouched local copy would read as an edit on the next plain sync
// and be uploaded over the teammate's change; with the old entry it reads as
// the remote change it is, and comes down then.
func TestBuildNewBaseManifest_SkippedKeepsTheOldBaseEntry(t *testing.T) {
	e := &Engine{
		base:   BaseManifest{"theirs.py": {Hash: "aa", Size: 1}, "same.py": {Hash: "ee", Size: 5}},
		remote: RemoteManifest{"theirs.py": {Hash: "dd", Size: 3}, "new.py": {Hash: "ff", Size: 6}, "same.py": {Hash: "ee", Size: 5}},
		plan: &SyncPlan{Skipped: []FileAction{
			{Path: "theirs.py", Action: ActDownloadModify},
			{Path: "new.py", Action: ActDownloadAdd},
		}},
	}

	manifest, err := buildNewBaseManifest(e, "ver-2", time.Now())
	require.NoError(t, err)

	assert.Equal(t, "aa", manifest.Files["theirs.py"].Hash, "the skipped download keeps the base it had")
	assert.NotContains(t, manifest.Files, "new.py", "a skipped addition stays unknown to the base")
	assert.Equal(t, "ee", manifest.Files["same.py"].Hash, "an untouched file follows the remote as before")
}

// Staging showed why the base alone is not enough: when the synced version
// equals the artifact's, the engine copies the remote from the base instead
// of listing it, so a kept entry would still hide the remote change and the
// next push-only run would upload the untouched local copy over it. The
// synced version advances as usual, since a deploy inherits the code from it,
// and the state marks that remote changes were set aside; the mark forces the
// listing and a run that applied everything clears it.
func TestPhase7State_SkippedMarksTheRemoteChangesAndKeepsTheVersionCurrent(t *testing.T) {
	// A first sync: no version yet, which used to write a time with no
	// version and corrupt the state.
	dir := initProject(t, nil)

	cfg, err := wapi.LoadConfig(dir)
	require.NoError(t, err)

	catalog := "6ac38bf84f3f11ca0434ce00"
	cfg.CatalogID = &catalog
	require.NoError(t, wapi.SaveConfig(dir, cfg))

	e := &Engine{
		projectDir:   dir,
		config:       cfg,
		nowFn:        time.Now,
		startedAt:    time.Now(),
		remote:       RemoteManifest{"theirs.py": {Hash: strings.Repeat("b", 64), Size: 2}},
		newVersionID: "6ac38c074f3f11ca0434ce02",
		plan:         &SyncPlan{Skipped: []FileAction{{Path: "theirs.py", Action: ActDownloadAdd}}},
	}

	require.NoError(t, phase7State(e))

	saved, err := wapi.LoadConfig(dir)
	require.NoError(t, err)
	assert.Equal(t, "6ac38c074f3f11ca0434ce02", *saved.LastSyncedVersionID, "the synced version is current")
	assert.True(t, saved.RemoteChangesSkipped, "and the state says the base is not the remote")
	assert.True(t, drifted("6ac38c074f3f11ca0434ce02", saved), "so the next sync lists the remote")

	_, err = wapi.LoadManifest(dir)
	require.NoError(t, err, "the base state loads")

	e.config = saved
	e.plan = &SyncPlan{}

	require.NoError(t, phase7State(e))

	saved, err = wapi.LoadConfig(dir)
	require.NoError(t, err)
	assert.False(t, saved.RemoteChangesSkipped, "a run that applied everything clears the mark")
	assert.False(t, drifted("6ac38c074f3f11ca0434ce02", saved))
}

// A run that produced no version records neither sync field: a time with no
// version is a state the loader refuses.
func TestBuildNewBaseManifest_NoVersionRecordsNoSyncTime(t *testing.T) {
	e := &Engine{remote: RemoteManifest{}, plan: &SyncPlan{}}

	manifest, err := buildNewBaseManifest(e, "", time.Now())
	require.NoError(t, err)
	assert.Nil(t, manifest.SyncedAt)
	assert.Nil(t, manifest.SyncedVersionID)
}

func paths(actions []FileAction) []string {
	out := make([]string, 0, len(actions))
	for _, fa := range actions {
		out = append(out, fa.Path)
	}

	return out
}
