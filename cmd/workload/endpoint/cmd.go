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

package endpoint

import (
	"fmt"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/telemetry"
	"github.com/datarobot/cli/internal/workload"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "endpoint <workload-id>",
		Short: "Print a workload's endpoint URL.",
		Long: `Print a workload's endpoint URL, and nothing else.

The bare URL on stdout is the whole contract, so the command composes
directly in scripts. The URL ends with a trailing slash, so append
sub-paths without a leading slash of their own:

  curl "$(dr workload endpoint 68b0c1d2e3f4a5b6c7d8e9f0)health"

The endpoint is a stable gateway URL assigned at creation; it serves
traffic once the workload is running. The command fails when the
workload has no endpoint URL. For the full document (including the
endpoint alongside status and metadata) use 'dr workload get'.

Example:
  dr workload endpoint 68b0c1d2e3f4a5b6c7d8e9f0`,
		Args:         cobra.ExactArgs(1),
		PreRunE:      auth.EnsureAuthenticatedE,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, args []string) error {
			wl, err := workload.GetWorkload(args[0])
			if err != nil {
				return err
			}

			// Fail loudly rather than print an empty line: a script doing
			// curl "$(dr workload endpoint ...)" must not curl "".
			if wl.Endpoint == "" {
				return fmt.Errorf("workload %s has no endpoint URL", args[0])
			}

			fmt.Println(wl.Endpoint)

			return nil
		},
	}

	telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
		return map[string]any{
			"workload_id": telemetry.FirstArg(args),
		}
	})

	return cmd
}
