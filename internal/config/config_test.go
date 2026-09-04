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

	"github.com/datarobot/cli/internal/config/viperx"
	"github.com/datarobot/cli/internal/testutil"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v3"
)

func TestConfigTestSuite(t *testing.T) {
	suite.Run(t, new(ConfigTestSuite))
}

type ConfigTestSuite struct {
	suite.Suite
	tempDir string
}

func (suite *ConfigTestSuite) SetupTest() {
	dir, _ := os.MkdirTemp("", "datarobot-config-test")
	suite.tempDir = dir
	// Sets HOME + USERPROFILE (Windows) and clears XDG_CONFIG_HOME so the
	// config dir resolves under the temp dir on every platform.
	testutil.SetTestHomeDir(suite.T(), suite.tempDir)

	// viper is process-global, so without this the suite's results depend on
	// what an earlier test in the -shuffle order happened to leave set.
	viperx.Reset()
	suite.T().Cleanup(viperx.Reset)
}

func (suite *ConfigTestSuite) TestCreateConfigFileDirIfNotExists() {
	err := CreateConfigFileDirIfNotExists()
	suite.Require().NoError(err)

	expectedDir := filepath.Join(suite.tempDir, ".config/datarobot")

	// Check if the directory was created
	suite.DirExists(expectedDir, "Expected config directory to be created")

	// Check if the file was created
	expectedFileName := "/drconfig.yaml"
	suite.FileExists(filepath.Join(expectedDir, expectedFileName), "Expected config file to be created")
}

func (suite *ConfigTestSuite) TestReadConfigFileNoPreviousFile() {
	err := ReadConfigFile("")
	suite.Require().NoError(err, "Expected no error when reading config file without a previous file")

	expectedFilePath := filepath.Join(suite.tempDir, ".config/datarobot/drconfig.yaml")
	suite.NoFileExists(expectedFilePath, "Expected config file to not be created at default path")
}

func (suite *ConfigTestSuite) TestReadConfigFileWithPreviousFile() {
	suite.Require().NoError(CreateConfigFileDirIfNotExists())

	yamlData := map[string]string{
		"host":  "https://parakeet.jones.datarobot.com/api/v2",
		"token": "squak-squak",
	}

	rawYamlData, err := yaml.Marshal(&yamlData)
	suite.Require().NoError(err)

	expectedFilePath := filepath.Join(suite.tempDir, ".config/datarobot/drconfig.yaml")
	suite.Require().NoError(os.WriteFile(expectedFilePath, rawYamlData, 0o600))

	suite.Require().NoError(ReadConfigFile(""), "Expected no error when reading an existing config file")

	suite.Equal(yamlData["host"], viper.GetString("host"), "Expected config file to have the same host")
	suite.Equal(yamlData["token"], viper.GetString("token"), "Expected config file to have the same token")
}

func (suite *ConfigTestSuite) TestCreateConfigFileDirWithXDGConfigHome() {
	xdgConfigDir := filepath.Join(suite.tempDir, "custom-config")
	testutil.SetXDGEnv(suite.T(), "XDG_CONFIG_HOME", xdgConfigDir)

	err := CreateConfigFileDirIfNotExists()
	suite.Require().NoError(err)

	expectedDir := filepath.Join(xdgConfigDir, "datarobot")

	// Check if the directory was created
	suite.DirExists(expectedDir, "Expected config directory to be created in XDG_CONFIG_HOME")

	// Check if the file was created
	expectedFileName := "/drconfig.yaml"
	suite.FileExists(filepath.Join(expectedDir, expectedFileName), "Expected config file to be created in XDG_CONFIG_HOME")
}

// writeConfigFile writes yamlContent to the suite's default drconfig.yaml
// path and returns that path.
func (suite *ConfigTestSuite) writeConfigFile(yamlContent string) string {
	suite.Require().NoError(CreateConfigFileDirIfNotExists())

	configFile := filepath.Join(suite.tempDir, ".config", "datarobot", "drconfig.yaml")
	suite.Require().NoError(os.WriteFile(configFile, []byte(yamlContent), 0o600))

	return configFile
}

func (suite *ConfigTestSuite) TestReadConfigFile_FlatConfigNoProfilesBlock_BackwardCompat() {
	suite.writeConfigFile(`endpoint: https://app.datarobot.com/api/v2
token: flat-token
`)

	suite.Require().NoError(ReadConfigFile(""))

	suite.Equal("https://app.datarobot.com/api/v2", viper.GetString(DataRobotURL))
	suite.Equal("flat-token", viper.GetString(DataRobotAPIKey))
	suite.Empty(ActiveProfile())
}

func (suite *ConfigTestSuite) TestReadConfigFile_ProfilesBlockPresentButNoneSelected() {
	suite.writeConfigFile(`endpoint: https://app.datarobot.com/api/v2
token: default-token
profiles:
  eu-mtsaas:
    endpoint: https://app.eu.datarobot.com/api/v2
    token: eu-token
`)

	suite.Require().NoError(ReadConfigFile(""))

	suite.Equal("https://app.datarobot.com/api/v2", viper.GetString(DataRobotURL))
	suite.Equal("default-token", viper.GetString(DataRobotAPIKey))
}

