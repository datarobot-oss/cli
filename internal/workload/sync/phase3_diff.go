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

import "context"

// phase3Diff turns the three manifests into a SyncPlan.
func phase3Diff(_ context.Context, e *Engine) error {
	plan := Diff(e.base, e.local, e.remote)
	plan.OldVersionShort = ShortVer(ptrOrEmpty(e.config.LastSyncedVersionID))
	e.plan = plan

	return nil
}

// ShortVer truncates a hex version ID to 8 chars for display. Inputs
// shorter than 8 chars are returned unchanged.
func ShortVer(s string) string {
	const shortLen = 8

	if len(s) > shortLen {
		return s[:shortLen]
	}

	return s
}
