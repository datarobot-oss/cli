// Copyright 2026 DataRobot, Inc. and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	"errors"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/datarobot/cli/internal/version"
	"github.com/spf13/viper"
)

const DRAPIURLSuffix = "/api/v2"

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

	return baseURL + endpoint, nil
}

func GetUserAgentHeader() string {
	return version.GetAppNameVersionText()
}

// apiConsumerTrace holds the dot-notation command path set at startup.
// Example: "datarobot.cli.templates.setup"
var apiConsumerTrace string

// SetAPIConsumerTrace stores the dot-notation trace value for the running command.
// Called once during PersistentPreRunE with the result of CommandPathToTrace.
func SetAPIConsumerTrace(trace string) {
	apiConsumerTrace = trace
}

// GetAPIConsumerTrace returns the dot-notation trace value for the running command.
// Falls back to "datarobot.cli" if SetAPIConsumerTrace has not been called.
func GetAPIConsumerTrace() string {
	if apiConsumerTrace == "" {
		return "datarobot.cli"
	}

	return apiConsumerTrace
}

// CommandPathToTrace converts a cobra command path (e.g. "dr templates setup")
// to the canonical dot-notation trace format (e.g. "datarobot.cli.templates.setup").
func CommandPathToTrace(commandPath string) string {
	parts := strings.Fields(commandPath)
	if len(parts) == 0 {
		return "datarobot.cli"
	}

	parts[0] = "datarobot.cli"

	return strings.Join(parts, ".")
}

// IsAPIConsumerTrackingEnabled returns true if the X-DataRobot-Api-Consumer-Trace
// header should be sent with API requests. Controlled by the
// DATAROBOT_API_CONSUMER_TRACKING_ENABLED environment variable (default: true).
func IsAPIConsumerTrackingEnabled() bool {
	return viper.GetBool(APIConsumerTrackingEnabled)
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

// TODO: I believe we want to delete this function as there is SetURLToConfig function
// But it is used in cmd/templates/setup/model.go
func SaveURLToConfig(newURL string) error {
	newURL, err := SchemeHostOnly(urlFromShortcut(newURL))
	if err != nil {
		return err
	}

	if err = CreateConfigFileDirIfNotExists(); err != nil {
		return err
	}

	// Saves the URL to the config file with the path prefix
	// Or as an empty string, if that's needed
	if newURL == "" {
		viper.Set(DataRobotURL, "")
		viper.Set(DataRobotAPIKey, "")

		return UpdateConfigFile(DataRobotURL, DataRobotAPIKey)
	}

	viper.Set(DataRobotURL, newURL+DRAPIURLSuffix)

	return UpdateConfigFile(DataRobotURL)
}

// SetURLToConfig is a helper function that sets the DataRobot URL with the DRAPIURLSuffix in the config object.
// It is used by both cmd/auth/set-url and cmd/auth/login to ensure consistent URL formatting.
// It does NOT write to the config file, in order not to break drconfig.yaml file once URL is not valid or some issues with API key.
func SetURLToConfig(newURL string) error {
	newURL, err := SchemeHostOnly(urlFromShortcut(newURL))
	if err != nil {
		return err
	}

	viper.Set(DataRobotURL, newURL+DRAPIURLSuffix)

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
