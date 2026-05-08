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

//go:build !windows

package sync

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// realAvailableBytes uses Bavail (not Bfree) because Bavail excludes
// blocks reserved for the superuser.
func realAvailableBytes(path string) (int64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, fmt.Errorf("statfs %s: %w", path, err)
	}

	//nolint:gosec // Bavail * Bsize fits in int64 on every supported FS.
	return int64(st.Bavail * uint64(st.Bsize)), nil
}
