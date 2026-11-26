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

package platformdoctor

import (
	"context"

	core "github.com/datarobot/cli/internal/doctor"
)

const idInstallRead = "platform.install.read"

// installRead says whether the install answered. A failure makes the other
// checks skip.
type installRead struct {
	snapshot *snapshot
}

func (c *installRead) ID() string { return idInstallRead }

func (c *installRead) Name() string { return "Install report" }

func (c *installRead) Run(ctx context.Context) core.Result {
	report, err := c.snapshot.get(ctx)
	if err != nil {
		return core.Result{
			Status:  core.StatusFAIL,
			Summary: "The install could not be read: " + err.Error(),
			Remedy:  remedyCheckCredentials,
		}
	}

	summary := "The install answered."
	if report.Server.Release != "" {
		summary = "The install answered, release " + report.Server.Release + "."
	}

	return core.Result{Status: core.StatusOK, Summary: summary}
}
