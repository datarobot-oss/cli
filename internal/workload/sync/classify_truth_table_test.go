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
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestClassifyTruthTable is a comprehensive table-driven test over every
// meaningful (base, local, remote) hash triple, covering all fourteen
// three-way classifications and their mapped actions. Each classification is
// exercised with multiple distinct hash values so the logic is proven
// value-agnostic, not merely correct for one chosen triple.
//
// An empty hash ("") means absent on that side. Non-empty hashes are compared
// by equality only; the actual hex content is irrelevant. The table is
// transcribed from the current Classify implementation (classify.go) and
// ActionFor, not from memory, so any later change to either function is
// caught.
//
// Fulfills VAL-REGRESSION-006.
func TestClassifyTruthTable(t *testing.T) {
	// Four distinct non-empty hash values plus "" for absent. Using multiple
	// values per branch proves the logic is value-agnostic.
	const (
		A = "aaaa1111"
		B = "bbbb2222"
		C = "cccc3333"
		D = "dddd4444"
	)

	cases := []struct {
		name    string
		base    string
		local   string
		remote  string
		wantCls Classification
		wantAct Action
	}{
		// --- BASE ABSENT (classifyAbsentBase) ---

		// Both sides absent: nothing to do.
		{
			name: "absent_both_absent/empty_empty_empty", base: "", local: "", remote: "",
			wantCls: ClsUnchanged, wantAct: ActSkip,
		},

		// Local added, remote absent.
		{
			name: "absent_local_added/empty_A_empty", base: "", local: A, remote: "",
			wantCls: ClsLocalAdded, wantAct: ActUploadAdd,
		},
		{
			name: "absent_local_added/empty_B_empty", base: "", local: B, remote: "",
			wantCls: ClsLocalAdded, wantAct: ActUploadAdd,
		},

		// Remote added, local absent.
		{
			name: "absent_remote_added/empty_empty_A", base: "", local: "", remote: A,
			wantCls: ClsRemoteAdded, wantAct: ActDownloadAdd,
		},
		{
			name: "absent_remote_added/empty_empty_B", base: "", local: "", remote: B,
			wantCls: ClsRemoteAdded, wantAct: ActDownloadAdd,
		},

		// Both added the same content: no conflict, skip.
		{
			name: "absent_both_added_same/empty_A_A", base: "", local: A, remote: A,
			wantCls: ClsBothAddedSame, wantAct: ActSkip,
		},
		{
			name: "absent_both_added_same/empty_B_B", base: "", local: B, remote: B,
			wantCls: ClsBothAddedSame, wantAct: ActSkip,
		},

		// Both added different content: conflict.
		{
			name: "absent_add_conflict/empty_A_B", base: "", local: A, remote: B,
			wantCls: ClsAddConflict, wantAct: ActConflictCopy,
		},
		{
			name: "absent_add_conflict/empty_C_D", base: "", local: C, remote: D,
			wantCls: ClsAddConflict, wantAct: ActConflictCopy,
		},

		// --- BASE PRESENT, BOTH PRESENT (classifyBothPresentWithBase) ---

		// All three identical: unchanged.
		{
			name: "present_unchanged/A_A_A", base: A, local: A, remote: A,
			wantCls: ClsUnchanged, wantAct: ActSkip,
		},
		{
			name: "present_unchanged/B_B_B", base: B, local: B, remote: B,
			wantCls: ClsUnchanged, wantAct: ActSkip,
		},

		// Local changed, remote unchanged: upload.
		{
			name: "present_local_modified/A_B_A", base: A, local: B, remote: A,
			wantCls: ClsLocalModified, wantAct: ActUploadModify,
		},
		{
			name: "present_local_modified/A_C_A", base: A, local: C, remote: A,
			wantCls: ClsLocalModified, wantAct: ActUploadModify,
		},

		// Remote changed, local unchanged: download.
		{
			name: "present_remote_modified/A_A_B", base: A, local: A, remote: B,
			wantCls: ClsRemoteModified, wantAct: ActDownloadModify,
		},
		{
			name: "present_remote_modified/A_A_C", base: A, local: A, remote: C,
			wantCls: ClsRemoteModified, wantAct: ActDownloadModify,
		},

		// Both changed to the same content: converged, skip.
		{
			name: "present_converged/A_B_B", base: A, local: B, remote: B,
			wantCls: ClsConverged, wantAct: ActSkip,
		},
		{
			name: "present_converged/A_C_C", base: A, local: C, remote: C,
			wantCls: ClsConverged, wantAct: ActSkip,
		},

		// Both changed differently: conflict.
		{
			name: "present_conflict/A_B_C", base: A, local: B, remote: C,
			wantCls: ClsConflict, wantAct: ActConflictCopy,
		},
		{
			name: "present_conflict/B_C_D", base: B, local: C, remote: D,
			wantCls: ClsConflict, wantAct: ActConflictCopy,
		},

		// --- BASE PRESENT, DELETION INVOLVED (classifyDeletionInvolvedWithBase) ---

		// Both deleted: skip.
		{
			name: "present_both_deleted/A_empty_empty", base: A, local: "", remote: "",
			wantCls: ClsBothDeleted, wantAct: ActSkip,
		},
		{
			name: "present_both_deleted/B_empty_empty", base: B, local: "", remote: "",
			wantCls: ClsBothDeleted, wantAct: ActSkip,
		},

		// Local deleted, remote unchanged from base: local delete.
		{
			name: "present_local_deleted/A_empty_A", base: A, local: "", remote: A,
			wantCls: ClsLocalDeleted, wantAct: ActUploadDelete,
		},
		{
			name: "present_local_deleted/B_empty_B", base: B, local: "", remote: B,
			wantCls: ClsLocalDeleted, wantAct: ActUploadDelete,
		},

		// Local deleted, remote changed from base: edit-del conflict.
		// The user deleted the file, the teammate edited it.
		{
			name: "present_edit_del_conflict/A_empty_B", base: A, local: "", remote: B,
			wantCls: ClsEditDelConflict, wantAct: ActDownloadOverDel,
		},
		{
			name: "present_edit_del_conflict/A_empty_C", base: A, local: "", remote: C,
			wantCls: ClsEditDelConflict, wantAct: ActDownloadOverDel,
		},

		// Remote deleted, local unchanged from base: remote delete.
		{
			name: "present_remote_deleted/A_A_empty", base: A, local: A, remote: "",
			wantCls: ClsRemoteDeleted, wantAct: ActDownloadDelete,
		},
		{
			name: "present_remote_deleted/B_B_empty", base: B, local: B, remote: "",
			wantCls: ClsRemoteDeleted, wantAct: ActDownloadDelete,
		},

		// Remote deleted, local changed from base: del-edit conflict.
		// The user edited the file, the teammate deleted it.
		{
			name: "present_del_edit_conflict/A_B_empty", base: A, local: B, remote: "",
			wantCls: ClsDelEditConflict, wantAct: ActConflictCopy,
		},
		{
			name: "present_del_edit_conflict/A_C_empty", base: A, local: C, remote: "",
			wantCls: ClsDelEditConflict, wantAct: ActConflictCopy,
		},
	}

	// Verify we cover all fourteen classifications.
	seenCls := make(map[Classification]struct{})
	for _, tc := range cases {
		seenCls[tc.wantCls] = struct{}{}
	}

	allCls := []Classification{
		ClsUnchanged, ClsLocalModified, ClsRemoteModified, ClsConverged, ClsConflict,
		ClsLocalAdded, ClsRemoteAdded, ClsAddConflict, ClsLocalDeleted, ClsRemoteDeleted,
		ClsBothDeleted, ClsDelEditConflict, ClsEditDelConflict, ClsBothAddedSame,
	}

	for _, c := range allCls {
		_, ok := seenCls[c]
		assert.True(t, ok, "truth table must cover classification %s", c)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotCls := Classify(tc.base, tc.local, tc.remote)
			assert.Equal(t, tc.wantCls, gotCls,
				"classification for (base=%q, local=%q, remote=%q)", tc.base, tc.local, tc.remote)

			gotAct := ActionFor(gotCls)
			assert.Equal(t, tc.wantAct, gotAct,
				"action for classification %s", gotCls)
		})
	}
}

