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
	"context"

	core "github.com/datarobot/cli/internal/doctor"
	"github.com/datarobot/cli/internal/workload/wapi"
)

// divergenceCheck verifies that the config's lastSyncedVersionId and the
// manifest's syncedVersionId agree, including nil-ness (both null is a
// healthy "never synced" state).
type divergenceCheck struct {
	projectDir string
}

func (c *divergenceCheck) ID() string {
	return CheckIDDivergence
}

func (c *divergenceCheck) Name() string {
	return "Config/manifest sync pointers"
}

// Run compares the two sync pointers with empty-string normalization. It
// SKIPs when the project is unlinked or when either side cannot be loaded —
// a corrupt file is that file's check's FAIL, not a divergence.
func (c *divergenceCheck) Run(_ context.Context) core.Result {
	if res, skip := skipIfUnlinked(c.projectDir); skip {
		return res
	}

	cfg, err := wapi.LoadConfig(c.projectDir)
	if err != nil {
		return core.Result{
			Status:  core.StatusSKIP,
			Summary: "config.json is missing or corrupt; sync pointers cannot be compared",
		}
	}

	manifest, err := wapi.LoadManifest(c.projectDir)
	if err != nil {
		return core.Result{
			Status:  core.StatusSKIP,
			Summary: "manifest.json is missing or corrupt; sync pointers cannot be compared",
		}
	}

	cfgPtr := normalizeStringPtr(cfg.LastSyncedVersionID)

	manifestPtr := normalizeStringPtr(manifest.SyncedVersionID)

	if pointersAgree(cfgPtr, manifestPtr) {
		return core.Result{
			Status:  core.StatusOK,
			Summary: "config and manifest sync pointers agree",
		}
	}

	return core.Result{
		Status: core.StatusFAIL,
		Summary: "config and manifest sync pointers diverge (config lastSyncedVersionId: " +
			ptrDisplay(cfg.LastSyncedVersionID) +
			", manifest syncedVersionId: " +
			ptrDisplay(manifest.SyncedVersionID) + ")",
		Remedy: RemedyDivergence,
		Details: map[string]string{
			"configLastSyncedVersionId": ptrDisplay(cfg.LastSyncedVersionID),
			"manifestSyncedVersionId":   ptrDisplay(manifest.SyncedVersionID),
		},
		Fixable: true,
	}
}

// pointersAgree reports whether the two normalized pointers describe the
// same sync state: both absent, or both present and equal.
func pointersAgree(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}

	return *a == *b
}
