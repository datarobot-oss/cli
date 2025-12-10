// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package auth

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/envbuilder"
	"github.com/datarobot/cli/internal/repo"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

func checkCLICredentials() bool {
	allValid := true

	datarobotHost := config.GetBaseURL()
	if datarobotHost == "" {
		fmt.Println(tui.BaseTextStyle.Render("❌ No DataRobot URL configured."))
		fmt.Print(tui.BaseTextStyle.Render("Run "))
		fmt.Print(tui.InfoStyle.Render("dr auth set-url"))
		fmt.Println(tui.BaseTextStyle.Render(" to configure your DataRobot URL."))

		allValid = false
	}

	cliToken := config.GetAPIKey()

	if cliToken == "" {
		fmt.Println(tui.BaseTextStyle.Render("❌ No valid API key found in CLI config."))
		fmt.Print(tui.BaseTextStyle.Render("Run "))
		fmt.Print(tui.InfoStyle.Render("dr auth login"))
		fmt.Println(tui.BaseTextStyle.Render(" to authenticate."))
		fmt.Println(tui.BaseTextStyle.Render("\n  If this error persists, your DATAROBOT_API_TOKEN environment variable"))
		fmt.Println(tui.BaseTextStyle.Render("  contains an expired or invalid token. Unset it:"))
		fmt.Print(tui.BaseTextStyle.Render("  "))
		fmt.Print(tui.InfoStyle.Render("unset DATAROBOT_API_TOKEN"))
		fmt.Print(tui.BaseTextStyle.Render(" (or "))
		fmt.Print(tui.InfoStyle.Render("Remove-Item Env:\\DATAROBOT_API_TOKEN"))
		fmt.Println(tui.BaseTextStyle.Render(" on Windows)"))

		allValid = false
	} else {
		fmt.Println(tui.BaseTextStyle.Render("✅ CLI authentication is valid."))
	}

	return allValid
}

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check if DataRobot credentials are valid.",
	Long: `Verify that your DataRobot credentials are properly configured and valid.

If you're in a project directory with a '.env' file, this will also check those credentials.`,
	Run: func(_ *cobra.Command, _ []string) {
		allValid := true

		// Check .env credentials if in a repo
		// If not, check the CLI credentials only
		repoRoot, err := repo.FindRepoRoot()
		if err != nil || repoRoot == "" {
			if !checkCLICredentials() {
				os.Exit(1)
			}
			return
		}

		dotenvPath := filepath.Join(repoRoot, ".env")

		_, statErr := os.Stat(dotenvPath)
		if statErr != nil {
			fmt.Println(tui.BaseTextStyle.Render("⚠️ No '.env' file found in repository."))
			fmt.Print(tui.BaseTextStyle.Render("Run "))
			fmt.Print(tui.InfoStyle.Render("dr start"))
			fmt.Print(tui.BaseTextStyle.Render(" or "))
			fmt.Print(tui.InfoStyle.Render("dr dotenv setup"))
			fmt.Println(tui.BaseTextStyle.Render(" to create one."))
			os.Exit(1)
		}

		fileContents, readErr := os.ReadFile(dotenvPath)
		if readErr != nil {
			fmt.Println(tui.BaseTextStyle.Render("❌ Failed to read '.env' file."))
			fmt.Print(tui.BaseTextStyle.Render("Run "))
			fmt.Print(tui.InfoStyle.Render("dr start"))
			fmt.Print(tui.BaseTextStyle.Render(" or "))
			fmt.Print(tui.InfoStyle.Render("dr dotenv setup"))
			fmt.Println(tui.BaseTextStyle.Render(" to create one."))
			os.Exit(1)
		}

		lines := make([]string, 0)
		for _, line := range strings.Split(string(fileContents), "\n") {
			lines = append(lines, line+"\n")
		}

		variables := envbuilder.ParseVariablesOnly(lines)

		var envToken, envEndpoint string

		for _, v := range variables {
			if v.Name == "DATAROBOT_API_TOKEN" {
				envToken = v.Value
			}

			if v.Name == "DATAROBOT_ENDPOINT" {
				envEndpoint = v.Value
			}
		}

		if envToken == "" {
			fmt.Println(tui.BaseTextStyle.Render("⚠️ No DATAROBOT_API_TOKEN found in '.env'."))
			fmt.Print(tui.BaseTextStyle.Render("Run "))
			fmt.Print(tui.InfoStyle.Render("dr start"))
			fmt.Print(tui.BaseTextStyle.Render(" or "))
			fmt.Print(tui.InfoStyle.Render("dr dotenv setup"))
			fmt.Println(tui.BaseTextStyle.Render(" to configure the '.env' file."))
			os.Exit(1)
		}

		if envEndpoint == "" {
			fmt.Println(tui.BaseTextStyle.Render("⚠️ No DATAROBOT_ENDPOINT found in '.env'."))
			fmt.Print(tui.BaseTextStyle.Render("Run "))
			fmt.Print(tui.InfoStyle.Render("dr start"))
			fmt.Print(tui.BaseTextStyle.Render(" or "))
			fmt.Print(tui.InfoStyle.Render("dr dotenv setup"))
			fmt.Println(tui.BaseTextStyle.Render(" to configure the '.env' file."))
			os.Exit(1)
		}

		// Extract base URL from the endpoint for token verification
		envBaseURL, err := config.SchemeHostOnly(envEndpoint)
		if err != nil {
			fmt.Println(tui.BaseTextStyle.Render("❌ Invalid DATAROBOT_ENDPOINT in '.env'."))
			fmt.Print(tui.BaseTextStyle.Render("Run "))
			fmt.Print(tui.InfoStyle.Render("dr dotenv update"))
			fmt.Println(tui.BaseTextStyle.Render(" to fix the configuration."))
			os.Exit(1)
		}

		tokenValid, _ := config.VerifyToken(envBaseURL, envToken)
		if !tokenValid {
			fmt.Println(tui.BaseTextStyle.Render("❌ DATAROBOT_API_TOKEN in '.env' is invalid or expired."))
			fmt.Print(tui.BaseTextStyle.Render("Run "))
			fmt.Print(tui.InfoStyle.Render("dr dotenv update"))
			fmt.Println(tui.BaseTextStyle.Render(" to refresh credentials."))
			os.Exit(1)
		}
		fmt.Println(tui.BaseTextStyle.Render("✅ '.env' credentials are valid."))

		if !checkCLICredentials() {
			allValid = false
		}

		if !allValid {
			os.Exit(1)
		}
	},
}
