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

package testutil

import (
	"runtime"
	"testing"
)

// SkipIfWindows skips the test when it runs on Windows. The reason must say
// what the test needs that Windows does not provide: the Windows CI leg is
// continue-on-error, so an unexplained skip is invisible in a CI summary and
// looks no different from a silent pass. A visible reason keeps the skipped
// run diagnosable rather than doubly invisible.
func SkipIfWindows(t *testing.T, reason string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip(reason)
	}
}
