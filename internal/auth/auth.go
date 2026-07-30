// Copyright 2026 DataRobot, Inc. and its affiliates.
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

package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/log"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/tui"
	"github.com/datarobot/cli/tui/hostpicker"
	"github.com/spf13/cobra"
)

// APIKeyCallbackFunc is a variable that holds the function for retrieving API keys.
// This can be overridden in tests to mock the browser-based authentication flow.
var APIKeyCallbackFunc = RunBrowserLogin

// AuthCallbackURL returns the DataRobot URL the user must visit to authorize the CLI.
func AuthCallbackURL(datarobotHost string) string {
	return datarobotHost + "/account/developer-tools?cliRedirect=true"
}

// ErrEnvCredentialsNotSet is returned when environment credentials are not fully configured.
var ErrEnvCredentialsNotSet = errors.New("environment credentials not set")

// PrintUnsetTokenInstructions prints platform-specific instructions for unsetting DATAROBOT_API_TOKEN.
func PrintUnsetTokenInstructions() {
	fmt.Print(tui.InfoStyle.Render("  unset DATAROBOT_API_TOKEN"))
	fmt.Print(tui.BaseTextStyle.Render(" (or "))
	fmt.Print(tui.InfoStyle.Render("Remove-Item Env:\\DATAROBOT_API_TOKEN"))
	fmt.Println(tui.BaseTextStyle.Render(" on Windows)"))
}

// EnvCredentials holds environment variable authentication credentials.
type EnvCredentials struct {
	Endpoint string
	Token    string
}

// GetEnvCredentials reads DATAROBOT_ENDPOINT and DATAROBOT_API_TOKEN from environment.
// Falls back to DATAROBOT_API_ENDPOINT if DATAROBOT_ENDPOINT is not set.
func GetEnvCredentials() EnvCredentials {
	endpoint := os.Getenv("DATAROBOT_ENDPOINT")
	if endpoint == "" {
		endpoint = os.Getenv("DATAROBOT_API_ENDPOINT")
	}

	return EnvCredentials{
		Endpoint: endpoint,
		Token:    os.Getenv("DATAROBOT_API_TOKEN"),
	}
}

// VerifyEnvCredentials checks if environment variable credentials are valid.
// Returns credentials and nil error if valid, credentials and error otherwise.
func VerifyEnvCredentials(ctx context.Context) (*EnvCredentials, error) {
	creds := GetEnvCredentials()
	if creds.Endpoint == "" || creds.Token == "" {
		return &creds, ErrEnvCredentialsNotSet
	}

	err := config.VerifyToken(ctx, creds.Endpoint, creds.Token)

	return &creds, err
}

// EnsureAuthenticatedE checks if valid authentication exists, and if not,
// triggers the login flow automatically. Returns an error if authentication
// fails, suitable for use in Cobra PreRunE hooks.
func EnsureAuthenticatedE(cmd *cobra.Command, _ []string) error {
	if !EnsureAuthenticated(cmd.Context()) {
		return errors.New("Authentication failed.")
	}

	return nil
}

