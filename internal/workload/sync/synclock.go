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
	"fmt"
	"os"
	"path/filepath"
)

const syncLockFile = "sync.lock"

// SyncLock is the platform-specific exclusive lock for a sync run.
type SyncLock struct {
	path string
	f    *os.File
}

// AcquireSyncLock opens .wapi/sync.lock and acquires an exclusive lock
// without waiting. Cross-platform polish is tracked in RAPTOR-16928.
func AcquireSyncLock(projectDir string) (*SyncLock, error) {
	wapiDir := filepath.Join(projectDir, ".wapi")
	if _, err := os.Stat(wapiDir); err != nil {
		return nil, fmt.Errorf("acquire sync lock: %w", err)
	}

	path := filepath.Join(wapiDir, syncLockFile)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open sync lock %s: %w", path, err)
	}

	if err := tryLockExclusive(f); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("another sync is already running on this project: %w", err)
	}

	return &SyncLock{path: path, f: f}, nil
}

// Release unlocks and closes the lock file. It does NOT remove the file
// because that would race with another process that has the file open
// but has not yet acquired the lock.
func (l *SyncLock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}

	_ = unlockExclusive(l.f)

	if err := l.f.Close(); err != nil {
		return fmt.Errorf("close sync lock: %w", err)
	}

	return nil
}
