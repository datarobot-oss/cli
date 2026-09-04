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

package fileops

import "os"

// ApplyMode chmods path to the low 9 permission bits of mode. mode == 0
// means "no mode information available" (a manifest/response predating mode
// tracking, or a server that hasn't shipped a mode field yet) and is a
// deliberate no-op: chmod-ing to literal 0 would strip all permissions,
// which is strictly worse than leaving the file at its just-written
// default mode.
//
// On Windows this is a safe no-op beyond the read-only attribute: NTFS has
// no owner/group/execute concept, so os.Chmod there only honors the 0200
// (owner-write) bit -- the executable bit this feature exists for is
// silently ignored, never errored, consistent with how Windows determines
// executability by extension/interpreter rather than a permission bit.
func ApplyMode(path string, mode uint32) error {
	perm := os.FileMode(mode).Perm()
	if perm == 0 {
		return nil
	}

	return os.Chmod(path, perm)
}
