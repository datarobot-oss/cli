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
	"slices"
	"testing"

	"github.com/datarobot/cli/internal/drapi/filesapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func entry(hash string, size int64) FileEntry { return FileEntry{Hash: hash, Size: size} }

func TestDiff_FastPathUnchanged(t *testing.T) {
	base := BaseManifest{
		"a.py": entry("aaa", 1),
		"b.py": entry("bbb", 2),
	}
	plan := Diff(base, base, base)

	assert.True(t, plan.IsEmpty())
	assert.False(t, plan.HasConflicts())
}

func TestDiff_PushOnly(t *testing.T) {
	base := BaseManifest{"a.py": entry("aaa", 10)}
	local := BaseManifest{"a.py": entry("aaa-local", 11), "new.py": entry("new", 5)}
	remote := base // remote unchanged

	plan := Diff(base, local, remote)
	plan.Sort()

	assert.Len(t, plan.Uploads, 2)
	assert.Empty(t, plan.Downloads)
	assert.Empty(t, plan.Deletes)
	assert.Empty(t, plan.Conflicts)
	assert.Equal(t, "a.py", plan.Uploads[0].Path)
	assert.Equal(t, ClsLocalModified, plan.Uploads[0].Classification)
	assert.Equal(t, "new.py", plan.Uploads[1].Path)
	assert.Equal(t, ClsLocalAdded, plan.Uploads[1].Classification)
}

func TestDiff_PullOnly(t *testing.T) {
	base := BaseManifest{"a.py": entry("aaa", 10)}
	local := base // local unchanged
	remote := BaseManifest{"a.py": entry("aaa-remote", 12), "new.py": entry("rem", 7)}

	plan := Diff(base, local, remote)
	plan.Sort()

	assert.Empty(t, plan.Uploads)
	assert.Len(t, plan.Downloads, 2)
	assert.Empty(t, plan.Deletes)
	assert.Empty(t, plan.Conflicts)
}

func TestDiff_Conflict(t *testing.T) {
	base := BaseManifest{"shared.py": entry("X", 10)}
	local := BaseManifest{"shared.py": entry("Y", 11)}
	remote := BaseManifest{"shared.py": entry("Z", 12)}

	plan := Diff(base, local, remote)

	assert.Empty(t, plan.Uploads)
	assert.Empty(t, plan.Downloads)
	assert.Len(t, plan.Conflicts, 1)
	assert.True(t, plan.HasConflicts())
	assert.Equal(t, ClsConflict, plan.Conflicts[0].Classification)
}

func TestDiff_Deletes(t *testing.T) {
	base := BaseManifest{
		"a.py": entry("aaa", 10),
		"b.py": entry("bbb", 20),
	}
	local := BaseManifest{"a.py": entry("aaa", 10)}  // user deleted b.py
	remote := BaseManifest{"b.py": entry("bbb", 20)} // teammate deleted a.py

	plan := Diff(base, local, remote)
	plan.Sort()

	assert.Empty(t, plan.Uploads)
	assert.Empty(t, plan.Downloads)
	assert.Len(t, plan.Deletes, 2)

	gotPaths := []string{plan.Deletes[0].Path, plan.Deletes[1].Path}
	assert.Contains(t, gotPaths, "a.py")
	assert.Contains(t, gotPaths, "b.py")
}

// A system-excluded path an older CLI uploaded is deleted from the remote,
// never downloaded: the walk does not list it locally, so left to the plain
// classification it reads as added or edited on the remote and lands on top
// of the real local file.
func TestDiff_SystemExcludedRemoteFilesAreDeletedThere(t *testing.T) {
	for name, tc := range map[string]struct{ base, remote BaseManifest }{
		"no base: would be REMOTE_ADDED": {
			remote: BaseManifest{"sub/.git/HEAD": entry("ref", 23)},
		},
		"base and a remote edit: would be EDIT_DEL_CONFLICT": {
			base:   BaseManifest{"sub/.git/HEAD": entry("ref", 23)},
			remote: BaseManifest{"sub/.git/HEAD": entry("other", 24)},
		},
		"base and no change: already LOCAL_DELETED": {
			base:   BaseManifest{"sub/.git/HEAD": entry("ref", 23)},
			remote: BaseManifest{"sub/.git/HEAD": entry("ref", 23)},
		},
	} {
		t.Run(name, func(t *testing.T) {
			local := BaseManifest{"agent.py": entry("aaa", 10)}
			if tc.remote["agent.py"].Hash == "" {
				tc.remote["agent.py"] = entry("aaa", 10)
			}

			plan := Diff(tc.base, local, tc.remote)

			assert.Empty(t, plan.Downloads, "nothing may land over the local file")
			assert.Empty(t, plan.Conflicts)
			require.Len(t, plan.Deletes, 1)
			assert.Equal(t, "sub/.git/HEAD", plan.Deletes[0].Path)
			assert.Equal(t, ClsLocalDeleted, plan.Deletes[0].Classification)
			assert.Equal(t, ActUploadDelete, plan.Deletes[0].Action, "the delete goes to the remote")
		})
	}

	// Gone from the remote already: nothing to do, whatever the base says.
	plan := Diff(BaseManifest{"sub/.git/HEAD": entry("ref", 23)}, BaseManifest{}, BaseManifest{})
	assert.Empty(t, plan.Deletes)
	assert.Empty(t, plan.Downloads)
}

