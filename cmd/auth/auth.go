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
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/datarobot/cli/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Store the API key in a file in the users home directory.
// In the real world this would probably need to be encrypted.

var DataRobotAPIKey = "token"

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

		fmt.Fprint(w, "Successfully processed API key, you may close this window.")

		apiKeyChan <- apiKey // send the key to the main goroutine
	})

	listen, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}

	// Start the server in a goroutine
	go func() {
		fmt.Printf("\n\nPlease visit this link to connect your DataRobot creds to the CLI \n(If you're prompted to log in, you may need to re-enter this URL):\n%s/account/developer-tools?cliRedirect=true\n\n", datarobotHost)

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

func writeConfigFile() {
	err := viper.WriteConfig()
	if err != nil {
		log.Error(err)
		return
	}

	fmt.Println("Config file written successfully.")
}

func LoginAction() error {
	reader := bufio.NewReader(os.Stdin)

	err := config.ReadConfigFile("")
	if err != nil {
		return err
	}

	datarobotHost, err := GetURL(false)
	if err != nil {
		return err
	}

	currentKey := viper.GetString(DataRobotAPIKey)

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
			viper.Set(DataRobotAPIKey, "")
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

	viper.Set(DataRobotAPIKey, strings.Replace(key, "\n", "", -1))

	writeConfigFile()

	return nil
}

func LogoutAction() error {
	viper.Set(DataRobotAPIKey, DataRobotAPIKey)

	writeConfigFile()

	return nil
}

func GetAPIKey() (string, error) {
	// Returns the API key if there is one, otherwise returns an empty string
	key := viper.GetString(DataRobotAPIKey)

	return key, nil
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

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Logout from DataRobot",
	Long:  `Logout from DataRobot and clear the stored API key.`,
	Run: func(_ *cobra.Command, _ []string) {
		err := LogoutAction()
		if err != nil {
			log.Error(err)
		}
	},
}
