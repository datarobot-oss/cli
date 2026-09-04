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
	"github.com/datarobot/cli/internal/fsutil"
	"github.com/datarobot/cli/internal/workload/wapi"
)

// rollbackCheck reports whether an interrupted sync left a stale rollback
// tree behind. The tree's existence — even empty — is the evidence: a sync
// crashed between staging its backups and clearing them.
type rollbackCheck struct {
	projectDir string
}

func (c *rollbackCheck) ID() string {
	return CheckIDRollback
}

func (c *rollbackCheck) Name() string {
	return "Interrupted rollback"
}

// Run looks for a .rollback/ tree at every location wapi.StaleRollbackDirs
// knows about (current and legacy), without touching any of its contents.
func (c *rollbackCheck) Run(_ context.Context) core.Result {
	if res, skip := skipIfUnlinked(c.projectDir); skip {
		return res
	}

	for _, dir := range wapi.StaleRollbackDirs(c.projectDir) {
		if fsutil.DirExists(dir) {
			return core.Result{
				Status:  core.StatusFAIL,
				Summary: "interrupted rollback present at " + absPath(dir),
				Remedy:  RemedyRollback,
				Details: map[string]string{"path": absPath(dir)},
				Fixable: true,
			}
		}
	}

	return core.Result{
		Status:  core.StatusOK,
		Summary: "no interrupted rollback",
	}
}