func TestDiff_BytesAccounting(t *testing.T) {
	base := BaseManifest{}
	local := BaseManifest{"big.bin": entry("L", 1024)}
	remote := BaseManifest{"other.bin": entry("R", 2048)}

	plan := Diff(base, local, remote)

	assert.Equal(t, int64(1024), plan.TotalUploadBytes())
	assert.Equal(t, int64(2048), plan.TotalDownloadBytes())
}

func TestFromFilesAPI_RoundTrip(t *testing.T) {
	in := map[string]filesapi.FileMeta{
		"a.py": {Hash: "aaa", Size: 10},
		"b.py": {Hash: "bbb", Size: 20},
	}

	got := FromFilesAPI(in)

	assert.Len(t, got, 2)
	assert.Equal(t, FileEntry{Hash: "aaa", Size: 10}, got["a.py"])
	assert.Equal(t, FileEntry{Hash: "bbb", Size: 20}, got["b.py"])
}

// A path reached through a link the walk followed is pushed whatever the
// remote did: pulling it would write through the link into what it points at.
func TestDiff_LinkedPathsArePushedNeverPulled(t *testing.T) {
	linked := []string{"lib", "config.py"}

	for name, tc := range map[string]struct {
		path                string
		base, local, remote string
		want                Action
		replaced            bool
	}{
		"remote changed a linked file": {
			path: "lib/x.py", base: "aaa", local: "aaa", remote: "rem", want: ActUploadModify, replaced: true,
		},
		"both changed a linked file": {
			path: "lib/x.py", base: "aaa", local: "loc", remote: "rem", want: ActUploadModify, replaced: true,
		},
		"local changed a linked file": {
			path: "lib/x.py", base: "aaa", local: "loc", remote: "aaa", want: ActUploadModify,
		},
		"remote changed a linked file at the link itself": {
			path: "config.py", base: "aaa", local: "aaa", remote: "rem", want: ActUploadModify, replaced: true,
		},
		"remote added a file under a linked directory": {
			path: "lib/new.py", remote: "rem", want: ActUploadDelete, replaced: true,
		},
		"remote deleted a linked file": {
			path: "lib/x.py", base: "aaa", local: "aaa", want: ActUploadAdd, replaced: true,
		},
		"remote changed a file beside the link, not under it": {
			path: "library/x.py", base: "aaa", local: "aaa", remote: "rem", want: ActDownloadModify,
		},
	} {
		t.Run(name, func(t *testing.T) {
			manifest := func(hash string) BaseManifest {
				if hash == "" {
					return BaseManifest{}
				}

				return BaseManifest{tc.path: entry(hash, 1)}
			}

			plan := diff(manifest(tc.base), manifest(tc.local), manifest(tc.remote), linked, nil)

			rows := slices.Concat(plan.Uploads, plan.Downloads, plan.Deletes, plan.Conflicts)
			require.Len(t, rows, 1)
			assert.Equal(t, tc.want, rows[0].Action)
			assert.Equal(t, tc.replaced, slices.Contains(plan.ReplacedRemote, tc.path), "replaced remote")
		})
	}
}

// With lib linked to shared, lib/x.py is shared/x.py by another name: it ends
// up holding whatever the plan leaves in shared/x.py, and is pushed as that.
func TestDiff_AFileReachedThroughALinkFollowsTheFileItIs(t *testing.T) {
	const own, alias = "shared/x.py", "lib/x.py"

	aliases := map[string]string{alias: own}
	both := func(hash string) BaseManifest { return BaseManifest{own: entry(hash, 1), alias: entry(hash, 1)} }

	for name, tc := range map[string]struct {
		local, remote BaseManifest
		want          map[string]Action
		replaced      []string
	}{
		"a teammate with the link deleted it": {
			local: both("aaa"), remote: BaseManifest{},
			want: map[string]Action{own: ActDownloadDelete},
		},
		"a teammate without the link deleted it": {
			local: both("aaa"), remote: BaseManifest{alias: entry("aaa", 1)},
			want: map[string]Action{own: ActDownloadDelete, alias: ActUploadDelete},
		},
		"a teammate with the link edited it": {
			local: both("aaa"), remote: both("rem"),
			want: map[string]Action{own: ActDownloadModify},
		},
		"it was edited here": {
			local: both("loc"), remote: both("aaa"),
			want: map[string]Action{own: ActUploadModify, alias: ActUploadModify},
		},
		"the server copy under the link was edited": {
			local: both("aaa"), remote: BaseManifest{own: entry("aaa", 1), alias: entry("rem", 1)},
			want:     map[string]Action{alias: ActUploadModify},
			replaced: []string{alias},
		},
	} {
		t.Run(name, func(t *testing.T) {
			plan := diff(both("aaa"), tc.local, tc.remote, []string{"lib"}, aliases)
			plan.planAliases(aliases, both("aaa"), tc.local, tc.remote)

			got := map[string]Action{}
			for _, fa := range slices.Concat(plan.Uploads, plan.Downloads, plan.Deletes, plan.Conflicts) {
				got[fa.Path] = fa.Action
			}

			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.replaced, plan.ReplacedRemote)
		})
	}
}
