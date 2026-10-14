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

// presenceCheck reports whether the project directory is linked to a remote
// artifact, i.e. a state directory exists at the current location or the
// legacy one.
type presenceCheck struct {
	projectDir string
}

func (c *presenceCheck) ID() string {
	return CheckIDPresence
}

func (c *presenceCheck) Name() string {
	return "State directory"
}

// Run stats the state directory through wapi.Exists, which resolves the
// current location first and falls back to legacy .wapi/. A regular file at
// the state path counts as not linked.
func (c *presenceCheck) Run(_ context.Context) core.Result {
	if wapi.Exists(c.projectDir) {
		return core.Result{
			Status:  core.StatusOK,
			Summary: "project is linked (state directory found)",
		}
	}

	return core.Result{
		Status:  core.StatusFAIL,
		Summary: "project is not linked (no state directory found)",
		Remedy:  RemedyPresence,
	}
}
