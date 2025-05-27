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
	"errors"
	"fmt"
	"os"
	"strings"
	"context"
	"net/http"

	"github.com/spf13/cobra"
)

// Store the API key in a file in the users home directory.
// In the real world this would probably need to be encrypted.
var (
	authFileDir  = os.Getenv("HOME") + "/.datarobot-cli/auth"
	authFileName = "datarobot-key"
	authFilePath = authFileDir + "/" + authFileName
)

func createAuthFileDirIfNotExists() error {
	// TODO: we create a CLI config file here basically, so need to reflect that in the method names and structure
	_, err := os.Stat(authFilePath)
	if (err == nil) {
		// File exists, do nothing
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("error checking auth file: %w", err)
	}

	// file was not found, let's create it

	err = os.MkdirAll(authFileDir, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to create auth file directory: %w", err)
	}

	_, err = os.Create(authFilePath)
	if err != nil {
		return fmt.Errorf("failed to create auth file: %w", err)
	}

	return nil
}

func clearAuthFile() error {
	return os.Truncate(authFilePath, 0)
}


func waitForAPIKeyCallback() string {
	addr := "localhost:51164"
	apiKeyChan := make(chan string)

	mux := http.NewServeMux()
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.URL.Query().Get("key")
		fmt.Fprintf(w, "Successfully processed API key, you may close this window.")
		apiKeyChan <- apiKey // send the key to the main goroutine
	})

	// Start the server in a goroutine
	go func() {
	    fmt.Println("Via this link : https://staging.datarobot.com/account/developer-tools?cliRedirect=true")
		if (err := server.ListenAndServe(); err != http.ErrServerClosed) {
			fmt.Printf("Server error: %v\n", err)
		}
	}()

	// Wait for the key from the handler
	apiKey := <-apiKeyChan

	// Now shut down the server after key is received
	if err := server.Shutdown(context.Background()); err != nil {
		fmt.Printf("Error during shutdown: %v\n", err)
	}

	return apiKey
}



func LoginAction() error {
	reader := bufio.NewReader(os.Stdin)

	if err := createAuthFileDirIfNotExists(); err != nil {
		panic(err)
	}

	fileInfo, _ := os.Stat(authFilePath)

	if fileInfo.Size() > 0 { //nolint: nestif
		fmt.Println("An API key is already present, do you want to overwrite? (y/N): ")
		// TODO: make this block simpler
		// TODO Verify the API Key is still valid
		selectedOption, err := reader.ReadString('\n')
		if err != nil {
			panic(err)
		}

		if strings.ToLower(strings.Replace(selectedOption, "\n", "", -1)) == "y" {
			if err := clearAuthFile(); err != nil {
				panic(err)
			}
		} else {
			fmt.Println("Exiting without overwriting the API key.")
			return nil
		}
	}

	fmt.Println("Assisted authentication is not supported yet.")
	fmt.Println("Please use the DataRobot web interface to log in and get an API key.")

	file, err := os.Create(authFilePath)
	if err != nil {
		return fmt.Errorf("failed to create auth file: %w", err)
	}

    key:= waitForAPIKeyCallback();

	if _, err := file.WriteString(strings.Replace(key, "\n", "", -1)); err != nil {
		return err
	}

	return nil
}

func LogoutAction() error {
	if err := createAuthFileDirIfNotExists(); err != nil {
		panic(err)
	}

	if err := clearAuthFile(); err != nil {
		panic(err)
	}

	return nil
}

func GetAPIKey() (string, error) {
	if err := createAuthFileDirIfNotExists(); err != nil {
		panic(err)
	}

	key, err := os.ReadFile(authFilePath)
	if err != nil {
		return "", err
	}

	fileInfo, _ := os.Stat(authFilePath)

	if fileInfo.Size() == 0 {
		fmt.Println("No API key found, please login first. Exiting.")
		return "", errors.New("no API key found")
	}

	return string(key), nil
}

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login to DataRobot",
	Long:  `Login to DataRobot to get and store an API key that can be used for other operation in the cli.`,
	Run: func(_ *cobra.Command, _ []string) {
		_ = LoginAction() // TODO: handler errors properly
	},
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Logout from DataRobot",
	Long:  `Logout from DataRobot and clear the stored API key.`,
	Run: func(_ *cobra.Command, _ []string) {
		_ = LogoutAction() // TODO: handler errors properly
	},
}

func init() {
	AuthCmd.AddCommand(
		loginCmd,
		logoutCmd,
	)
}
