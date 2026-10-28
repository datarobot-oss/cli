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

package doctor

import (
	"fmt"
	"runtime"
)

// lockSkipsOnHost reports whether the sync-lock check skips here: flock is
// not enforced on Windows, so a healthy project reports SKIP for it there.
func lockSkipsOnHost() bool {
	return runtime.GOOS == "windows"
}

// healthyStatus is what a healthy project reports for the named check on
// this host.
func healthyStatus(checkID string) string {
	if checkID == "local.lock" && lockSkipsOnHost() {
		return "SKIP"
	}

	return "OK"
}

// healthySummary is the summary of a healthy project on this host, with
// remoteSkipped of the remote checks skipped.
func healthySummary(remoteSkipped int) jsonSummary {
	sum := jsonSummary{OK: len(pinnedCheckOrder) - remoteSkipped, SKIP: remoteSkipped}

	if lockSkipsOnHost() {
		sum.OK--
		sum.SKIP++
	}

	return sum
}

// healthySummaryLine is the text summary of a healthy project on this host.
func healthySummaryLine() string {
	sum := healthySummary(0)

	return fmt.Sprintf("Summary: %d ok, 0 warn, 0 fail, %d skip — verdict: ok", sum.OK, sum.SKIP)
}
