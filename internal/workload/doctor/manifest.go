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
	"errors"

	core "github.com/datarobot/cli/internal/doctor"
	"github.com/datarobot/cli/internal/workload/wapi"
)

// manifestCheck verifies that manifest.json (the BASE snapshot) parses and
// passes wapi's semantic validation (version, both-or-neither sync state,
// per-file metadata).
type manifestCheck struct {
	projectDir string
}

func (c *manifestCheck) ID() string {
	return CheckIDManifest
}

func (c *manifestCheck) Name() string {
	return "Manifest file"
}

// Run loads the manifest read-only. Unlike the divergence check it does not
// depend on config.json, so it still runs when the config is broken. A
// missing or corrupt manifest is a FAIL carrying the absolute path in
// details.path, and is repairable by `--fix` (empty-BASE rebuild).
func (c *manifestCheck) Run(_ context.Context) core.Result {
	if res, skip := skipIfUnlinked(c.projectDir); skip {
		return res
	}

	_, err := wapi.LoadManifest(c.projectDir)

	switch {
	case err == nil:
		return core.Result{
			Status:  core.StatusOK,
			Summary: "manifest.json is valid",
		}
	case errors.Is(err, wapi.ErrNotInitialized):
		return corruptFileResult("manifest.json is missing", wapi.ManifestPath(c.projectDir), RemedyManifest, true)
	default:
		return corruptFileResult(
			"manifest.json is corrupt: "+corruptReason(err),
			stateErrPath(err, wapi.ManifestPath(c.projectDir)),
			RemedyManifest,
			true,
		)
	}
}
