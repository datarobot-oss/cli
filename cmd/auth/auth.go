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
	"fmt"
	"net"
	"net/http"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/assets"
	"github.com/datarobot/cli/internal/misc/open"
	"github.com/spf13/viper"
)

// Store the API key in a file in the users home directory.
// In the real world this would probably need to be encrypted.

// apiKeyCallbackFunc is a variable that holds the function for retrieving API keys.
// This can be overridden in tests to mock the browser-based authentication flow.
var apiKeyCallbackFunc = waitForAPIKeyCallback

func waitForAPIKeyCallback(ctx context.Context, datarobotHost string) (string, error) {
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
		fmt.Println("Successfully consumed API Key from API Request")
		// Now shut down the server after key is received
		if err := server.Shutdown(ctx); err != nil {
			return "", fmt.Errorf("error during shutdown: %v", err)
		}

		return apiKey, nil
	case <-ctx.Done():
		fmt.Println("\nCtrl-C received, exiting...")
		return "", fmt.Errorf("interrupt request received")
	}

	return "", nil
}

func WriteConfigFileSilent() {
	err := viper.WriteConfig()
	if err != nil {
		log.Error(err)
		return
	}
}

func writeConfigFile() {
	WriteConfigFileSilent()

	fmt.Println("Config file written successfully.")
}
