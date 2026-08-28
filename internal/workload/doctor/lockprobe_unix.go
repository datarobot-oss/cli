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

package doctor

import (
	"os"

	"golang.org/x/sys/unix"
)

// tryLockSyncLockExclusive attempts a non-blocking exclusive advisory lock
// on f. It fails with EWOULDBLOCK when another live process holds the lock.
func tryLockSyncLockExclusive(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) //nolint:gosec // uintptr and int are same size on supported platforms
}

// unlockSyncLock releases the advisory lock held on f.
func unlockSyncLock(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_UN) //nolint:gosec // uintptr and int are same size on supported platforms
}
