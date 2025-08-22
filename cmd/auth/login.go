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
	"github.com/datarobot/cli/internal/base_auth"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func LoginAction() error {
	reader := bufio.NewReader(os.Stdin)

	datarobotHost, err := GetURL(false)
	if err != nil {
		return err
	}

	currentKey := viper.GetString(base_auth.DataRobotAPIKey)

	isValidKeyPair, err := verifyAPIKey(datarobotHost, currentKey)
	if err != nil {
		return err
	}

	if isValidKeyPair {
		fmt.Println("An API key is already present, do you want to overwrite? (y/N): ")

		selectedOption, err := reader.ReadString('\n')
		if err != nil {
			return err
		}

		if strings.ToLower(strings.Replace(selectedOption, "\n", "", -1)) == "y" {
			// Set the DataRobot API key to be an empty string
			viper.Set(base_auth.DataRobotAPIKey, "")
		} else {
			fmt.Println("Exiting without overwriting the API key.")

			writeConfigFile()

			return nil
		}
	} else {
		log.Warn("The stored API key is invalid or expired. Retrieving a new one")
	}

	key, err := waitForAPIKeyCallback(datarobotHost)
	if err != nil {
		log.Error(err)
	}

	viper.Set(base_auth.DataRobotAPIKey, strings.Replace(key, "\n", "", -1))

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
