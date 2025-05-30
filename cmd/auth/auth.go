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
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"sigs.k8s.io/yaml"
)

// Store the API key in a file in the users home directory.
// In the real world this would probably need to be encrypted.
var (
	configFileDir  = os.Getenv("HOME") + "/.config/datarobot"
	configFileName = "drconfig.yaml"
	configFilePath = configFileDir + "/" + configFileName
)

type PartialConfig struct {
	Token    string `yaml:"token"`
	Endpoint string `yaml:"endpoint"`
}

var DataRobotAPIKey = "token"

func createConfigFileDirIfNotExists() error {
	_, err := os.Stat(configFilePath)
	if err == nil {
		// File exists, do nothing
		return nil
	}

	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("error checking config file: %w", err)
	}

	// file was not found, let's create it

	err = os.MkdirAll(configFileDir, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to create config file directory: %w", err)
	}

	_, err = os.Create(configFilePath)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}

	return nil
}

func setValueInConfigFile(key string, value string) error {
	// Set the key of the YAML to be a given value.
	// The "key" here is either token or endpoint, and the value
	// is empty string or not a key (if it's undefined) or the given value
	data, err := os.ReadFile(configFilePath)
	if err != nil {
		return err
	}

	var full map[string]interface{}
	if err := yaml.Unmarshal(data, &full); err != nil {
		return err
	}

	if full == nil {
		full = make(map[string]interface{})
	}

	full[key] = value

	updatedYAML, err := yaml.Marshal(full)
	if err != nil {
		return err
	}

	err = os.WriteFile(configFilePath, updatedYAML, 0o644)
	if err != nil {
		return err
	}

	return nil
}

func readValueFromConfigFile(key string) (string, error) {
	data, err := os.ReadFile(configFilePath)
	if err != nil {
		return "", err
	}

	var full map[string]interface{}
	if err := yaml.Unmarshal(data, &full); err != nil {
		return "", err
	}

	value, exists := full[key]

	if !exists {
		return "", nil
	}

	return value.(string), nil
}

func waitForAPIKeyCallback(datarobotHost string) string {
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
		panic(err)
	}

	// Start the server in a goroutine
	go func() {
		fmt.Printf("\n\nPlease visit this link to connect your DataRobot creds to the CLI \n(If you're prompted to log in, you may need to re-enter this URL):\n%s/account/developer-tools?cliRedirect=true\n\n", datarobotHost)

		err := server.Serve(listen)
		if err != http.ErrServerClosed {
			fmt.Printf("Server error: %v\n", err)
		}
	}()

	// Wait for the key from the handler
	apiKey := <-apiKeyChan

	fmt.Println("Successfully consumed API Key from API Request")
	// Now shut down the server after key is received
	if err := server.Shutdown(context.Background()); err != nil {
		fmt.Printf("Error during shutdown: %v\n", err)
	}

	return apiKey
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

func LoginAction() error { //nolint: cyclop
	reader := bufio.NewReader(os.Stdin)

	if err := createConfigFileDirIfNotExists(); err != nil {
		panic(err)
	}

	fileInfo, _ := os.Stat(configFilePath)

	datarobotHost, err := GetURL(false)
	if err != nil {
		panic(err)
	}

	if fileInfo.Size() > 0 { //nolint: nestif
		currentKey, err := GetAPIKey()
		if err != nil {
			panic(err)
		}

		isValidKeyPair, err := verifyAPIKey(datarobotHost, currentKey)
		if err != nil {
			panic(err)
		}

		if isValidKeyPair {
			fmt.Println("An API key is already present, do you want to overwrite? (y/N): ")

			selectedOption, err := reader.ReadString('\n')
			if err != nil {
				panic(err)
			}

			if strings.ToLower(strings.Replace(selectedOption, "\n", "", -1)) == "y" {
				// Set the DataRobot API key to be an empty string
				if err := setValueInConfigFile(DataRobotAPIKey, ""); err != nil {
					panic(err)
				}
			} else {
				fmt.Println("Exiting without overwriting the API key.")
				return nil
			}
		} else {
			fmt.Println("The stored API key is invalid or expired. Retrieving a new one")
		}
	}

	key := waitForAPIKeyCallback(datarobotHost)
	if err := setValueInConfigFile(DataRobotAPIKey, strings.Replace(key, "\n", "", -1)); err != nil {
		return err
	}

	return nil
}

func LogoutAction() error {
	if err := createConfigFileDirIfNotExists(); err != nil {
		panic(err)
	}

	if err := setValueInConfigFile(DataRobotAPIKey, ""); err != nil {
		panic(err)
	}

	return nil
}

func GetAPIKey() (string, error) {
	// Returns the API key if there is one, otherwise returns an empty string
	if err := createConfigFileDirIfNotExists(); err != nil {
		return "", err
	}

	key, err := readValueFromConfigFile(DataRobotAPIKey)
	if err != nil {
		return "", err
	}

	return key, nil
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
		setURLCmd,
	)
}
