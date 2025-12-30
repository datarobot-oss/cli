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
	"strings"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/assets"
	"github.com/datarobot/cli/internal/config"
	"github.com/datarobot/cli/internal/misc/open"
	"github.com/datarobot/cli/internal/misc/reader"
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
func EnsureAuthenticated(ctx context.Context) bool {
	if viper.GetBool("skip_auth") {
		log.Warn("Authentication checks are disabled via the '--skip-auth' flag. This may cause API calls to fail.")

		return true
	}

	datarobotHost := config.GetBaseURL()
	if datarobotHost == "" {
		log.Warn("No DataRobot URL configured. Running auth setup...")

		SetURLAction()

		datarobotHost = config.GetBaseURL()
		if datarobotHost == "" {
			log.Error("Failed to configure the DataRobot URL.")
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

	key, err := APIKeyCallbackFunc(ctx, datarobotHost)
	if err != nil {
		log.Error("Failed to retrieve API key.", "error", err)
		return false
	}

	viper.Set(config.DataRobotAPIKey, strings.ReplaceAll(key, "\n", ""))
	WriteConfigFileSilent()

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

func WriteConfigFileSilent() {
	err := viper.WriteConfig()
	if err != nil {
		log.Error(err)
		return
	}
}

func WriteConfigFile() {
	WriteConfigFileSilent()

	fmt.Println("Config file written successfully.")
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