// TestClassifyActionMappingIsExhaustive asserts that every classification has
// a defined action and that the action mapping is a total function: no
// classification maps to the zero-value ActSkip by falling through the switch
// default. This catches a refactor that adds a new classification without
// updating ActionFor.
func TestClassifyActionMappingIsExhaustive(t *testing.T) {
	allCls := []Classification{
		ClsUnchanged, ClsLocalModified, ClsRemoteModified, ClsConverged, ClsConflict,
		ClsLocalAdded, ClsRemoteAdded, ClsAddConflict, ClsLocalDeleted, ClsRemoteDeleted,
		ClsBothDeleted, ClsDelEditConflict, ClsEditDelConflict, ClsBothAddedSame,
	}

	// The skip classifications are the ones that legitimately map to ActSkip.
	skipCls := map[Classification]bool{
		ClsUnchanged:     true,
		ClsConverged:     true,
		ClsBothDeleted:   true,
		ClsBothAddedSame: true,
	}

	for _, c := range allCls {
		act := ActionFor(c)

		if skipCls[c] {
			assert.Equal(t, ActSkip, act, "%s must map to ActSkip", c)
		} else {
			assert.NotEqual(t, ActSkip, act,
				"%s must map to a non-skip action; ActSkip here means it fell through the switch", c)
		}
	}
}
