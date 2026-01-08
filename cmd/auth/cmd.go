// Copyright 2025 DataRobot, Inc. and its affiliates.
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

package auth

import (
	"github.com/datarobot/cli/cmd/auth/check"
	"github.com/datarobot/cli/cmd/auth/login"
	"github.com/datarobot/cli/cmd/auth/logout"
	"github.com/datarobot/cli/cmd/auth/seturl"
	"github.com/datarobot/cli/internal/version"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "auth",
		GroupID: "core",
		Short:   "🔐 DataRobot authentication commands",
		Long: `Authentication commands for ` + version.AppName + `.

Manage your DataRobot credentials and connection settings:
  • Configure your DataRobot environment URL
  • Log in using OAuth authentication
  • Log out and clear stored credentials

🚀 Quick start: dr auth set-url && dr auth login`,
	}

	cmd.AddCommand(
		check.Cmd(),
		login.Cmd(),
		logout.Cmd(),
		seturl.Cmd(),
	)

	return cmd
}
