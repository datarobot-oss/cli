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

// Package platform holds the commands that ask a DataRobot install what it
// supports.
package platform

import (
	"github.com/datarobot/cli/cmd/platform/describe"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "platform",
		GroupID: "core",
		Short:   "🧭 Ask a DataRobot install what it supports",
		Long: `Ask a DataRobot install what it supports.

Read-only commands that report what the install says about itself, so a
deploy pipeline can be written for that install before anything is uploaded.`,
	}

	cmd.AddCommand(describe.Cmd())

	return cmd
}
