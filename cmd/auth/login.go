// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package auth

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
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
	reader := bufio.NewReader(os.Stdin)

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

	if token := config.GetAPIKey(); token != "" {
		fmt.Println("🔑You're already logged in to DataRobot, do you want to login with a different account? (y/N): ")

		selectedOption, err := reader.ReadString('\n')
		if err != nil {
			return err
		}

		if strings.ToLower(strings.TrimSpace(selectedOption)) == "y" {
			// Set the DataRobot API key to be an empty string
			viper.Set(config.DataRobotAPIKey, "")
		} else {
			fmt.Println("✅ Keeping current login. You're all set!")
			return nil
		}
	} else {
		log.Warn("The stored API key is invalid or expired. Retrieving a new one")
	}

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
	Short: "🔐 Login to DataRobot",
	Long: `Login to DataRobot using OAuth authentication in your browser.

This command will:
  1. Open your default browser
  2. Redirect you to DataRobot login page  
  3. Securely store your API key for future CLI operations`,
	Run: func(cmd *cobra.Command, _ []string) {
		err := LoginAction(cmd.Context())
		if err != nil {
			log.Error(err)
		}
	},
}
