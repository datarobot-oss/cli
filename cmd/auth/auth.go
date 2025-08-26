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
	"github.com/spf13/viper"
)

// Store the API key in a file in the users home directory.
// In the real world this would probably need to be encrypted.

func waitForAPIKeyCallback(datarobotHost string) (string, error) {
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
		fmt.Println("\n\nPlease visit this link to connect your DataRobot credentials to the CLI")
		fmt.Println("(If you're prompted to log in, you may need to re-enter this URL):")
		fmt.Printf("%s/account/developer-tools?cliRedirect=true\n\n", datarobotHost)

		err := server.Serve(listen)
		if err != http.ErrServerClosed {
			log.Errorf("Server error: %v\n", err)
		}
	}()

	// Wait for the key from the handler
	apiKey := <-apiKeyChan

	fmt.Println("Successfully consumed API Key from API Request")
	// Now shut down the server after key is received
	if err := server.Shutdown(context.Background()); err != nil {
		return "", fmt.Errorf("error during shutdown: %v", err)
	}

	return apiKey, nil
}

func verifyAPIKey(datarobotHost string, apiKey string) (bool, error) {
	// Verifies if the datarobot host + api key pair correspond to a valid
	// pair.
	req, err := http.NewRequest(http.MethodGet, datarobotHost+"/api/v2/version/", nil)
	if err != nil {
		return false, err
	}

	req.Header.Add("Authorization", "bearer "+apiKey)

	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}

	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
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
