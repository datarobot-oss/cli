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

package logout

import (
	"os"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func Run(_ *cobra.Command, _ []string) {
	viper.Set(config.DataRobotAPIKey, "")

	err := auth.WriteConfigFile()
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}
}

func Cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Log out from DataRobot.",
		Long:  `Log out from DataRobot and clear the stored API key.`,
		Run:   Run,
	}
}
