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
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	DATAROBOT_URL = "endpoint"
)

func getBaseURL() (string, error) {
	urlContent, err := readValueFromConfigFile(DATAROBOT_URL)
	if err != nil {
		return "", err
	}
	if urlContent == "" {
		return "", nil
	}

	baseURL, err := loadBaseURLFromURL(urlContent)
	if err != nil {
		return "", err
	}

	return baseURL, nil
}

func loadBaseURLFromURL(longURL string) (string, error) {
	// Takes a URL like: https://app.datarobot.com/api/v2 and just
	// returns https://app.datarobot.com (no trailing slash)
	parsedURL, err := url.Parse(longURL)
	if err != nil {
		return "", err
	}

	base := fmt.Sprintf("%s://%s", parsedURL.Scheme, parsedURL.Host)

	return base, nil
}

func saveUrlToConfig(newURL string) error {
	// Saves the URL to the config file with the path prefix
	// Or as an empty string, if that's needed
	if newURL == "" {
		err := setValueInConfigFile(DATAROBOT_API_KEY, "")
		if err != nil {
			return err
		}
	}

	baseURL, err := loadBaseURLFromURL(newURL)
	if err != nil {
		return err
	}

	err = setValueInConfigFile(DATAROBOT_URL, baseURL+"/api/v2")
	if err != nil {
		return err
	}
	return nil
}

func GetURL(promptIfFound bool) (string, error) { //nolint: cyclop
	// This is the entrypoint for using a URL. The flow is:
	// * Check if there's a file with the content.  If there's no file, make it.
	// * If the file exists, and has content return it **UNLESS** the promptIfFound bool
	//   is supplied. This promptIfFound should really only be called if we're doing the setURL flow.
	// * If there's no file, then prompt the user for a URL, save it to the file, and return the URL to the caller func
	err := createConfigFileDirIfNotExists()
	if err != nil {
		return "", err
	}

	reader := bufio.NewReader(os.Stdin)

	urlContent, err := getBaseURL()
	if err != nil {
		return "", err
	}
	emptyURLContent := len(urlContent) > 0

	if emptyURLContent && !promptIfFound {
		return urlContent, nil
	}

	if emptyURLContent && promptIfFound { //nolint: nestif

		fmt.Printf("A DataRobot URL of %s is already present, do you want to overwrite? (y/N): ", urlContent)

		selectedOption, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}

		if strings.ToLower(strings.Replace(selectedOption, "\n", "", -1)) == "y" {
			if err := setValueInConfigFile(DATAROBOT_URL, ""); err != nil {
				return "", err
			}
		} else {
			fmt.Println("Exiting without overwriting the DataRobot URL.")
			return urlContent, nil
		}
	}

	fmt.Println("Please specify your DataRobot URL, or enter the numbers 1 - 3 If you are using that multi tenant cloud offering")
	fmt.Println("Please enter 1 if you're using https://app.datarobot.com")
	fmt.Println("Please enter 2 if you're using https://app.eu.datarobot.com")
	fmt.Println("Please enter 3 if you're using https://app.jp.datarobot.com")
	fmt.Println("Otherwise, please enter the URL you use")

	selectedOption, err := reader.ReadString('\n')
	if err != nil {
		return "", nil
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
		url, err = loadBaseURLFromURL(selected)
		if err != nil {
			return "", nil
		}
	}

	errors := saveUrlToConfig(url)
	if errors != nil {
		return url, errors
	}

	return url, nil
}

func SetURLAction() {
	_, err := GetURL(true)
	if err != nil {
		panic(err)
	}
}

var setURLCmd = &cobra.Command{
	Use:   "setURL",
	Short: "Set URL for Loging to DataRobot",
	Long:  `Set URL for DataRobot to get and store that URL which can be used for other operations in the cli.`,
	Run: func(_ *cobra.Command, _ []string) {
		SetURLAction() // TODO: handler errors properly
	},
}