// EnsureAuthenticated checks if valid authentication exists, and if not,
// triggers the login flow automatically. Returns true if authentication
// is valid or was successfully obtained.
func EnsureAuthenticated(ctx context.Context) bool { //nolint: cyclop
	if viperx.GetBool(config.SkipAuthKey) {
		log.Warn("Authentication checks are disabled via the '--skip-auth' flag. This may cause API calls to fail.")

		return true
	}

	// bindValidAuthEnv binds DATAROBOT ENDPOINT/API_TOKEN to viper config only if these credentials are valid
	creds, envErr := VerifyEnvCredentials(ctx)
	if envErr == nil {
		// Now map other environment variables to config keys
		// such as those used by the DataRobot platform or other SDKs
		// and clients. If the DATAROBOT_CLI equivalents are not set,
		// then Viper will fallback to these
		_ = viperx.BindEnv("endpoint", "DATAROBOT_ENDPOINT", "DATAROBOT_API_ENDPOINT")
		_ = viperx.BindEnv("token", "DATAROBOT_API_TOKEN")

		return true
	}

	datarobotHost := GetBaseURLOrAsk()
	if datarobotHost == "" {
		// Appropriate error message was already displayed in GetBaseURLOrAsk() and SetURLAction()
		return false
	}

	_, viperErr := config.GetAPIKey(ctx)
	if viperErr == nil {
		// Valid token exists in viper config file
		return true
	}

	skipAuthFlow := false

	if errors.Is(envErr, context.DeadlineExceeded) {
		envDatarobotHost, _ := config.SchemeHostOnly(creds.Endpoint)

		fmt.Print(tui.BaseTextStyle.Render("❌ Connection to "))
		fmt.Print(tui.InfoStyle.Render(envDatarobotHost))
		fmt.Println(tui.BaseTextStyle.Render(" from DATAROBOT_ENDPOINT environment variable timed out."))
		fmt.Println(tui.BaseTextStyle.Render("Check your network and try again."))

		skipAuthFlow = true
	} else if creds.Token != "" {
		fmt.Println(tui.BaseTextStyle.Render("Your DATAROBOT_API_TOKEN environment variable"))
		fmt.Println(tui.BaseTextStyle.Render("contains an expired or invalid token. Unset it:"))
		PrintUnsetTokenInstructions()

		skipAuthFlow = true
	}

	if errors.Is(viperErr, context.DeadlineExceeded) {
		fmt.Print(tui.BaseTextStyle.Render("❌ Connection to "))
		fmt.Print(tui.InfoStyle.Render(datarobotHost))
		fmt.Println(tui.BaseTextStyle.Render(" from dr cli config timed out."))
		fmt.Println(tui.BaseTextStyle.Render("Check your network and try again."))

		skipAuthFlow = true
	}

	if skipAuthFlow {
		return false
	}

	// No valid token, attempt to get one
	log.Warn("No valid API key found. Starting authentication flow...")

	// Auto-retrieve new credentials without prompting
	viperx.Set(config.DataRobotAPIKey, "")

	key, err := APIKeyCallbackFunc(ctx, datarobotHost)
	if err != nil {
		log.Error("Failed to retrieve API key.", "error", err)
		return false
	}

	viperx.Set(config.DataRobotAPIKey, strings.ReplaceAll(key, "\n", ""))

	err = WriteConfigFileSilent()
	if err != nil {
		log.Error("Failed to write config file.", "error", err)
		return false
	}

	log.Info("Authentication successful")

	return true
}

func WriteConfigFileSilent() error {
	// Only persist allowlisted keys (see config.PersistableKeys). This avoids
	// writing transient flags such as --yes back to drconfig.yaml.
	err := config.UpdateConfigFile()
	if err != nil {
		log.Error(err)
		return err
	}

	return nil
}

func WriteConfigFile() error {
	err := WriteConfigFileSilent()
	if err != nil {
		return err
	}

	fmt.Println("Config file written successfully.")

	return nil
}

func printSetURLPrompt() {
	fmt.Println("🌐 DataRobot URL Configuration")
	fmt.Println("")
	fmt.Println("Choose your DataRobot environment:")
	fmt.Println("")
	fmt.Println("┌────────────────────────────────────────────────────────┐")
	fmt.Println("│  [1] 🇺🇸 US Cloud        https://app.datarobot.com      │")
	fmt.Println("│  [2] 🇪🇺 EU Cloud        https://app.eu.datarobot.com   │")
	fmt.Println("│  [3] 🇯🇵 Japan Cloud     https://app.jp.datarobot.com   │")
	fmt.Println("│      🏢 Custom          Enter your custom URL          │")
	fmt.Println("└────────────────────────────────────────────────────────┘")
	fmt.Println("")
	fmt.Println("🔗 Don't know which one? Check your DataRobot login page URL in your browser.")
	fmt.Println("")
	fmt.Print("Enter your choice: ")
}

func askForNewHost() bool {
	datarobotHost := config.GetBaseURL()

	if len(datarobotHost) == 0 {
		return true
	}

	fmt.Printf("A DataRobot URL of %s is already present; do you want to overwrite it? (y/N): ", datarobotHost)

	selectedOption, err := reader.ReadString()
	if err != nil {
		return false
	}

	return strings.ToLower(strings.TrimSpace(selectedOption)) == "y"
}

