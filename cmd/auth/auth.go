// Copyright {{current_year}} DataRobot, Inc. and its affiliates.
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

	"github.com/spf13/cobra"
)

// Store the API key in a file in the users home directory.
// In the real world this would probably need to be encrypted.
var (
	authFileDir  string = os.Getenv("HOME") + "/.datarobot-cli/auth"
	authFileName string = "datarobot-key"
	authFilePath string = authFileDir + "/" + authFileName
)

func createAuthFileDirIfNotExists() error {
	_, err := os.Stat(authFilePath)

	if errors.Is(err, os.ErrNotExist) {
		os.MkdirAll(authFileDir, os.ModePerm)
		os.Create(authFilePath)
		return nil
	}

	return err
}

func clearAuthFile() error {
	return os.Truncate(authFilePath, 0)
}

func LoginAction() error {
	reader := bufio.NewReader(os.Stdin)

	if err := createAuthFileDirIfNotExists(); err != nil {
		panic(err)
	}

	fileInfo, _ := os.Stat(authFilePath)

	if fileInfo.Size() > 0 {
		fmt.Println("An API key is already present, do you want to overwrite? (y/N): ")
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

	key, err := reader.ReadString('\n')
	if err != nil {
		panic(err)
	}

	if _, err := file.WriteString(strings.Replace(key, "\n", "", -1)); err != nil {
		panic(err)
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
	Run: func(cmd *cobra.Command, args []string) {
		LoginAction()
	},
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Logout from DataRobot",
	Long:  `Logout from DataRobot and clear the stored API key.`,
	Run: func(cmd *cobra.Command, args []string) {
		LogoutAction()
	},
}

func init() {
	AuthCmd.AddCommand(
		loginCmd,
		logoutCmd,
	)
}
