// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package envbuilder

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
)

func TestBuilderTestSuite(t *testing.T) {
	suite.Run(t, new(BuilderTestSuite))
}

type BuilderTestSuite struct {
	suite.Suite
	tempDir string
}

func (suite *BuilderTestSuite) SetupTest() {
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

func (suite *BuilderTestSuite) TestBuilderGeneratesInterfaces() {
	prompts, err := GatherUserPrompts(suite.tempDir, nil)
	suite.Require().NoError(err)

	suite.Len(prompts, 6, "Expected to find 6 UserPrompt entries")

	suite.Equal("INFRA_ENABLE_LLM", prompts[0].Env, "Expected [0] prompt env to match")
	suite.True(prompts[0].Active, "Expected [0] prompt active to match")

	suite.Equal("TEXTGEN_DEPLOYMENT_ID", prompts[1].Env, "Expected [1] prompt env to match")
	suite.False(prompts[1].Active, "Expected [1] prompt active to match")

	suite.Equal("TEXTGEN_REGISTERED_MODEL_ID", prompts[2].Env, "Expected [2] prompt env to match")
	suite.False(prompts[2].Active, "Expected [2] prompt active to match")

	suite.Equal("DATAROBOT_TIMEOUT_MINUTES", prompts[3].Env, "Expected [3] prompt env to match")
	suite.False(prompts[3].Active, "Expected [3] prompt active to match")

	suite.Equal("PULUMI_CONFIG_PASSPHRASE", prompts[4].Env, "Expected [4] prompt env to match")
	suite.True(prompts[4].Active, "Expected [4] prompt active to match")

	suite.Equal("DATAROBOT_DEFAULT_USE_CASE", prompts[5].Env, "Expected [5] prompt env to match")
	suite.True(prompts[5].Active, "Expected [5] prompt active to match")
}

func (suite *BuilderTestSuite) TestUserPromptTypeDeserialization() {
	yamlContent := `
root:
  - key: test-string
    env: TEST_STRING
    type: string
    help: A string type
  - key: test-secret
    env: TEST_SECRET
    type: secret_string
    help: A secret string type
  - key: test-boolean
    env: TEST_BOOLEAN
    type: boolean
    help: A boolean type
  - key: test-unknown
    env: TEST_UNKNOWN
    type: some_unknown_type
    help: An unknown type
`

	// Create a temporary YAML file
	tmpFile := filepath.Join(suite.tempDir, ".datarobot", "test_types.yaml")
	err := os.WriteFile(tmpFile, []byte(yamlContent), 0o600)
	suite.Require().NoError(err)

	// Parse the file
	prompts, err := filePrompts(tmpFile)
	suite.Require().NoError(err)
	suite.Require().Len(prompts, 4, "Expected 4 prompts")

	// Verify that Type field is preserved exactly as specified in YAML
	suite.Equal(PromptTypeString, prompts[0].Type, "Known types work")
	suite.Equal(PromptTypeSecret, prompts[1].Type, "Known types work")
	suite.NotEqual(PromptTypeSecret, prompts[0].Type, "Not equal works")
	suite.NotEqual(PromptTypeSecret, prompts[2].Type, "Not equal works")
	suite.Equal(PromptType("string"), prompts[0].Type, "String type should be preserved")
	suite.Equal(PromptType("secret_string"), prompts[1].Type, "Secret string type should be preserved")
	suite.Equal(PromptType("boolean"), prompts[2].Type, "Boolean type should be preserved")
	suite.Equal(PromptType("some_unknown_type"), prompts[3].Type, "Unknown type should be preserved")
}
