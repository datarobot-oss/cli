// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// EnsureAuthenticatedE checks if valid authentication exists, and if not,
// triggers the login flow automatically. Returns an error if authentication
// fails, suitable for use in Cobra PreRunE hooks.
func EnsureAuthenticatedE(ctx context.Context) error {
	if !EnsureAuthenticated(ctx) {
		return errors.New("authentication failed")
	}

	return nil
}

// EnsureAuthenticated checks if valid authentication exists, and if not,
// triggers the login flow automatically. This is a non-interactive version
// intended for use in automated workflows. Returns true if authentication
// is valid or was successfully obtained.
func EnsureAuthenticated(ctx context.Context) bool {
	if viper.GetBool("skip_auth") {
		log.Warn("Authentication checks are disabled via --skip-auth flag. This may cause API calls to fail.")

		return true
	}

	datarobotHost := config.GetBaseURL()
	if datarobotHost == "" {
		log.Warn("No DataRobot URL configured. Running auth setup...")
		SetURLAction()

		datarobotHost = config.GetBaseURL()
		if datarobotHost == "" {
			log.Error("Failed to configure DataRobot URL")
			return false
		}
	}

	if token := config.GetAPIKey(); token != "" {
		// Valid token exists
		return true
	}

	// No valid token, attempt to get one
	log.Warn("No valid API key found. Starting authentication flow...")

	// Auto-retrieve new credentials without prompting
	viper.Set(config.DataRobotAPIKey, "")

	key, err := apiKeyCallbackFunc(ctx, datarobotHost)
	if err != nil {
		log.Error("Failed to retrieve API key", "error", err)
		return false
	}

	viper.Set(config.DataRobotAPIKey, strings.ReplaceAll(key, "\n", ""))
	WriteConfigFileSilent()

	log.Info("Authentication successful")

	return true
}

func LoginAction(ctx context.Context) error {
	// short-circuit if skip_auth is enabled. This allows users to avoid login prompts
	// when authentication is intentionally disabled, say if the user is offline, or in
	// a CI/CD environment, or in a script.
	if viper.GetBool("skip_auth") {
		return errors.New("login has been disabled via --skip-auth flag")
	}

	datarobotHost := config.GetBaseURL()
	if datarobotHost == "" {
		SetURLAction()

		datarobotHost = config.GetBaseURL()
	}

	// If they explicitly ran 'dr auth login', just authenticate them
	if token := config.GetAPIKey(); token != "" {
		log.Info("Re-authenticating with DataRobot...")
	} else {
		log.Warn("No valid API key found. Retrieving a new one")
	}

	log.Info("💡 To change your DataRobot URL, run 'dr auth set-url'")

	// Clear existing token and get new one
	viper.Set(config.DataRobotAPIKey, "")

	key, err := apiKeyCallbackFunc(ctx, datarobotHost)
	if err != nil {
		return err
	}

	if key == "" {
		return nil
	}

	viper.Set(config.DataRobotAPIKey, strings.ReplaceAll(key, "\n", ""))

	writeConfigFile()

	return nil
}

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "🔐 Login to DataRobot using OAuth authentication.",
	Long: `Login to DataRobot using OAuth authentication in your browser.

This command will:
  1. Open your default browser.
  2. Redirect you to the DataRobot login page.
  3. Securely store your API key for future CLI operations.`,
	Run: func(cmd *cobra.Command, _ []string) {
		err := LoginAction(cmd.Context())
		if err != nil {
			log.Error(err)
		}
	},
}
