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
	"strings"
	"testing"
	"time"

	"github.com/datarobot/cli/internal/workload/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrintPlan_Empty(t *testing.T) {
	var buf bytes.Buffer

	require.NoError(t, PrintPlan(&buf, &sync.SyncPlan{}))
	assert.Equal(t, "Up to date.\n", buf.String())
}

func TestPrintPlan_Full(t *testing.T) {
	plan := &sync.SyncPlan{
		Uploads: []sync.FileAction{
			{Path: "agent.py", Classification: sync.ClsLocalModified, Action: sync.ActUploadModify, LocalSize: 1234},
			{Path: "new.py", Classification: sync.ClsLocalAdded, Action: sync.ActUploadAdd, LocalSize: 256},
		},
		Downloads: []sync.FileAction{
			{Path: "config.yaml", Classification: sync.ClsRemoteModified, Action: sync.ActDownloadModify, RemoteSize: 890},
		},
		Deletes: []sync.FileAction{
			{Path: "old.py", Classification: sync.ClsLocalDeleted, Action: sync.ActUploadDelete, LocalSize: 0},
		},
		Conflicts: []sync.FileAction{
			{Path: "shared.py", Classification: sync.ClsConflict, Action: sync.ActConflictCopy, RemoteSize: 100},
		},
	}

	var buf bytes.Buffer

	require.NoError(t, PrintPlan(&buf, plan))

	out := buf.String()
	assert.Contains(t, out, "Sync plan:")
	assert.Contains(t, out, "↑ UPLOAD (2):")
	assert.Contains(t, out, "M  agent.py")
	assert.Contains(t, out, "A  new.py")
	assert.Contains(t, out, "↓ DOWNLOAD (1):")
	assert.Contains(t, out, "✕ DELETE (1):")
	assert.Contains(t, out, "⚠ CONFLICT (remote wins) (1):")
	assert.Contains(t, out, "shared.py")
}

func TestPrintResult_FullCounts(t *testing.T) {
	r := &sync.Result{
		OldVersion:      "abcdefgh12345",
		NewVersion:      "12345678abcde",
		UploadedCount:   3,
		DownloadedCount: 2,
		DeletedCount:    1,
		ConflictCount:   1,
		ConflictCopies:  []string{"shared.py.LOCAL.20260410T143052Z"},
		Duration:        500 * time.Millisecond,
	}

	var buf bytes.Buffer

	require.NoError(t, PrintResult(&buf, r))

	out := stripANSI(buf.String())
	assert.Contains(t, out, "Sync complete: abcdefgh → 12345678  (↑3 ↓2 ✕1 ⚠1)")
	assert.Contains(t, out, "Conflict copies saved:")
	assert.Contains(t, out, "shared.py.LOCAL.20260410T143052Z")
}

func TestPrintResult_FirstSyncEmptyOld(t *testing.T) {
	r := &sync.Result{NewVersion: "ee0011ff", UploadedCount: 5}

	var buf bytes.Buffer

	require.NoError(t, PrintResult(&buf, r))
	assert.Contains(t, stripANSI(buf.String()), "∅ → ee0011ff")
}

func TestPrintResult_NoChanges(t *testing.T) {
	r := &sync.Result{OldVersion: "abcdefgh", NewVersion: "abcdefgh"}

	var buf bytes.Buffer

	require.NoError(t, PrintResult(&buf, r))
	assert.Contains(t, stripANSI(buf.String()), "(no changes)")
}

func stripANSI(s string) string {
	for {
		i := strings.Index(s, "\x1b[")
		if i == -1 {
			return s
		}

		j := strings.Index(s[i:], "m")
		if j == -1 {
			return s
		}

		s = s[:i] + s[i+j+1:]
	}
}

func TestFormatSize(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{1024 * 1024, "1.0 MiB"},
		{1024 * 1024 * 1024, "1.0 GiB"},
	}

	for _, tc := range cases {
		assert.Equal(t, tc.want, formatSize(tc.n))
	}
}
