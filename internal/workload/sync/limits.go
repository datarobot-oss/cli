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

// Sync-engine orchestration tunables. File-level limits live in
// internal/workload/fileops.
const (
	UploadConcurrency   = 4
	DownloadConcurrency = 6

	// UploadRetries is the per-file retry budget for transport failures
	// (network reset, 5xx). Permanent 4xx errors are not retried.
	UploadRetries = 3

	UploadTimeoutSecs   = 300
	DownloadTimeoutSecs = 300

	// DiskSpaceMarginMB is the headroom required on top of the download
	// size before Phase 5 begins, to prevent disk-full mid-sync.
	DiskSpaceMarginMB = 100

	// RollbackMaxFiles caps the rollback set so a single sync cannot
	// produce a multi-gigabyte .wapi/.rollback/ tree.
	RollbackMaxFiles = 1000

	// Stage path is used when files <= threshold AND bytes <= threshold;
	// zip path otherwise.
	StageVsZipFileThreshold  = 20
	StageVsZipBytesThreshold = 50 * 1024 * 1024

	ZipPollIntervalMS  = 500
	ZipPollTimeoutSecs = 600
)
