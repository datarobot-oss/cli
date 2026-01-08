// Copyright 2025 DataRobot, Inc. and its affiliates.
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

package envbuilder

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
)

var testYamlFile1 = `
root:
  - env: PULUMI_CONFIG_PASSPHRASE
    type: string
    default: "123"
    optional: true
    help: "The passphrase used to encrypt and decrypt the private key. This value is required if you're not using pulumi cloud."
  - env: DATAROBOT_DEFAULT_USE_CASE
    type: string
    default:
    optional: true
    help: "The default use case for this application. If not set, a new use case will be created automatically"
`

var testYamlFile2 = `
root:
  - env: INFRA_ENABLE_LLM
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
        requires: deployed_llm
      - name: "Registered Model with an LLM Blueprint"
        value: "registered_model.py"
        requires: registered_model
  - env: DUPLICATE
    type: string
    optional: true
    help: "Duplicate root."

deployed_llm:
  - env: TEXTGEN_DEPLOYMENT_ID
    type: string
    optional: false
    help: "The deployment ID of the DataRobot Deployed LLM to use."
  - env: DUPLICATE
    type: string
    optional: true
    help: "Duplicate deployed_llm."

registered_model:
  - env: TEXTGEN_REGISTERED_MODEL_ID
    type: string
    optional: false
    help: "The ID of the registered model with an LLM blueprint to use."
  - env: DATAROBOT_TIMEOUT_MINUTES
    type: number
    default: "30"
    optional: true
    help: "The timeout in minutes for DataRobot operations. Default is 30 minutes."
  - env: DUPLICATE
    type: string
    optional: true
    help: "Duplicate registered_model."
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

	file1, err := os.OpenFile(filepath.Join(datarobotDir, "parakeet.yaml"), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		suite.T().Errorf("Failed to create test YAML file one: %v", err)
	}

	defer file1.Close()

	_, err = file1.WriteString(testYamlFile1)
	if err != nil {
		suite.T().Errorf("Failed to write to test YAML file one: %v", err)
	}

	file2, err := os.OpenFile(filepath.Join(datarobotDir, "another_parakeet.yaml"), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
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
	suite.Require().NoError(err)

	suite.Len(foundPaths, 2, "Expected to find 2 YAML files")
	suite.Contains(foundPaths, suite.tempDir+"/.datarobot/parakeet.yaml")
	suite.Contains(foundPaths, suite.tempDir+"/.datarobot/another_parakeet.yaml")
}

func (suite *DiscoverTestSuite) TestDiscoverFindsNestedFiles() {
	parakeetDir := filepath.Join(suite.tempDir, ".datarobot", "parakeet")

	err := os.MkdirAll(parakeetDir, os.ModePerm)
	if err != nil {
		suite.T().Errorf("Failed to create nested .datarobot directory: %v", err)
	}

	file3, err := os.OpenFile(filepath.Join(parakeetDir, "yet_another_parakeet.yml"), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		suite.T().Errorf("Failed to create test YAML file three: %v", err)
	}

	defer file3.Close()

	_, err = file3.WriteString(testYamlFile1)
	if err != nil {
		suite.T().Errorf("Failed to write to test YAML file three: %v", err)
	}

	defer os.RemoveAll(parakeetDir)

	foundPaths, err := Discover(suite.tempDir, 5)
	suite.Require().NoError(err)

	suite.Len(foundPaths, 3, "Expected to find 3 YAML files")
	suite.Contains(foundPaths, suite.tempDir+"/.datarobot/parakeet.yaml")
	suite.Contains(foundPaths, suite.tempDir+"/.datarobot/another_parakeet.yaml")
	suite.Contains(foundPaths, suite.tempDir+"/.datarobot/parakeet/yet_another_parakeet.yml")
}
