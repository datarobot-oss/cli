// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package envbuilder

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
)

var testYamlFile1 = `
pulumi_config_passphrase:
  env: PULUMI_CONFIG_PASSPHRASE
  type: string
  default: "123"
  optional: true
  help: "The passphrase used to encrypt and decrypt the private key. This value is required if you're not using pulumi cloud."
datarobot_default_use_case:
  env: DATAROBOT_DEFAULT_USE_CASE
  type: string
  default:
  optional: true
  help: "The default use case for this application. If not set, a new use case will be created automatically"
`

var testYamlFile2 = `
infra_enable_llm:
  env: INFRA_ENABLE_LLM
  type: string
  optional: true
  help: "Select the type of LLM integration to enable."
  options:
    - name: "External LLM"
      value: "blueprint_with_external_llm.py"
    - name: "LLM Gateway"
      value: "blueprint_with_llm_gateway.py"
    - name: "DataRobot Deployed LLM"
      value: "deployed_llm.py"
    - name: "Registered Model with an LLM Blueprint"
      value: "registered_model.py"
deployed_llm:
  requires:
    - name: infra_enable_llm
      value: "deployed_llm.py"
  env: TEXTGEN_DEPLOYMENT_ID
  type: string
  optional: false
  help: "The deployment ID of the DataRobot Deployed LLM to use."
registered_model:
  requires:
    - name: infra_enable_llm
      value: "registered_model.py"
  prompts:
    - env: TEXTGEN_REGISTERED_MODEL_ID
      type: string
      optional: false
      help: "The ID of the registered model with an LLM blueprint to use."
    - env: DATAROBOT_TIMEOUT_MINUTES
      type: number
      default: "30"
      optional: true
      help: "The timeout in minutes for DataRobot operations. Default is 30 minutes."
`

func TestDiscoverTestSuite(t *testing.T) {
	suite.Run(t, new(DiscoverTestSuite))
}

type DiscoverTestSuite struct {
	suite.Suite
	tempDir string
}

func (suite *DiscoverTestSuite) SetupTest() {
	dir, _ := os.MkdirTemp("", "a_template_repo")
	datarobotDir := filepath.Join(dir, ".datarobot")
	err := os.MkdirAll(datarobotDir, os.ModePerm)
	if err != nil {
		suite.T().Errorf("Failed to create .datarobot directory: %v", err)
	}

	file1, err := os.OpenFile(filepath.Join(datarobotDir, "parakeet.yaml"), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		suite.T().Errorf("Failed to create test YAML file one: %v", err)
	}

	defer file1.Close()

	_, err = file1.WriteString(testYamlFile1)
	if err != nil {
		suite.T().Errorf("Failed to write to test YAML file one: %v", err)
	}

	file2, err := os.OpenFile(filepath.Join(datarobotDir, "another_parakeet.yaml"), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		suite.T().Errorf("Failed to create test YAML file two: %v", err)
	}

	defer file2.Close()

	_, err = file2.WriteString(testYamlFile2)
	if err != nil {
		suite.T().Errorf("Failed to write to test YAML file two: %v", err)
	}

	suite.tempDir = dir
}

func (suite *DiscoverTestSuite) TestDiscoverFindsFiles() {
	foundPaths, err := Discover(suite.tempDir, 5)
	suite.NoError(err) //nolint: testifylint

	suite.Equal(2, len(foundPaths), "Expected to find 2 YAML files")
	suite.Contains(foundPaths, fmt.Sprintf("%s/.datarobot/parakeet.yaml", suite.tempDir))
	suite.Contains(foundPaths, fmt.Sprintf("%s/.datarobot/another_parakeet.yaml", suite.tempDir))
}

func (suite *DiscoverTestSuite) TestDiscoverFindsNestedFiles() {
	foundPaths, err := Discover(suite.tempDir, 5)
	suite.NoError(err) //nolint: testifylint

	suite.Equal(2, len(foundPaths), "Expected to find 2 YAML files")
	suite.Contains(foundPaths, fmt.Sprintf("%s/.datarobot/parakeet.yaml", suite.tempDir))
	suite.Contains(foundPaths, fmt.Sprintf("%s/.datarobot/another_parakeet.yaml", suite.tempDir))
}
