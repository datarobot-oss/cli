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

import "sort"

// FileAction is one row of the SyncPlan.
type FileAction struct {
	Path           string
	Classification Classification
	Action         Action
	LocalSize      int64
	RemoteSize     int64
	LocalHash      string
	RemoteHash     string
}

// SyncPlan is the blueprint Phase 5 executes and the structure the display
// layer renders.
type SyncPlan struct {
	Uploads   []FileAction // LOCAL_MODIFIED + LOCAL_ADDED
	Downloads []FileAction // REMOTE_MODIFIED + REMOTE_ADDED + EDIT_DEL_CONFLICT
	Deletes   []FileAction // LOCAL_DELETED + REMOTE_DELETED
	Conflicts []FileAction // CONFLICT + ADD_CONFLICT + DEL_EDIT_CONFLICT

	// OldVersionShort is the 8-char prefix of the BASE manifest's
	// syncedVersionId; empty before the first successful sync.
	OldVersionShort string
}

// Append routes a FileAction into the right group based on its Action.
// Skip actions are dropped.
func (p *SyncPlan) Append(fa FileAction) {
	switch fa.Action {
	case ActSkip:
	case ActUploadModify, ActUploadAdd:
		p.Uploads = append(p.Uploads, fa)
	case ActDownloadModify, ActDownloadAdd, ActDownloadOverDel:
		p.Downloads = append(p.Downloads, fa)
	case ActUploadDelete, ActDownloadDelete:
		p.Deletes = append(p.Deletes, fa)
	case ActConflictCopy:
		// Conflicts also need the remote download (remote wins); the
		// executor issues that download alongside the conflict copy.
		p.Conflicts = append(p.Conflicts, fa)
	}
}

// Sort orders every group by path. Call once after all Append calls.
func (p *SyncPlan) Sort() {
	for _, group := range [][]FileAction{p.Uploads, p.Downloads, p.Deletes, p.Conflicts} {
		sort.Slice(group, func(i, j int) bool { return group[i].Path < group[j].Path })
	}
}

// HasConflicts reports whether any conflict-class rows exist.
func (p *SyncPlan) HasConflicts() bool {
	return len(p.Conflicts) > 0
}

// IsEmpty reports whether the plan has nothing to do.
func (p *SyncPlan) IsEmpty() bool {
	return len(p.Uploads) == 0 && len(p.Downloads) == 0 && len(p.Deletes) == 0 && len(p.Conflicts) == 0
}

// TotalUploadBytes sums the bytes the upload step will push.
func (p *SyncPlan) TotalUploadBytes() int64 {
	var n int64

	for _, fa := range p.Uploads {
		n += fa.LocalSize
	}

	return n
}

// TotalDownloadBytes sums download bytes plus remote bytes for conflict
// copies (since those also pull the remote).
func (p *SyncPlan) TotalDownloadBytes() int64 {
	var n int64

	for _, fa := range p.Downloads {
		n += fa.RemoteSize
	}

	for _, fa := range p.Conflicts {
		n += fa.RemoteSize
	}

	return n
}

// ConflictPaths returns the conflict paths sorted alphabetically.
func (p *SyncPlan) ConflictPaths() []string {
	out := make([]string, 0, len(p.Conflicts))
	for _, fa := range p.Conflicts {
		out = append(out, fa.Path)
	}

	sort.Strings(out)

	return out
}
