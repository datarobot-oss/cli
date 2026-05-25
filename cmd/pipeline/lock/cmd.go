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

package lock

import (
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var outputFormat pipeline.OutputFormat

	cmd := &cobra.Command{
		Use:   "lock <pipeline-id>",
		Short: "Lock a draft pipeline",
		Long: `Promote a draft pipeline to locked mode. Once locked, the pipeline can
no longer be updated and locked runs/inputs/schedules become valid.

Example:
  dr pipeline lock 507f1f77bcf86cd799439011
  dr pipeline lock 507f1f77bcf86cd799439011 --output-format json`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, args []string) error {
			result, err := pipeline.LockPipeline(args[0])
			if err != nil {
				return err
			}

			return pipeline.RenderCreateResponse(outputFormat, *result)
		},
	}

	pipelines.AddOutputFlag(cmd, &outputFormat)

	return cmd
}
