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

	"github.com/datarobot/cli/internal/config"
	"github.com/spf13/cobra"
)

func SetURLAction() {
	reader := bufio.NewReader(os.Stdin)

	datarobotHost := config.GetBaseURL()

	if len(datarobotHost) > 0 {
		fmt.Printf("A DataRobot URL of %s is already present, do you want to overwrite? (y/N): ", datarobotHost)

		selectedOption, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		if strings.ToLower(strings.TrimSpace(selectedOption)) != "y" {
			fmt.Println("Exiting without overwriting the DataRobot URL.")
			return
		}
	}

	fmt.Println("Please specify your DataRobot URL, or enter the numbers 1 - 3 If you are using that multi tenant cloud offering")
	fmt.Println("Please enter 1 if you're using https://app.datarobot.com")
	fmt.Println("Please enter 2 if you're using https://app.eu.datarobot.com")
	fmt.Println("Please enter 3 if you're using https://app.jp.datarobot.com")
	fmt.Println("Otherwise, please enter the URL you use")

	url, err := reader.ReadString('\n')
	if err != nil {
		return
	}

	err = config.SaveURLToConfig(url)
	if err != nil {
		return
	}
}

var setURLCmd = &cobra.Command{
	Use:   "setURL",
	Short: "Set URL for Login to DataRobot",
	Long:  `Set URL for DataRobot to get and store that URL which can be used for other operations in the cli.`,
	Run: func(_ *cobra.Command, _ []string) {
		SetURLAction()
	},
}
