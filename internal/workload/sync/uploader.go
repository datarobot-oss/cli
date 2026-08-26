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

// UploadOutcome is what an Uploader actually accomplished. Sent carries the
// hash and size of the bytes that really crossed the wire, keyed by the same
// forward-slash relative path used everywhere else in the manifest. Phase 6
// seeds each uploaded file's manifest entry from Sent, never from the Phase-2
// planned hash — a per-path fallback to the planned hash is the original
// poisoning bug and must not exist anywhere in the code.
type UploadOutcome struct {
	CatalogID string
	VersionID string
	Sent      map[string]FileEntry
}

// Uploader pushes a SyncPlan's Uploads and returns the resulting outcome.
// When the artifact has no catalog (first-sync against an empty artifact)
// the implementation creates a new one.
type Uploader interface {
	ApplyUploads(e *Engine, files []FileAction) (UploadOutcome, error)
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
