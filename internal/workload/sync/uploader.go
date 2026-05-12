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

// Uploader pushes a SyncPlan's Uploads and returns the resulting
// (catalogID, newVersionID). When catalogID is empty (first-sync against
// an empty artifact) the implementation creates a new catalog.
type Uploader interface {
	ApplyUploads(e *Engine, files []FileAction) (catalogID, versionID string, err error)
}

// ChooseUploader picks stage for small change sets (tight error semantics)
// or zip for larger ones (lower per-file overhead).
func ChooseUploader(plan *SyncPlan) Uploader {
	if plan == nil || len(plan.Uploads) == 0 {
		return &StageUploader{}
	}

	if len(plan.Uploads) <= StageVsZipFileThreshold && plan.TotalUploadBytes() <= int64(StageVsZipBytesThreshold) {
		return &StageUploader{}
	}

	return &ZipUploader{}
}