// SetURLAction asks the user which DataRobot environment to use and stores the
// answer. It reports whether the configured URL changed.
//
// On an interactive terminal this is a TUI list (arrow keys plus Enter, US Cloud
// preselected). On a terminal with DATAROBOT_CLI_NON_INTERACTIVE set it falls back
// to the plain-text numbered prompt, which is what the expect based smoke tests
// drive - they spawn a PTY and set that variable. With no terminal at all there is
// nothing to prompt, so it prints the non-interactive instructions and gives up.
func SetURLAction() bool {
	// Every path below reads stdin, askForNewHost's overwrite prompt included, so
	// this has to come first. Without a terminal no answer is coming: on Windows
	// cancelreader discards a redirected stdin in favour of the real console and
	// then blocks forever waiting for keystrokes. See internal/misc/reader.
	if !reader.IsStdinTerminal() {
		printURLUnchanged()

		return false
	}

	if !askForNewHost() {
		printURLUnchanged()

		return false
	}

	if interactiveURLPickerAvailable() {
		return selectURLInteractively()
	}

	return readURLFromStdin()
}

// interactiveURLPickerAvailable reports whether the TUI picker can be shown.
//
// The testing.Testing() guard matches internal/misc/open: `go test` normally hands
// the binary /dev/null for stdin, but under a PTY it would not, and a test that
// launched the real picker would block until the suite timed out.
func interactiveURLPickerAvailable() bool {
	if testing.Testing() {
		return false
	}

	return reader.IsStdinTerminal() && !reader.IsNonInteractive()
}

// selectURLInteractively runs the TUI environment picker.
func selectURLInteractively() bool {
	for {
		url, ok, err := hostpicker.Select()
		if err != nil {
			log.Error("Failed to show the environment picker.", "error", err)
			printURLUnchanged()

			return false
		}

		if !ok {
			// The user pressed Esc or Ctrl-C.
			printURLUnchanged()

			return false
		}

		err = config.SetURLToConfig(url)
		if err == nil {
			return true
		}

		if errors.Is(err, config.ErrInvalidURL) {
			fmt.Println(tui.ErrorStyle.Render("Invalid URL provided. Verify your URL and try again."))

			continue
		}

		log.Error(err)
		printURLUnchanged()

		return false
	}
}

// readURLFromStdin is the non-interactive fallback: a numbered prompt read from
// stdin, so DATAROBOT_CLI_NON_INTERACTIVE and the expect-based smoke tests keep
// working.
func readURLFromStdin() bool {
	for {
		printSetURLPrompt()

		// reader.ReadString strips the newline, so an empty answer is "" - testing
		// for "\n" here would send it to SetURLToConfig and loop on "Invalid URL".
		url, err := reader.ReadString()
		if err != nil || url == "" {
			break
		}

		err = config.SetURLToConfig(url)
		if err != nil {
			if errors.Is(err, config.ErrInvalidURL) {
				fmt.Print("\nInvalid URL provided. Verify your URL and try again.\n\n")
				continue
			}

			log.Error(err)

			break
		}

		fmt.Println("Thank you for providing the URL. Validating it and retrieving your API key...")

		return true
	}

	printURLUnchanged()

	return false
}

// printURLUnchanged explains that nothing was configured and how to proceed.
//
// The bare "Exiting without changing the DataRobot URL." this replaces left users
// with no next step, which is especially unhelpful when the cause was redirected
// stdin rather than a deliberate cancel.
func printURLUnchanged() {
	fmt.Println(tui.BaseTextStyle.Render("No DataRobot URL was configured."))
	fmt.Print(tui.BaseTextStyle.Render("Set one non-interactively with "))
	fmt.Println(tui.InfoStyle.Render("dr auth set-url <url>"))
	fmt.Print(tui.BaseTextStyle.Render("or export "))
	fmt.Print(tui.InfoStyle.Render("DATAROBOT_ENDPOINT"))
	fmt.Print(tui.BaseTextStyle.Render(" and "))
	fmt.Print(tui.InfoStyle.Render("DATAROBOT_API_TOKEN"))
	fmt.Println(tui.BaseTextStyle.Render(" to skip the login entirely."))
}

func GetBaseURLOrAsk() string {
	datarobotHost := config.GetBaseURL()
	if datarobotHost == "" {
		log.Warn("No DataRobot URL configured. Running auth setup...")

		SetURLAction()

		datarobotHost = config.GetBaseURL()
		if datarobotHost == "" {
			log.Error("Failed to configure the DataRobot URL.")
			return ""
		}
	}

	return datarobotHost
}
