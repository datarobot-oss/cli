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

package login

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func RunE(cmd *cobra.Command, args []string) error { //nolint: cyclop
	// short-circuit if skip_auth is enabled. This allows users to avoid login prompts
	// when authentication is intentionally disabled, say if the user is offline, or in
	// a CI/CD environment, or in a script.
	if viper.GetBool("skip_auth") {
		err := errors.New("Login has been disabled via the '--skip-auth' flag.")
		log.Error(err)

		return err
	}

	var url string
	if len(args) > 0 {
		url = args[0]
	}

	if url != "" {
		err := config.SaveURLToConfig(url)
		if err != nil {
			log.Error(err.Error())
		}
	}

	datarobotHost := auth.GetBaseURLOrAsk()
	if datarobotHost == "" {
		log.Info("💡 To set your DataRobot URL, run 'dr auth set-url'.")
		os.Exit(1)

		return nil
	}

	token, err := config.GetAPIKey()
	if errors.Is(err, context.DeadlineExceeded) {
		log.Errorf("Connection to %s timed out. Check your network and try again.", datarobotHost)
		os.Exit(1)

		return nil
	}

	// If they explicitly ran 'dr auth login', just authenticate them
	if token != "" {
		log.Info("Re-authenticating with DataRobot...")
	} else {
		log.Warn("No valid API key found. Retrieving a new one...")
	}

	log.Info("💡 To change your DataRobot URL, run 'dr auth set-url'.")

	// Clear existing token and get new one
	viper.Set(config.DataRobotAPIKey, "")

	key, err := auth.WaitForAPIKeyCallback(cmd.Context(), datarobotHost)
	if err != nil {
		log.Error(err)

		cmd.SilenceUsage = true

		return err
	}

	if key == "" {
		return nil
	}

	viper.Set(config.DataRobotAPIKey, strings.ReplaceAll(key, "\n", ""))

	err = auth.WriteConfigFile()
	if err != nil {
		log.Error(err)

		cmd.SilenceUsage = true

		return err
	}

	return nil
}

func Cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login [url]",
		Short: "🔐 Log in to DataRobot using OAuth authentication.",
		Long: `Log in to DataRobot using OAuth authentication in your browser.

This command will:
  1. Open your default browser.
  2. Redirect you to the DataRobot login page.
  3. Securely store your API key for future CLI operations.`,
		RunE: RunE,
	}
}
