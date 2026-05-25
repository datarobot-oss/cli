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

// Package del implements `dr pipeline environment delete`. Directory
// is named `del` rather than `delete` to avoid shadowing Go's built-in
// `delete()` in importing files.

package del

import (
	"fmt"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/pipeline"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <environment-id>",
		Short: "Delete a pipeline execution environment",
		Long: `Soft-delete the most recent active version of a pipeline
execution environment. If no active versions remain after the delete,
the parent environment is soft-deleted too.

To delete a specific older version, use:
  dr pipeline environment version delete --environment <id> <version>

Example:
  dr pipeline environment delete env-123`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, args []string) error {
			err := pipeline.DeleteEnvironment(args[0])
			if err != nil {
				return err
			}

			fmt.Println(tui.BaseTextStyle.Render("Deleted environment: " + args[0]))

			return nil
		},
	}

	return cmd
}
