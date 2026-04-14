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
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"
)

func TestAPITestSuite(t *testing.T) {
	suite.Run(t, new(APITestSuite))
}

type APITestSuite struct {
	suite.Suite
	tempDir string
}

func (suite *APITestSuite) SetupTest() {
	dir, _ := os.MkdirTemp("", "datarobot-api-test")
	suite.tempDir = dir
	suite.T().Setenv("HOME", suite.tempDir)
	suite.T().Setenv("XDG_CONFIG_HOME", "")
	viper.Reset()
}

func (suite *APITestSuite) TestSetURLToConfig() {
	tests := []struct {
		name        string
		input       string
		expectedURL string
		expectError bool
	}{
		{
			name:        "full URL with API suffix",
			input:       "https://app.datarobot.com/api/v2",
			expectedURL: "https://app.datarobot.com/api/v2",
		},
		{
			name:        "base URL without suffix",
			input:       "https://app.datarobot.com",
			expectedURL: "https://app.datarobot.com/api/v2",
		},
		{
			name:        "shortcut 1 resolves to app.datarobot.com",
			input:       "1",
			expectedURL: "https://app.datarobot.com/api/v2",
		},
		{
			name:        "shortcut 2 resolves to app.eu.datarobot.com",
			input:       "2",
			expectedURL: "https://app.eu.datarobot.com/api/v2",
		},
		{
			name:        "shortcut 3 resolves to app.jp.datarobot.com",
			input:       "3",
			expectedURL: "https://app.jp.datarobot.com/api/v2",
		},
		{
			name:        "URL with trailing path gets trimmed to host",
			input:       "https://custom.example.com/some/path",
			expectedURL: "https://custom.example.com/api/v2",
		},
		{
			name:        "invalid URL without host returns error",
			input:       "not-a-url",
			expectError: true,
		},
	}

	for _, tc := range tests {
		suite.Run(tc.name, func() {
			viper.Reset()

			err := SetURLToConfig(tc.input)

			if tc.expectError {
				suite.Require().Error(err)
			} else {
				suite.Require().NoError(err)
				suite.Equal(tc.expectedURL, viper.GetString(DataRobotURL))
			}
		})
	}
}

func (suite *APITestSuite) TestSetURLToConfigDoesNotWriteFile() {
	err := SetURLToConfig("https://app.datarobot.com")
	suite.Require().NoError(err)

	configFile := filepath.Join(suite.tempDir, ".config/datarobot/drconfig.yaml")
	suite.NoFileExists(configFile, "SetURLToConfig must not write the config file to disk")
}
