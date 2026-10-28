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

// Package profile provides read-only inspection of the named profiles
// stored in drconfig.yaml. Profiles are created by pointing --profile (or
// DATAROBOT_CLI_PROFILE) at a name that doesn't exist yet and running
// `dr auth login` or `dr auth set-url`; there is no `create` here.
package profile

import (
	"github.com/datarobot/cli/cmd/auth/profile/list"
	"github.com/datarobot/cli/cmd/auth/profile/show"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "profile",
		Aliases: []string{"profiles"},
		Short:   "📇 Inspect named DataRobot profiles",
		Long: `Inspect the named profiles stored in drconfig.yaml.

A profile is a named set of credentials (endpoint + token, optionally
ca-cert) alongside the default one, so you can work against
several DataRobot installations without re-authenticating each time.
Select one with --profile <name> or DATAROBOT_CLI_PROFILE=<name> on any
command.

There is no 'create' or 'delete' subcommand: a profile is created the first
time you run 'dr --profile <name> auth login' (or 'auth set-url') for a name
that doesn't exist yet, and removed by editing drconfig.yaml directly.`,
	}

	cmd.AddCommand(
		list.Cmd(),
		show.Cmd(),
	)

	return cmd
}
