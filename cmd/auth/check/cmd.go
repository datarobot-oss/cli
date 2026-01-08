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

package check

import (
	"context"
	"errors"
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

	_, err := config.GetAPIKey()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			fmt.Print(tui.BaseTextStyle.Render("❌ Connection to "))
			fmt.Print(tui.InfoStyle.Render(datarobotHost))
			fmt.Println(tui.BaseTextStyle.Render(" timed out. Check your network and try again."))
		} else {
			fmt.Println(tui.BaseTextStyle.Render("❌ No valid API key found in CLI config."))
			fmt.Print(tui.BaseTextStyle.Render("Run "))
			fmt.Print(tui.InfoStyle.Render("dr auth login"))
			fmt.Println(tui.BaseTextStyle.Render(" to authenticate."))
		}

		allValid = false
	} else {
		fmt.Println(tui.BaseTextStyle.Render("✅ CLI authentication is valid."))
	}

	return allValid
}

func printDotenvMissingError() {
	fmt.Println(tui.BaseTextStyle.Render("⚠️ No '.env' file found in repository."))
	fmt.Print(tui.BaseTextStyle.Render("Run "))
	fmt.Print(tui.InfoStyle.Render("dr start"))
	fmt.Print(tui.BaseTextStyle.Render(" or "))
	fmt.Print(tui.InfoStyle.Render("dr dotenv setup"))
	fmt.Println(tui.BaseTextStyle.Render(" to create one."))
}

func printDotenvReadError() {
	fmt.Println(tui.BaseTextStyle.Render("❌ Failed to read '.env' file."))
	fmt.Print(tui.BaseTextStyle.Render("Run "))
	fmt.Print(tui.InfoStyle.Render("dr start"))
	fmt.Print(tui.BaseTextStyle.Render(" or "))
	fmt.Print(tui.InfoStyle.Render("dr dotenv setup"))
	fmt.Println(tui.BaseTextStyle.Render(" to create one."))
}

func printMissingEnvVarError(varName string) {
	fmt.Println(tui.BaseTextStyle.Render(fmt.Sprintf("⚠️ No %s found in '.env'.", varName)))
	fmt.Print(tui.BaseTextStyle.Render("Run "))
	fmt.Print(tui.InfoStyle.Render("dr start"))
	fmt.Print(tui.BaseTextStyle.Render(" or "))
	fmt.Print(tui.InfoStyle.Render("dr dotenv setup"))
	fmt.Println(tui.BaseTextStyle.Render(" to configure the '.env' file."))
}

func extractDotenvVars(dotenvPath string) (string, string, error) {
	fileContents, readErr := os.ReadFile(dotenvPath)
	if readErr != nil {
		return "", "", readErr
	}

	lines := make([]string, 0)

	for _, line := range strings.Split(string(fileContents), "\n") {
		lines = append(lines, line+"\n")
	}

	variables := envbuilder.ParseVariablesOnly(lines)

	var dotenvToken, dotenvEndpoint string

	for _, v := range variables {
		if v.Name == "DATAROBOT_API_TOKEN" {
			dotenvToken = v.Value
		}

		if v.Name == "DATAROBOT_ENDPOINT" {
			dotenvEndpoint = v.Value
		}
	}

	return dotenvToken, dotenvEndpoint, nil
}

func verifyDotenvToken(dotenvEndpoint, dotenvToken string) bool {
	dotenvBaseURL, err := config.SchemeHostOnly(dotenvEndpoint)
	if err != nil {
		fmt.Println(tui.BaseTextStyle.Render("❌ Invalid DATAROBOT_ENDPOINT in '.env'."))
		fmt.Print(tui.BaseTextStyle.Render("Run "))
		fmt.Print(tui.InfoStyle.Render("dr dotenv update"))
		fmt.Println(tui.BaseTextStyle.Render(" to fix the configuration."))

		return false
	}

	err = config.VerifyToken(dotenvEndpoint, dotenvToken)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			fmt.Print(tui.BaseTextStyle.Render("❌ Connection to "))
			fmt.Print(tui.InfoStyle.Render(dotenvBaseURL))
			fmt.Println(tui.BaseTextStyle.Render(" timed out. Check your network and try again."))
		} else {
			fmt.Println(tui.BaseTextStyle.Render("❌ DATAROBOT_API_TOKEN in '.env' is invalid or expired."))
			fmt.Print(tui.BaseTextStyle.Render("Run "))
			fmt.Print(tui.InfoStyle.Render("dr dotenv update"))
			fmt.Println(tui.BaseTextStyle.Render(" to refresh credentials."))
		}

		return false
	}

	return true
}

func checkDotenvCredentials(repoRoot string) bool {
	dotenvPath := filepath.Join(repoRoot, ".env")

	_, statErr := os.Stat(dotenvPath)
	if statErr != nil {
		printDotenvMissingError()

		return false
	}

	dotenvToken, dotenvEndpoint, err := extractDotenvVars(dotenvPath)
	if err != nil {
		printDotenvReadError()

		return false
	}

	if dotenvToken == "" {
		printMissingEnvVarError("DATAROBOT_API_TOKEN")

		return false
	}

	if dotenvEndpoint == "" {
		printMissingEnvVarError("DATAROBOT_ENDPOINT")

		return false
	}

	if !verifyDotenvToken(dotenvEndpoint, dotenvToken) {
		return false
	}

	fmt.Println(tui.BaseTextStyle.Render("✅ '.env' credentials are valid."))

	return true
}

func Run(_ *cobra.Command, _ []string) {
	// Check .env credentials if in a repo
	// If not, check the CLI credentials only
	repoRoot, err := repo.FindRepoRoot()
	if err != nil || repoRoot == "" {
		if checkCLICredentials() {
			return
		}

		os.Exit(1)
	}

	if checkDotenvCredentials(repoRoot) {
		return
	}

	if checkCLICredentials() {
		return
	}

	os.Exit(1)
}

func Cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Check if DataRobot credentials are valid.",
		Long: `Verify that your DataRobot credentials are properly configured and valid.

If you're in a project directory with a '.env' file, this will check those credentials.`,
		Run: Run,
	}
}
