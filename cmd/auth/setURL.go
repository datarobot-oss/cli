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
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// Store the DataRobot URL in a file in the users home directory.
// In the real world this would probably need to be encrypted.
var (
	urlFileName = "datarobot-url"
	urlFilePath = authFileDir + "/" + urlFileName
)

func createURLFileDirIfNotExists() error {
	_, err := os.Stat(urlFilePath)
	if err == nil {
		// File exists, do nothing
		return nil
	}

	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("error checking auth file: %w", err)
	}

	// file was not found, let's create it

	err = os.MkdirAll(authFileDir, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to create url file directory: %w", err)
	}

	_, err = os.Create(urlFilePath)
	if err != nil {
		return fmt.Errorf("failed to create auth file: %w", err)
	}

	return nil
}

func createOrUpdateUrl(url string) error {
	return os.WriteFile(urlFilePath, []byte(url), 0644)
}

func readUrlFromFile() (string, error) {
	data, err := os.ReadFile(urlFilePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func clearUrlFile() error {
	return os.Truncate(urlFilePath, 0)
}

func getBaseURL(input string) (string, error) {
	parsedURL, err := url.Parse(input)
	if err != nil {
		return "", err
	}

	base := fmt.Sprintf("%s://%s", parsedURL.Scheme, parsedURL.Host)
	return base, nil
}

func SetURLAction() {
	err := createURLFileDirIfNotExists()
	if err != nil {
		panic(err)
	}

	reader := bufio.NewReader(os.Stdin)
	urlContent, err := readUrlFromFile()
	if err != nil {
		panic(err)
	}
	if len(urlContent) > 0 {
		fmt.Printf("A DataRobot URL of %s is already present, do you want to overwrite? (y/N): ", urlContent)
		selectedOption, err := reader.ReadString('\n')
		if err != nil {
			panic(err)
		}

		if strings.ToLower(strings.Replace(selectedOption, "\n", "", -1)) == "y" {
			if err := clearUrlFile(); err != nil {
				panic(err)
			}
		} else {
			fmt.Println("Exiting without overwriting the DataRobot URL.")
			return
		}
	}
	fmt.Println("Please specify your DataRobot URL, or enter the numbers 1 - 3 If you are using that multi tenant cloud offering")
	fmt.Println("Please enter 1 if you're using https://app.datarobot.com")
	fmt.Println("Please enter 2 if you're using https://app.eu.datarobot.com")
	fmt.Println("Please enter 3 if you're using https://app.jp.datarobot.com")
	fmt.Println("Otherwise, please enter the URl you use")
	selectedOption, err := reader.ReadString('\n')
	if err != nil {
		panic(err)
	}
	selected := strings.ToLower(strings.Replace(selectedOption, "\n", "", -1))

	var url string
	if selected == "1" {
		url = "https://app.datarobot.com"
	} else if selected == "2" {
		url = "https://app.eu.datarobot.com"
	} else if selected == "3" {
		url = "https://app.jp.datarobot.com"
	} else {
		url, err = getBaseURL(selected)
		if err != nil {
			panic(err)
		}
	}
	createOrUpdateUrl(url)
}

var setUrlCmd = &cobra.Command{
	Use:   "setURL",
	Short: "Set URL for Loging to DataRobot",
	Long:  `Set URL for DataRobot to get and store that URL which can be used for other operations in the cli.`,
	Run: func(_ *cobra.Command, _ []string) {
		SetURLAction() // TODO: handler errors properly
	},
}
