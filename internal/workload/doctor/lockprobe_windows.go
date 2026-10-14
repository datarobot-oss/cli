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

//go:build windows

package doctor

import "os"

// tryLockSyncLockExclusive on Windows is a no-op: the lock check reports
// SKIP through the platform seam before ever reaching this path (real
// LockFileEx support is tracked in RAPTOR-16928). It exists so the package
// compiles on GOOS=windows.
func tryLockSyncLockExclusive(_ *os.File) error {
	return nil
}

// unlockSyncLock mirrors the unix release for the same compile-only reason.
func unlockSyncLock(_ *os.File) error {
	return nil
}
