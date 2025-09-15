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
	prompts, roots, err := GatherUserPrompts(suite.tempDir)
	suite.NoError(err) //nolint: testifylint

	suite.Len(prompts, 6, "Expected to find 6 UserPrompt entries")

	suite.Equal("INFRA_ENABLE_LLM", prompts[0].Env, "Expected [0] prompt env to match")
	suite.Equal("TEXTGEN_DEPLOYMENT_ID", prompts[1].Env, "Expected [1] prompt env to match")
	suite.Equal("TEXTGEN_REGISTERED_MODEL_ID", prompts[2].Env, "Expected [2] prompt env to match")
	suite.Equal("DATAROBOT_TIMEOUT_MINUTES", prompts[3].Env, "Expected [3] prompt env to match")
	suite.Equal("PULUMI_CONFIG_PASSPHRASE", prompts[4].Env, "Expected [4] prompt env to match")
	suite.Equal("DATAROBOT_DEFAULT_USE_CASE", prompts[5].Env, "Expected [5] prompt env to match")

	suite.Len(roots, 2, "Expected to find 2 root entries")

	suite.Contains(roots[0], ".datarobot/another_parakeet.yaml:root")
	suite.Contains(roots[1], ".datarobot/parakeet.yaml:root")
}
