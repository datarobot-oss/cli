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
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/assets"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/misc/open"
	"github.com/datarobot/cli/internal/misc/reader"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/viper"
)

// APIKeyCallbackFunc is a variable that holds the function for retrieving API keys.
// This can be overridden in tests to mock the browser-based authentication flow.
var APIKeyCallbackFunc = WaitForAPIKeyCallback

// EnsureAuthenticatedE checks if valid authentication exists, and if not,
// triggers the login flow automatically. Returns an error if authentication
// fails, suitable for use in Cobra PreRunE hooks.
func EnsureAuthenticatedE(ctx context.Context) error {
	if !EnsureAuthenticated(ctx) {
		return errors.New("Authentication failed.")
	}

	return nil
}

// EnsureAuthenticated checks if valid authentication exists, and if not,
// triggers the login flow automatically. This is a non-interactive version
// intended for use in automated workflows. Returns true if authentication
// is valid or was successfully obtained.
func EnsureAuthenticated(ctx context.Context) bool { //nolint: cyclop
	if viper.GetBool("skip_auth") {
		log.Warn("Authentication checks are disabled via the '--skip-auth' flag. This may cause API calls to fail.")

		return true
	}

	// bindValidAuthEnv binds DATAROBOT ENDPOINT/API_TOKEN to viper config only if these credentials are valid
	envEndpoint := os.Getenv("DATAROBOT_ENDPOINT")
	envToken := os.Getenv("DATAROBOT_API_TOKEN")

	if envEndpoint == "" {
		if apiEndpoint := os.Getenv("DATAROBOT_API_ENDPOINT"); apiEndpoint != "" {
			envEndpoint = apiEndpoint
		}
	}

	envErr := config.VerifyToken(envEndpoint, envToken)
	if envErr == nil {
		// Now map other environment variables to config keys
		// such as those used by the DataRobot platform or other SDKs
		// and clients. If the DATAROBOT_CLI equivalents are not set,
		// then Viper will fallback to these
		_ = viper.BindEnv("endpoint", "DATAROBOT_ENDPOINT", "DATAROBOT_API_ENDPOINT")
		_ = viper.BindEnv("token", "DATAROBOT_API_TOKEN")

		return true
	}

	datarobotHost := GetBaseURLOrAsk()
	if datarobotHost == "" {
		// Appropriate error message was already displayed in GetBaseURLOrAsk() and SetURLAction()
		return false
	}

	_, viperErr := config.GetAPIKey()
	if viperErr == nil {
		// Valid token exists in viper config file
		return true
	}

	skipAuthFlow := false

	if errors.Is(envErr, context.DeadlineExceeded) {
		envDatarobotHost, _ := config.SchemeHostOnly(envEndpoint)

		fmt.Print(tui.BaseTextStyle.Render("❌ Connection to "))
		fmt.Print(tui.InfoStyle.Render(envDatarobotHost))
		fmt.Println(tui.BaseTextStyle.Render(" from DATAROBOT_ENDPOINT environment variable timed out."))
		fmt.Println(tui.BaseTextStyle.Render("Check your network and try again."))

		skipAuthFlow = true
	} else if envToken != "" {
		fmt.Println(tui.BaseTextStyle.Render("Your DATAROBOT_API_TOKEN environment variable"))
		fmt.Println(tui.BaseTextStyle.Render("contains an expired or invalid token. Unset it:"))
		fmt.Print(tui.InfoStyle.Render("  unset DATAROBOT_API_TOKEN"))
		fmt.Print(tui.BaseTextStyle.Render(" (or "))
		fmt.Print(tui.InfoStyle.Render("Remove-Item Env:\\DATAROBOT_API_TOKEN"))
		fmt.Println(tui.BaseTextStyle.Render(" on Windows)"))

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
	viper.Set(config.DataRobotAPIKey, "")

	key, err := APIKeyCallbackFunc(ctx, datarobotHost)
	if err != nil {
		log.Error("Failed to retrieve API key.", "error", err)
		return false
	}

	viper.Set(config.DataRobotAPIKey, strings.ReplaceAll(key, "\n", ""))

	err = WriteConfigFileSilent()
	if err != nil {
		log.Error("Failed to write config file.", "error", err)
		return false
	}

	log.Info("Authentication successful")

	return true
}

func WaitForAPIKeyCallback(ctx context.Context, datarobotHost string) (string, error) {
	addr := "localhost:51164"
	apiKeyChan := make(chan string, 1) // If we don't have a buffer of 1, this may hang.

	mux := http.NewServeMux()
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.URL.Query().Get("key")

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = assets.Write(w, "templates/success.html")

		apiKeyChan <- apiKey // send the key to the main goroutine
	})

	listen, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}

	// Start the server in a goroutine
	go func() {
		authURL := datarobotHost + "/account/developer-tools?cliRedirect=true"

		fmt.Println("\n\nPlease visit this link to connect your DataRobot credentials to the CLI")
		fmt.Println("(If you're prompted to log in, you may need to re-enter this URL):")
		fmt.Printf("%s\n\n", authURL)

		open.Open(authURL)

		err := server.Serve(listen)
		if err != http.ErrServerClosed {
			log.Errorf("Server error: %v\n", err)
		}
	}()

	select {
	// Wait for the key from the handler
	case apiKey := <-apiKeyChan:
		fmt.Println("Successfully consumed API key from API request")
		// Now shut down the server after key is received
		if err := server.Shutdown(ctx); err != nil {
			return "", fmt.Errorf("Error during shutdown: %v", err)
		}

		return apiKey, nil
	case <-ctx.Done():
		fmt.Println("\nCtrl-C received, exiting...")
		return "", errors.New("Interrupt request received.")
	}
}

func WriteConfigFileSilent() error {
	err := viper.WriteConfig()
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

func SetURLAction() bool {
	if askForNewHost() {
		for {
			printSetURLPrompt()

			url, err := reader.ReadString()
			if err != nil || url == "\n" {
				break
			}

			err = config.SaveURLToConfig(url)
			if err != nil {
				if errors.Is(err, config.ErrInvalidURL) {
					fmt.Print("\nInvalid URL provided. Verify your URL and try again.\n\n")
					continue
				}

				break
			}

			fmt.Println("Environment URL configured successfully!")

			return true
		}
	}

	fmt.Println("Exiting without changing the DataRobot URL.")

	return false
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
