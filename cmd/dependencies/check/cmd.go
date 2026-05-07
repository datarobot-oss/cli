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

package check

import (
	"errors"
	"fmt"

	"github.com/datarobot/cli/internal/tools"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "✅ Check template dependencies",
		RunE:  RunE,
	}

	return cmd
}

func RunE(cmd *cobra.Command, _ []string) error {
	_, _, missingMsgs, wrongVersionMsgs := tools.CheckPrerequisites()
	if len(missingMsgs) > 0 || len(wrongVersionMsgs) > 0 {
		return errors.New(tools.PrerequisitesMsg(missingMsgs, wrongVersionMsgs))
	}

	fmt.Fprintln(cmd.OutOrStdout(), "✅ All dependencies are already up to date.")

	return nil
}
