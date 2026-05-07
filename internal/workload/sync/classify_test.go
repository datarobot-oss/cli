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

func TestClassify(t *testing.T) {
	const (
		x = "X"
		y = "Y"
		z = "Z"
	)

	cases := []struct {
		name           string
		base, lh, rh   string
		want           Classification
		expectedAction Action
	}{
		// BASE present (file existed at last sync)
		{name: "Unchanged_X_X_X", base: x, lh: x, rh: x, want: ClsUnchanged, expectedAction: ActSkip},
		{name: "LocalModified_X_Y_X", base: x, lh: y, rh: x, want: ClsLocalModified, expectedAction: ActUploadModify},
		{name: "RemoteModified_X_X_Y", base: x, lh: x, rh: y, want: ClsRemoteModified, expectedAction: ActDownloadModify},
		{name: "Converged_X_Y_Y", base: x, lh: y, rh: y, want: ClsConverged, expectedAction: ActSkip},
		{name: "Conflict_X_Y_Z", base: x, lh: y, rh: z, want: ClsConflict, expectedAction: ActConflictCopy},
		{name: "LocalDeleted_X_empty_X", base: x, lh: "", rh: x, want: ClsLocalDeleted, expectedAction: ActUploadDelete},
		{name: "RemoteDeleted_X_X_empty", base: x, lh: x, rh: "", want: ClsRemoteDeleted, expectedAction: ActDownloadDelete},
		{name: "BothDeleted_X_empty_empty", base: x, lh: "", rh: "", want: ClsBothDeleted, expectedAction: ActSkip},
		{name: "DelEditConflict_X_Y_empty", base: x, lh: y, rh: "", want: ClsDelEditConflict, expectedAction: ActConflictCopy},
		{name: "EditDelConflict_X_empty_Y", base: x, lh: "", rh: y, want: ClsEditDelConflict, expectedAction: ActDownloadOverDel},

		// BASE absent (new file on at least one side)
		{name: "LocalAdded_empty_Y_empty", base: "", lh: y, rh: "", want: ClsLocalAdded, expectedAction: ActUploadAdd},
		{name: "RemoteAdded_empty_empty_Y", base: "", lh: "", rh: y, want: ClsRemoteAdded, expectedAction: ActDownloadAdd},
		{name: "AddConflict_empty_Y_Z", base: "", lh: y, rh: z, want: ClsAddConflict, expectedAction: ActConflictCopy},
		{name: "BothAddedSame_empty_Y_Y", base: "", lh: y, rh: y, want: ClsBothAddedSame, expectedAction: ActSkip},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(tc.base, tc.lh, tc.rh)
			assert.Equal(t, tc.want, got, "classification: %s", got)
			assert.Equal(t, tc.expectedAction, ActionFor(got))
		})
	}
}

func TestIsConflict(t *testing.T) {
	conflicts := []Classification{ClsConflict, ClsAddConflict, ClsDelEditConflict, ClsEditDelConflict}
	for _, c := range conflicts {
		assert.True(t, c.IsConflict(), "%s should be a conflict", c)
	}

	nonConflicts := []Classification{
		ClsUnchanged, ClsLocalModified, ClsRemoteModified, ClsConverged,
		ClsLocalAdded, ClsRemoteAdded, ClsLocalDeleted, ClsRemoteDeleted,
		ClsBothDeleted, ClsBothAddedSame,
	}

	for _, c := range nonConflicts {
		assert.False(t, c.IsConflict(), "%s should NOT be a conflict", c)
	}
}

func TestClassificationStringIsExhaustive(t *testing.T) {
	all := []Classification{
		ClsUnchanged, ClsLocalModified, ClsRemoteModified, ClsConverged, ClsConflict,
		ClsLocalAdded, ClsRemoteAdded, ClsAddConflict, ClsLocalDeleted, ClsRemoteDeleted,
		ClsBothDeleted, ClsDelEditConflict, ClsEditDelConflict, ClsBothAddedSame,
	}

	for _, c := range all {
		assert.NotEqual(t, "UNKNOWN", c.String(), "classification %d has no name", c)
	}
}
