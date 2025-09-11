// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package config

import (
	"errors"
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
		return "", errors.New("invalid url")
	}

	parsedURL.Path, parsedURL.RawQuery, parsedURL.Fragment = "", "", ""

	return parsedURL.String(), nil
}

func GetBaseURL() string {
	if endpoint := os.Getenv("DATAROBOT_ENDPOINT"); endpoint != "" {
		if newURL, err := schemeHostOnly(endpoint); err == nil {
			return newURL
		}
	}

	if url, err := schemeHostOnly(viper.GetString(DataRobotURL)); err == nil {
		return url
	}

	return ""
}

func GetEndpointURL(endpoint string) (string, error) {
	baseURL := GetBaseURL()
	if baseURL == "" {
		return "", errors.New("empty url")
	}

	return url.JoinPath(baseURL, endpoint)
}

func SaveURLToConfig(newURL string) error {
	newURL, err := schemeHostOnly(urlFromShortcut(newURL))
	if err != nil {
		return err
	}

	if err := CreateConfigFileDirIfNotExists(); err != nil {
		return err
	}

	// Saves the URL to the config file with the path prefix
	// Or as an empty string, if that's needed
	if newURL == "" {
		viper.Set(DataRobotURL, "")
		viper.Set(DataRobotAPIKey, "")
		_ = viper.WriteConfig()

		return nil
	}

	viper.Set(DataRobotURL, newURL+"/api/v2")
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
		return selected
	}
}

func GetAPIKey() string {
	if token := os.Getenv("DATAROBOT_API_TOKEN"); token != "" {
		return token
	}

	// Returns the API key if there is one, otherwise returns an empty string
	return viper.GetString(DataRobotAPIKey)
}
