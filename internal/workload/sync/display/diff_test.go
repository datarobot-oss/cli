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
	"errors"
	"testing"

	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeFetcher struct {
	local     map[string]string
	remote    map[string]string
	localErr  error
	remoteErr error
}

func (f *fakeFetcher) LocalContent(path string) ([]byte, error) {
	if f.localErr != nil {
		return nil, f.localErr
	}

	v, ok := f.local[path]
	if !ok {
		return nil, errors.New("local not found: " + path)
	}

	return []byte(v), nil
}

func (f *fakeFetcher) RemoteContent(path string) ([]byte, error) {
	if f.remoteErr != nil {
		return nil, f.remoteErr
	}

	v, ok := f.remote[path]
	if !ok {
		return nil, errors.New("remote not found: " + path)
	}

	return []byte(v), nil
}

func TestPrintDiffs(t *testing.T) {
	cases := []struct {
		name     string
		plan     *sync.SyncPlan
		local    map[string]string
		remote   map[string]string
		want     []string
		notWant  []string
		assertEq func(t *testing.T, out string)
	}{
		{
			name: "LOCAL_ADDED renders new file content",
			plan: &sync.SyncPlan{Uploads: []sync.FileAction{
				{Path: "test.py", Classification: sync.ClsLocalAdded, Action: sync.ActUploadAdd},
			}},
			local: map[string]string{"test.py": "print('hi')\n"},
			want:  []string{"+++ test.py (local; new file)", "--- /dev/null", "print('hi')"},
		},
		{
			name: "LOCAL_MODIFIED renders both versions",
			plan: &sync.SyncPlan{Uploads: []sync.FileAction{
				{Path: "agent.py", Classification: sync.ClsLocalModified, Action: sync.ActUploadModify},
			}},
			// Disjoint character sets so per-side runs stay contiguous
			// in dmp.DiffPrettyText output.
			local:  map[string]string{"agent.py": "AAAA\n"},
			remote: map[string]string{"agent.py": "ZZZZ\n"},
			want:   []string{"--- agent.py (remote)", "+++ agent.py (local)", "AAAA", "ZZZZ"},
		},
		{
			name: "LOCAL_DELETED renders deleted content",
			plan: &sync.SyncPlan{Deletes: []sync.FileAction{
				{Path: "old.py", Classification: sync.ClsLocalDeleted, Action: sync.ActUploadDelete},
			}},
			remote: map[string]string{"old.py": "going away\n"},
			want:   []string{"--- old.py (remote)", "+++ /dev/null", "going away"},
		},
		{
			name: "REMOTE_ADDED renders pulled content",
			plan: &sync.SyncPlan{Downloads: []sync.FileAction{
				{Path: "pulled.py", Classification: sync.ClsRemoteAdded, Action: sync.ActDownloadAdd},
			}},
			remote: map[string]string{"pulled.py": "from remote\n"},
			want:   []string{"--- /dev/null", "+++ pulled.py (remote; new file)", "from remote"},
		},
		{
			name: "REMOTE_MODIFIED renders both versions",
			plan: &sync.SyncPlan{Downloads: []sync.FileAction{
				{Path: "config.py", Classification: sync.ClsRemoteModified, Action: sync.ActDownloadModify},
			}},
			local:  map[string]string{"config.py": "AAAA\n"},
			remote: map[string]string{"config.py": "ZZZZ\n"},
			want:   []string{"--- config.py (local)", "+++ config.py (remote)", "AAAA", "ZZZZ"},
		},
		{
			name: "REMOTE_DELETED renders local content as deletion",
			plan: &sync.SyncPlan{Deletes: []sync.FileAction{
				{Path: "removed.py", Classification: sync.ClsRemoteDeleted, Action: sync.ActDownloadDelete},
			}},
			local: map[string]string{"removed.py": "still here\n"},
			want:  []string{"--- removed.py (local)", "+++ /dev/null", "still here"},
		},
		{
			name: "CONFLICT renders local vs remote",
			plan: &sync.SyncPlan{Conflicts: []sync.FileAction{
				{Path: "shared.py", Classification: sync.ClsConflict, Action: sync.ActConflictCopy},
			}},
			local:  map[string]string{"shared.py": "AAAA\n"},
			remote: map[string]string{"shared.py": "ZZZZ\n"},
			want:   []string{"--- shared.py (local)", "+++ shared.py (remote, wins)", "AAAA", "ZZZZ"},
		},
		{
			name: "ADD_CONFLICT renders local vs remote",
			plan: &sync.SyncPlan{Conflicts: []sync.FileAction{
				{Path: "race.py", Classification: sync.ClsAddConflict, Action: sync.ActConflictCopy},
			}},
			local:  map[string]string{"race.py": "11111\n"},
			remote: map[string]string{"race.py": "99999\n"},
			want:   []string{"--- race.py (local)", "+++ race.py (remote, wins)", "11111", "99999"},
		},
		{
			name: "DEL_EDIT_CONFLICT renders the local edit as a deletion",
			plan: &sync.SyncPlan{Conflicts: []sync.FileAction{
				{Path: "doomed.py", Classification: sync.ClsDelEditConflict, Action: sync.ActConflictCopy},
			}},
			local: map[string]string{"doomed.py": "local edit text\n"},
			want:  []string{"--- doomed.py (local edit)", "+++ /dev/null (remote deleted)", "local edit text"},
		},
		{
			name: "EDIT_DEL_CONFLICT renders remote as restoration",
			plan: &sync.SyncPlan{Downloads: []sync.FileAction{
				{Path: "restored.py", Classification: sync.ClsEditDelConflict, Action: sync.ActDownloadOverDel},
			}},
			remote: map[string]string{"restored.py": "remote restored\n"},
			want:   []string{"--- /dev/null (local deleted)", "+++ restored.py (remote)", "remote restored"},
		},
		{
			name: "UNCHANGED is skipped",
			plan: &sync.SyncPlan{Uploads: []sync.FileAction{
				{Path: "same.py", Classification: sync.ClsUnchanged, Action: sync.ActSkip},
			}},
			notWant: []string{"same.py"},
			assertEq: func(t *testing.T, out string) {
				assert.Empty(t, out, "UNCHANGED row must produce no output")
			},
		},
		{
			name: "fetcher error is reported inline, not fatal",
			plan: &sync.SyncPlan{Uploads: []sync.FileAction{
				{Path: "broken.py", Classification: sync.ClsLocalModified, Action: sync.ActUploadModify},
			}},
			local: map[string]string{"broken.py": "ok\n"},
			want:  []string{"--- broken.py", "*** ", "remote not found: broken.py", " ***"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fetcher := &fakeFetcher{local: tc.local, remote: tc.remote}

			var buf bytes.Buffer

			err := PrintDiffs(&buf, tc.plan, fetcher)
			require.NoError(t, err)

			out := buf.String()

			for _, frag := range tc.want {
				assert.Contains(t, out, frag, "expected output to contain %q\n--- output ---\n%s", frag, out)
			}

			for _, frag := range tc.notWant {
				assert.NotContains(t, out, frag)
			}

			if tc.assertEq != nil {
				tc.assertEq(t, out)
			}
		})
	}
}

