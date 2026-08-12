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

package clone

import (
	"errors"
	"fmt"
	"strings"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/outputformat"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var (
		outputFormat outputformat.OutputFormat
		name         string
	)

	cmd := &cobra.Command{
		Use:   "clone <pipeline-id>",
		Short: "Clone a pipeline into a new draft",
		Long: `Clone an existing pipeline into a brand-new draft, copying over its source
code, description, latest input params, and latest assigned image.

The clone is named "Clone of <source name>" by default. Supply --name to use a
custom label instead. If the name is already taken by another draft, a numeric
suffix is appended automatically so repeated clones never fail.

Example:
  dr pipeline clone 507f1f77bcf86cd799439011
  dr pipeline clone 507f1f77bcf86cd799439011 --name "My Clone"
  dr pipeline clone 507f1f77bcf86cd799439011 --output-format json`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			outputFormat = outputformat.GetFormat(cmd)

			name = strings.TrimSpace(name)
			if cmd.Flags().Changed("name") && name == "" {
				return errors.New("--name must not be blank")
			}

			result, err := pipeline.ClonePipeline(args[0], name)
			if err != nil {
				return fmt.Errorf("clone pipeline: %w", err)
			}

			return pipeline.RenderCreateResponse(outputFormat, *result)
		},
	}

	outputformat.AddFlag(cmd, &outputFormat)
	cmd.Flags().StringVar(&name, "name", "", "Optional display name for the clone; defaults to \"Clone of <source name>\"")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"pipeline_id":   telemetry.FirstArg(args),
			"output_format": string(outputFormat),
		}
	})

	return cmd
}
