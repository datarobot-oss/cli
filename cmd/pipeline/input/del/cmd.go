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

// Package del implements the `dr pipeline input delete` verb. The
// directory is named `del` rather than `delete` because the latter
// shadows Go's built-in delete() function in importing files.

package del

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/datarobot/cli/cmd/pipeline/scopeflag"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	var flags scopeflag.Flags

	cmd := &cobra.Command{
		Use:   "delete <input-id>",
		Short: "Delete a pipeline input set",
		Long: `Delete an input payload from a pipeline.

Example:
  dr pipeline input delete --pipeline <id> <input-id>
  dr pipeline input delete --pipeline <id> --version=2 <input-id>`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			scope, version, err := flags.Resolve(cmd)
			if err != nil {
				return err
			}

			err = pipeline.DeleteInput(flags.PipelineID, scope, version, args[0])
			if err != nil {
				return handleDeleteError(err, args[0])
			}

			fmt.Println(tui.BaseTextStyle.Render("Deleted input: " + args[0]))

			return nil
		},
	}

	flags.Bind(cmd)
	_ = cmd.MarkFlagRequired("pipeline")

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"pipeline_id": flags.PipelineID,
			"input_id":    telemetry.FirstArg(args),
			"scope":       flags.Scope,
			"version":     flags.Version,
		}
	})

	return cmd
}

// handleDeleteError converts a 404 into a friendly informational message
// (returns nil) so the user does not see a stack-trace-style HTTP error
// for what is effectively a no-op.
func handleDeleteError(err error, id string) error {
	var httpErr *drapi.HTTPError

	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		fmt.Println(tui.DimStyle.Render("No input found with id: " + id))

		return nil
	}

	return err
}
