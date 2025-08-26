// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package config

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/viper"
)

func schemeHostOnly(longURL string) (string, error) {
	// Takes a URL like: https://app.datarobot.com/api/v2 and just
	// returns https://app.datarobot.com (no trailing slash)
	parsedURL, err := url.Parse(longURL)
	if err != nil {
		return "", err
	}

	if parsedURL.Host == "" {
		return "", err
	}

	parsedURL.Path, parsedURL.RawQuery, parsedURL.Fragment = "", "", ""

	return parsedURL.String(), nil
}

func GetBaseURL() (string, error) {
	urlContent := viper.GetString(DataRobotURL)

	if urlContent == "" {
		return "", nil
	}

	baseURL, err := schemeHostOnly(urlContent)
	if err != nil {
		return "", err
	}

	return baseURL, nil
}

func SaveURLToConfig(newURL string) error {
	newURL = urlFromShortcut(newURL)

	// Saves the URL to the config file with the path prefix
	// Or as an empty string, if that's needed
	if newURL == "" {
		viper.Set(DataRobotURL, "")
		viper.Set(DataRobotAPIKey, "")
		_ = viper.WriteConfig()

		return nil
	}

	datarobotURL, err := url.Parse(newURL)
	if err != nil {
		return err
	}

	datarobotURL.Path, datarobotURL.RawQuery, datarobotURL.Fragment = "/api/v2", "", ""

	viper.Set(DataRobotURL, datarobotURL.String())

	_ = viper.WriteConfig()

	return nil
}

func urlFromShortcut(selectedOption string) string {
	selected := strings.ToLower(strings.TrimSpace(selectedOption))

	switch selected {
	case "":
		return ""
	case "1":
		return "https://app.datarobot.com"
	case "2":
		return "https://app.eu.datarobot.com"
	case "3":
		return "https://app.jp.datarobot.com"
	default:
		url, err := schemeHostOnly(selected)
		if err != nil {
			return ""
		}

		return url
	}
}

func GetURL(promptIfFound bool) (string, error) {
	// This is the entrypoint for using a URL. The flow is:
	// * Check if there's a file with the content.  If there's no file, make it.
	// * If the file exists, and has content return it **UNLESS** the promptIfFound bool
	//   is supplied. This promptIfFound should really only be called if we're doing the setURL flow.
	// * If there's no file, then prompt the user for a URL, save it to the file, and return the URL to the caller func
	reader := bufio.NewReader(os.Stdin)

	urlContent, err := GetBaseURL()
	if err != nil {
		return "", err
	}

	presentURLContent := len(urlContent) > 0

	if presentURLContent && !promptIfFound {
		return urlContent, nil
	}

	if presentURLContent && promptIfFound {
		fmt.Printf("A DataRobot URL of %s is already present, do you want to overwrite? (y/N): ", urlContent)

		selectedOption, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}

		if strings.ToLower(strings.TrimSpace(selectedOption)) != "y" {
			fmt.Println("Exiting without overwriting the DataRobot URL.")
			return urlContent, nil
		}
	}

	fmt.Println("Please specify your DataRobot URL, or enter the numbers 1 - 3 If you are using that multi tenant cloud offering")
	fmt.Println("Please enter 1 if you're using https://app.datarobot.com")
	fmt.Println("Please enter 2 if you're using https://app.eu.datarobot.com")
	fmt.Println("Please enter 3 if you're using https://app.jp.datarobot.com")
	fmt.Println("Otherwise, please enter the URL you use")

	url, err := reader.ReadString('\n')
	if err != nil {
		return "", nil
	}

	errors := SaveURLToConfig(url)
	if errors != nil {
		return url, errors
	}

	return url, nil
}

func GetAPIKey() (string, error) {
	// Returns the API key if there is one, otherwise returns an empty string
	key := viper.GetString(DataRobotAPIKey)

	return key, nil
}
