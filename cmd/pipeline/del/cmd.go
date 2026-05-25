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

// Package del implements the `dr pipeline delete` verb. The directory
// is named `del` rather than `delete` because the latter shadows Go's
// built-in delete() function in importing files.

package del

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/drapi"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <pipeline-id>",
		Short: "Delete a pipeline",
		Long: `Delete a pipeline by id. The pipeline and all of its versions are
removed from the registry.

Example:
  dr pipeline delete 507f1f77bcf86cd799439011`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, args []string) error {
			err := pipeline.DeletePipeline(args[0])
			if err != nil {
				return handleDeleteError(err, args[0])
			}

			fmt.Println(tui.BaseTextStyle.Render("Deleted pipeline: " + args[0]))

			return nil
		},
	}

	return cmd
}

// handleDeleteError converts a 404 into a friendly informational message
// (returns nil) so the user does not see a stack-trace-style HTTP error
// for what is effectively a no-op.
func handleDeleteError(err error, pipelineID string) error {
	var httpErr *drapi.HTTPError

	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		fmt.Println(tui.DimStyle.Render("No pipeline found with id: " + pipelineID))

		return nil
	}

	return err
}
