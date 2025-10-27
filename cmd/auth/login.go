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
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// EnsureAuthenticated checks if valid authentication exists, and if not,
// triggers the login flow automatically. This is a non-interactive version
// intended for use in automated workflows. Returns true if authentication
// is valid or was successfully obtained.
func EnsureAuthenticated() bool {
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

	key, err := apiKeyCallbackFunc(datarobotHost)
	if err != nil {
		log.Error("Failed to retrieve API key", "error", err)
		return false
	}

	viper.Set(config.DataRobotAPIKey, strings.ReplaceAll(key, "\n", ""))
	WriteConfigFileSilent()

	log.Info("Authentication successful")

	return true
}

func LoginAction() error {
	reader := bufio.NewReader(os.Stdin)

	datarobotHost := config.GetBaseURL()
	if datarobotHost == "" {
		SetURLAction()

		datarobotHost = config.GetBaseURL()
	}

	if token := config.GetAPIKey(); token != "" {
		fmt.Println("An API key is already present, do you want to overwrite? (y/N): ")

		selectedOption, err := reader.ReadString('\n')
		if err != nil {
			return err
		}

		if strings.ToLower(strings.TrimSpace(selectedOption)) == "y" {
			// Set the DataRobot API key to be an empty string
			viper.Set(config.DataRobotAPIKey, "")
		} else {
			fmt.Println("Exiting without overwriting the API key.")
			return nil
		}
	} else {
		log.Warn("The stored API key is invalid or expired. Retrieving a new one")
	}

	key, err := apiKeyCallbackFunc(datarobotHost)
	if err != nil {
		log.Error(err)
	}

	viper.Set(config.DataRobotAPIKey, strings.ReplaceAll(key, "\n", ""))

	writeConfigFile()

	return nil
}

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login to DataRobot",
	Long:  `Login to DataRobot to get and store an API key that can be used for other operation in the cli.`,
	Run: func(_ *cobra.Command, _ []string) {
		err := LoginAction()
		if err != nil {
			log.Error(err)
		}
	},
}
