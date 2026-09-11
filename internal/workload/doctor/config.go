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

// configCheck verifies that config.json parses and passes wapi's semantic
// validation (including the coupled catalogId/lastSyncedVersionId rule).
type configCheck struct {
	projectDir string
}

func (c *configCheck) ID() string {
	return CheckIDConfig
}

func (c *configCheck) Name() string {
	return "Config file"
}

// Run loads the config read-only. A missing file and a parse/validation
// failure are both FAILs carrying the absolute file path in details.path;
// an unlinked project SKIPs (presence cascade).
func (c *configCheck) Run(_ context.Context) core.Result {
	if res, skip := skipIfUnlinked(c.projectDir); skip {
		return res
	}

	_, err := wapi.LoadConfig(c.projectDir)

	switch {
	case err == nil:
		return core.Result{
			Status:  core.StatusOK,
			Summary: "config.json is valid",
		}
	case errors.Is(err, wapi.ErrNotInitialized):
		// The state directory exists but config.json does not.
		return corruptFileResult("config.json is missing", wapi.ConfigPath(c.projectDir), RemedyConfig, false)
	default:
		return corruptFileResult(
			"config.json is corrupt: "+corruptReason(err),
			stateErrPath(err, wapi.ConfigPath(c.projectDir)),
			RemedyConfig,
			false,
		)
	}
}
