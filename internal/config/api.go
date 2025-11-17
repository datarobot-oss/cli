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
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/datarobot/cli/internal/version"
	"github.com/spf13/viper"
)

var ErrInvalidURL = errors.New("Invalid URL.")

// SchemeHostOnly takes a URL like: https://app.datarobot.com/api/v2 and just
// returns https://app.datarobot.com (no trailing slash)
func SchemeHostOnly(longURL string) (string, error) {
	parsedURL, err := url.Parse(longURL)
	if err != nil {
		return "", err
	}

	if parsedURL.Host == "" {
		return "", ErrInvalidURL
	}

	parsedURL.Path, parsedURL.RawQuery, parsedURL.Fragment = "", "", ""

	return parsedURL.String(), nil
}

func GetBaseURL() string {
	if endpoint := viper.GetString(DataRobotURL); endpoint != "" {
		if newURL, err := SchemeHostOnly(endpoint); err == nil {
			return newURL
		}
	}

	return ""
}

func GetEndpointURL(endpoint string) (string, error) {
	baseURL := GetBaseURL()
	if baseURL == "" {
		return "", errors.New("Empty URL.")
	}

	return url.JoinPath(baseURL, endpoint)
}

func GetUserAgentHeader() string {
	return version.GetAppNameVersionText()
}

func RedactedReqInfo(req *http.Request) string {
	// Dump the request to a byte slice after cloning and removing Auth header
	dumpReq := req.Clone(req.Context())
	if auth := dumpReq.Header.Get("Authorization"); auth != "" {
		dumpReq.Header.Set("Authorization", "[REDACTED]")
	}

	requestDump, err := httputil.DumpRequestOut(dumpReq, true)
	if err != nil {
		return ""
	}

	return string(requestDump)
}

func SaveURLToConfig(newURL string) error {
	if err := CreateConfigFileDirIfNotExists(); err != nil {
		return err
	}

	// Create a new viper instance to avoid affecting global state
	v := viper.New()
	v.SetConfigType("yaml")

	defaultConfigFileDir := filepath.Join(os.Getenv("HOME"), ".config", "datarobot")
	defaultConfigFilePath := filepath.Join(defaultConfigFileDir, "drconfig.yaml")

	v.SetConfigFile(defaultConfigFilePath)

	// Read existing config to preserve all fields
	if err := v.ReadInConfig(); err != nil {
		// Ignore error if config file not found, as we'll create it
		if !errors.As(err, &viper.ConfigFileNotFoundError{}) {
			return err
		}
	}

	// Saves the URL to the config file with the path prefix
	// Or as an empty string, if that's needed
	expandedURL := urlFromShortcut(newURL)
	if expandedURL == "" {
		v.Set(DataRobotURL, "")
		v.Set(DataRobotAPIKey, "")
	} else {
		processedURL, err := SchemeHostOnly(expandedURL)
		if err != nil {
			return err
		}

		v.Set(DataRobotURL, processedURL+"/api/v2")
	}

	if err := v.WriteConfig(); err != nil {
		return err
	}

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
