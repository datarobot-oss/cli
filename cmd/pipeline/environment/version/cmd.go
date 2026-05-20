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
	"github.com/datarobot/cli/cmd/pipeline/environment/version/del"
	"github.com/spf13/cobra"
)

// Cmd returns the parent command for `dr pipelines environment version`.
// Currently only delete is exposed; the pipelines-api does not surface
// per-version GET endpoints.
func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Manage versions of a pipeline execution environment",
	}

	cmd.AddCommand(del.Cmd())

	return cmd
}
