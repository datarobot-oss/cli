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

package cancel

import (
	"errors"
	"fmt"

	"github.com/datarobot/cli/cmd/pipeline/scopeflag"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var flags scopeflag.Flags

	cmd := &cobra.Command{
		Use:   "cancel <run-id>",
		Short: "Cancel a pipeline run",
		Long: `Request cancellation of an in-flight run.

The API rejects cancellation if the run has already reached a terminal
state (COMPLETED, FAILED, CANCELLED).

Example:
  dr pipeline run cancel --pipeline <id> <run-id>
  dr pipeline run cancel --pipeline <id> --version=2 <run-id>`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.PipelineID == "" {
				return errors.New("--pipeline is required")
			}

			scope, version, err := flags.Resolve(cmd)
			if err != nil {
				return err
			}

			err = pipeline.CancelRun(flags.PipelineID, scope, version, args[0])
			if err != nil {
				return err
			}

			fmt.Println(tui.BaseTextStyle.Render("Cancelled run: " + args[0]))

			return nil
		},
	}

	flags.Bind(cmd)

	return cmd
}
