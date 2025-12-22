// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package login

import (
	"errors"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/auth"
	"github.com/datarobot/cli/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func RunE(cmd *cobra.Command, _ []string) error {
	// short-circuit if skip_auth is enabled. This allows users to avoid login prompts
	// when authentication is intentionally disabled, say if the user is offline, or in
	// a CI/CD environment, or in a script.
	if viper.GetBool("skip_auth") {
		err := errors.New("Login has been disabled via the '--skip-auth' flag.")
		log.Error(err)

		return err
	}

	datarobotHost := config.GetBaseURL()
	if datarobotHost == "" {
		checkHost := true
		auth.SetURLAction(checkHost)

		datarobotHost = config.GetBaseURL()
	}

	// If they explicitly ran 'dr auth login', just authenticate them
	if token := config.GetAPIKey(); token != "" {
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
		return err
	}

	if key == "" {
		return nil
	}

	viper.Set(config.DataRobotAPIKey, strings.ReplaceAll(key, "\n", ""))

	if err := auth.WriteConfigFile(); err != nil {
		log.Error(err)
		return err
	}

	return nil
}

func Cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "🔐 Log in to DataRobot using OAuth authentication.",
		Long: `Log in to DataRobot using OAuth authentication in your browser.

This command will:
  1. Open your default browser.
  2. Redirect you to the DataRobot login page.
  3. Securely store your API key for future CLI operations.`,
		RunE: RunE,
	}
}
