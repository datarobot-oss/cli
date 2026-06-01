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

package version

import (
	"github.com/datarobot/cli/cmd/pipeline/version/get"
	"github.com/datarobot/cli/cmd/pipeline/version/list"
	"github.com/spf13/cobra"
)

// Cmd returns the parent command for `dr pipeline version`.
func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Inspect pipeline versions",
		Long: `Read-only access to pipeline versions.

Versions are also surfaced inline by ` + "`dr pipeline get`" + `; this group
provides a paginated list and a single-version detail view.`,
	}

	cmd.AddCommand(
		list.Cmd(),
		get.Cmd(),
	)

	return cmd
}
