// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package config

import (
	"net/url"
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

func GetBaseURL() string {
	urlContent := viper.GetString(DataRobotURL)

	if urlContent == "" {
		return ""
	}

	baseURL, err := schemeHostOnly(urlContent)
	if err != nil {
		return ""
	}

	return baseURL
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
	selected := strings.TrimSpace(selectedOption)

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

func GetAPIKey() string {
	// Returns the API key if there is one, otherwise returns an empty string
	return viper.GetString(DataRobotAPIKey)
}
