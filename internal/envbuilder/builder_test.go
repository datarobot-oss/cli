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
	envBuilder := NewEnvBuilder()
	prompts, err := envBuilder.GatherUserPrompts(suite.tempDir)
	suite.NoError(err) //nolint: testifylint

	suite.Equal(5, len(prompts), "Expected to find four sets of prompts")
	userPromptCount := 0
	usePromptCollectionCount := 0
	for _, prompt := range prompts {
		switch prompt.(type) {
		case UserPrompt:
			userPromptCount++
		case UserPromptCollection:
			usePromptCollectionCount++
		}
	}
	suite.Equal(4, userPromptCount, "Expected to find found UserPrompt entries")
	suite.Equal(1, usePromptCollectionCount, "Expected to find one UserPromptCollection entries")
	firstPrompt := prompts[0].(UserPrompt)
	suite.IsType(UserPrompt{}, firstPrompt, "Expected first prompt to be of type UserPrompt")
	suite.Equal(firstPrompt.Key, "infra_enable_llm", "Expected first prompt key to match")
	suite.Equal(firstPrompt.Env, "INFRA_ENABLE_LLM", "Expected first prompt env to match")
}
