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

package testutil

import "testing"

// SkipIfRoot is a no-op on Windows. os.Geteuid does not exist there, so the
// real guard cannot compile; every call site reaches this helper only after
// SkipIfWindows has already skipped, because a chmod fault is a POSIX-only
// mechanism. A no-op therefore loses no coverage, and keeping the signature
// lets call sites use both helpers unconditionally without their own
// platform branching.
func SkipIfRoot(t *testing.T) {
	t.Helper()
}
