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

	fmt.Println("🌐 DataRobot URL Configuration")
	fmt.Println("")
	fmt.Println("Choose your DataRobot environment:")
	fmt.Println("")
	fmt.Println("┌─────────────────────────────────────────────────────┐")
	fmt.Println("│  [1] 🇺🇸 US Cloud        https://app.datarobot.com      │")
	fmt.Println("│  [2] 🇪🇺 EU Cloud        https://app.eu.datarobot.com   │")
	fmt.Println("│  [3] 🇯🇵 Japan Cloud     https://app.jp.datarobot.com   │")
	fmt.Println("│  [4] 🏢 Custom   Enter your custom URL         │")
	fmt.Println("└─────────────────────────────────────────────────────┘")
	fmt.Println("")
	fmt.Println("🔗 Don't know which one? Check your DataRobot login page URL")
	fmt.Println("")
	fmt.Print("Enter your choice (1-4): ")

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
	Use:   "set-url",
	Short: "🌐 Configure your DataRobot environment URL",
	Long: `Configure your DataRobot environment URL with an interactive selection.

This command helps you choose the correct DataRobot environment:
  • US Cloud (most common): https://app.datarobot.com
  • EU Cloud: https://app.eu.datarobot.com  
  • Japan Cloud: https://app.jp.datarobot.com
  • Custom/On-Premise: Your organization's DataRobot URL

💡 If you're unsure, check the URL you use to login to DataRobot in your browser.`,
	Run: func(_ *cobra.Command, _ []string) {
		SetURLAction()
	},
}
