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

package logout

import (
	"fmt"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/cli"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/log"
	"github.com/spf13/cobra"
)

func RunE(_ *cobra.Command, _ []string) error {
	viperx.Set(config.DataRobotAPIKey, "")

	err := auth.WriteConfigFile()
	if err != nil {
		log.Error(fmt.Errorf("failed to write config: %w", err))

		return cli.ErrSilent
	}

	return nil
}

func Cmd() *cobra.Command {
	return &cobra.Command{
		Use:           "logout",
		Short:         "🚪 Log out from DataRobot",
		SilenceErrors: true,
		SilenceUsage:  true,
		Long:          `Log out from DataRobot and clear the stored API key.`,
		RunE:          RunE,
	}
}
