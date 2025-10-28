// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
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
	suite.T().Setenv("HOME", suite.tempDir)
}

func (suite *ConfigTestSuite) TestCreateConfigFileDirIfNotExists() {
	err := CreateConfigFileDirIfNotExists()
	suite.NoError(err) //nolint: testifylint

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
	expectedFilePath := filepath.Join(suite.tempDir, ".config/datarobot/drconfig.yaml")
	yamlData := map[string]string{
		"host":  "https://parakeet.jones.datarobot.com/api/v2",
		"token": "squak-squak",
	}
	rawYamlData, _ := yaml.Marshal(&yamlData)
	_ = os.WriteFile(expectedFilePath, rawYamlData, 0o644)

	readYamlData := make(map[string]string)

	err := ReadConfigFile("")
	suite.Require().NoError(err, "Expected no error when reading config file without a previous file")

	host := viper.GetString("host")
	suite.Equal(host, readYamlData["host"], "Expected config file to have the same host")

	token := viper.GetString("token")
	suite.Equal(token, readYamlData["token"], "Expected config file to have the same token")
}