func TestPrintDiffs_NilPlan_NoOp(t *testing.T) {
	var buf bytes.Buffer

	err := PrintDiffs(&buf, nil, &fakeFetcher{})
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestPrintDiffs_NilFetcher_NoOp(t *testing.T) {
	plan := &sync.SyncPlan{Uploads: []sync.FileAction{
		{Path: "x.py", Classification: sync.ClsLocalAdded, Action: sync.ActUploadAdd},
	}}

	var buf bytes.Buffer

	err := PrintDiffs(&buf, plan, nil)
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestPrintDiffs_EmptyPlan(t *testing.T) {
	var buf bytes.Buffer

	err := PrintDiffs(&buf, &sync.SyncPlan{}, &fakeFetcher{})
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestShouldDiff_AllClassifications(t *testing.T) {
	cases := map[sync.Classification]bool{
		sync.ClsLocalAdded:      true,
		sync.ClsLocalModified:   true,
		sync.ClsLocalDeleted:    true,
		sync.ClsRemoteAdded:     true,
		sync.ClsRemoteModified:  true,
		sync.ClsRemoteDeleted:   true,
		sync.ClsConflict:        true,
		sync.ClsAddConflict:     true,
		sync.ClsDelEditConflict: true,
		sync.ClsEditDelConflict: true,
		sync.ClsUnchanged:       false,
		sync.ClsConverged:       false,
		sync.ClsBothDeleted:     false,
		sync.ClsBothAddedSame:   false,
	}

	for cls, want := range cases {
		got := shouldDiff(sync.FileAction{Classification: cls})
		assert.Equal(t, want, got, "shouldDiff(%s)", cls)
	}
}

func TestHeaderFor_HasDevNullForAddDelete(t *testing.T) {
	cases := []struct {
		cls       sync.Classification
		left, rt  string
		bothSides bool
	}{
		{sync.ClsLocalAdded, devNull, "(local; new file)", false},
		{sync.ClsLocalDeleted, "(remote)", devNull, false},
		{sync.ClsRemoteAdded, devNull, "(remote; new file)", false},
		{sync.ClsRemoteDeleted, "(local)", devNull, false},
		{sync.ClsDelEditConflict, "(local edit)", devNull, false},
		{sync.ClsEditDelConflict, devNull, "(remote)", false},
	}

	for _, tc := range cases {
		l, r := headerFor(sync.FileAction{Path: "x.py", Classification: tc.cls})
		assert.Contains(t, l, tc.left, "%s left header", tc.cls)
		assert.Contains(t, r, tc.rt, "%s right header", tc.cls)
	}
}