func (suite *ConfigTestSuite) TestReadConfigFile_ActiveProfileShadowsEndpointAndToken() {
	suite.writeConfigFile(`endpoint: https://app.datarobot.com/api/v2
token: default-token
profiles:
  eu-mtsaas:
    endpoint: https://app.eu.datarobot.com/api/v2
    token: eu-token
`)

	viper.Set(ProfileKey, "eu-mtsaas")

	suite.Require().NoError(ReadConfigFile(""))

	suite.Equal("https://app.eu.datarobot.com/api/v2", viper.GetString(DataRobotURL))
	suite.Equal("eu-token", viper.GetString(DataRobotAPIKey))
}

func (suite *ConfigTestSuite) TestReadConfigFile_ProfileOmittingCACertInheritsTopLevel() {
	suite.writeConfigFile(`endpoint: https://app.datarobot.com/api/v2
token: default-token
ca-cert: /etc/ssl/corp.pem
profiles:
  eu-mtsaas:
    endpoint: https://app.eu.datarobot.com/api/v2
    token: eu-token
`)

	viper.Set(ProfileKey, "eu-mtsaas")

	suite.Require().NoError(ReadConfigFile(""))

	suite.Equal("/etc/ssl/corp.pem", viper.GetString("ca-cert"), "ca-cert should be inherited from the top level")
}

func (suite *ConfigTestSuite) TestReadConfigFile_EnvVarPairStillWinsOverActiveProfile() {
	suite.writeConfigFile(`endpoint: https://app.datarobot.com/api/v2
token: default-token
profiles:
  eu-mtsaas:
    endpoint: https://app.eu.datarobot.com/api/v2
    token: eu-token
`)

	viper.Set(ProfileKey, "eu-mtsaas")

	// DATAROBOT_CLI_ENDPOINT / DATAROBOT_CLI_TOKEN are automatically mapped
	// by AutomaticEnv + SetEnvPrefix once bindViperFlags-equivalent setup has
	// run; reproduce that here since this test reads ReadConfigFile in
	// isolation from cmd/root_factory.go.
	viper.SetEnvPrefix("DATAROBOT_CLI")
	viper.AutomaticEnv()
	suite.T().Setenv("DATAROBOT_CLI_ENDPOINT", "https://override.example.com/api/v2")

	suite.Require().NoError(ReadConfigFile(""))

	suite.Equal("https://override.example.com/api/v2", viper.GetString(DataRobotURL),
		"an explicit env var must still beat the active profile's merged value")
}

func (suite *ConfigTestSuite) TestReadConfigFile_ProfileNameIsCaseInsensitive() {
	suite.writeConfigFile(`endpoint: https://app.datarobot.com/api/v2
token: default-token
profiles:
  EU-MTSaaS:
    endpoint: https://app.eu.datarobot.com/api/v2
    token: eu-token
`)

	viper.Set(ProfileKey, "eu-mtsaas")

	suite.Require().NoError(ReadConfigFile(""))

	suite.Equal("https://app.eu.datarobot.com/api/v2", viper.GetString(DataRobotURL))
}

func (suite *ConfigTestSuite) TestReadConfigFile_UnknownProfileReturnsTypedError() {
	suite.writeConfigFile(`endpoint: https://app.datarobot.com/api/v2
token: default-token
profiles:
  staging:
    endpoint: https://staging.example.com/api/v2
    token: staging-token
`)

	viper.Set(ProfileKey, "eu-mtsaas")

	err := ReadConfigFile("")
	suite.Require().Error(err)

	var unknown *UnknownProfileError

	suite.Require().ErrorAs(err, &unknown)
	suite.Equal("eu-mtsaas", unknown.Name)
	suite.Equal([]string{"staging"}, unknown.Known)
}

func TestDebugViperConfig_RedactsNestedProfileToken(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.Set("profiles", map[string]any{
		"eu-mtsaas": map[string]any{
			"endpoint": "https://app.eu.datarobot.com/api/v2",
			"token":    "SUPER_SECRET_EU_TOKEN",
		},
	})

	output, err := DebugViperConfig()
	require.NoError(t, err)
	assert.Contains(t, output, "****")
	assert.NotContains(t, output, "SUPER_SECRET_EU_TOKEN")
}

func TestDebugViperConfig_RedactsPulumiConfigPassphrase(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	const secret = "SUPER_SECRET_PASSPHRASE"

	viper.Set("pulumi_config_passphrase", secret)

	output, err := DebugViperConfig()
	require.NoError(t, err)
	assert.Contains(t, output, "pulumi_config_passphrase")
	assert.Contains(t, output, "****")
	assert.NotContains(t, output, secret)
}

func TestDebugViperConfig_RedactsToken(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	const secret = "SUPER_SECRET_TOKEN"

	viper.Set("token", secret)

	output, err := DebugViperConfig()
	require.NoError(t, err)
	assert.Contains(t, output, "token")
	assert.Contains(t, output, "****")
	assert.NotContains(t, output, secret)
}

func TestDebugViperConfig_DoesNotRedactNonSensitiveKey(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.Set("debug", true)

	output, err := DebugViperConfig()
	require.NoError(t, err)
	assert.Contains(t, output, "debug: true")
}
