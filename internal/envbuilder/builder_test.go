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

	suite.Len(prompts, 11, "Expected to find 11 UserPrompt entries")

	i := 0
	suite.Equal("DATAROBOT_ENDPOINT", prompts[i].Env, "Expected prompt[i].Env to match")
	suite.True(prompts[i].Active, "Expected prompt[i].Active to be true")
	suite.True(prompts[i].Hidden, "Expected prompt[i].Hidden to be true")

	i++
	suite.Equal("DATAROBOT_API_TOKEN", prompts[i].Env, "Expected prompt[i].Env to match")
	suite.True(prompts[i].Active, "Expected prompt[i].Active to be true")
	suite.True(prompts[i].Hidden, "Expected prompt[i].Hidden to be true")

	i++
	suite.Equal("INFRA_ENABLE_LLM", prompts[i].Env, "Expected prompt[i].Env to match")
	suite.True(prompts[i].Active, "Expected prompt[i].Active to be true")

	i++
	suite.Equal("TEXTGEN_DEPLOYMENT_ID", prompts[i].Env, "Expected prompt[i].Env to match")
	suite.False(prompts[i].Active, "Expected prompt[i].Active to be false")

	i++
	suite.Equal("DUPLICATE", prompts[i].Env, "Expected prompt[i].Env to match")
	suite.Equal("Duplicate deployed_llm.", prompts[i].Help, "Expected prompt[i].Env to match")
	suite.False(prompts[i].Active, "Expected prompt[i].Active to be false")

	i++
	suite.Equal("TEXTGEN_REGISTERED_MODEL_ID", prompts[i].Env, "Expected prompt[i].Env to match")
	suite.False(prompts[i].Active, "Expected prompt[i].Active to be false")

	i++
	suite.Equal("DATAROBOT_TIMEOUT_MINUTES", prompts[i].Env, "Expected prompt[i].Env to match")
	suite.False(prompts[i].Active, "Expected prompt[i].Active to be false")

	i++
	suite.Equal("DUPLICATE", prompts[i].Env, "Expected prompt[i].Env to match")
	suite.Equal("Duplicate registered_model.", prompts[i].Help, "Expected prompt[i].Env to match")
	suite.False(prompts[i].Active, "Expected prompt[i].Active to be false")

	i++
	suite.Equal("DUPLICATE", prompts[i].Env, "Expected prompt[i].Env to match")
	suite.Equal("Duplicate root.", prompts[i].Help, "Expected prompt[i].Env to match")
	suite.True(prompts[i].Active, "Expected prompt[i].Active to be true")

	i++
	suite.Equal("PULUMI_CONFIG_PASSPHRASE", prompts[i].Env, "Expected prompt[i].Env to match")
	suite.True(prompts[i].Active, "Expected prompt[i].Active to be true")

	i++
	suite.Equal("DATAROBOT_DEFAULT_USE_CASE", prompts[i].Env, "Expected prompt[i].Env to match")
	suite.True(prompts[i].Active, "Expected prompt[i].Active to be true")
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

func (suite *BuilderTestSuite) TestUserPromptMultilineHelpString() {
	yamlContent := `
root:
  - key: test-string
    env: TEST_STRING
    type: string
    help: |-
        A string type.
        With a multiline help string.
  - key: test-secret
    env: TEST_SECRET
    type: secret_string
    help: A secret string type
`

	// Create a temporary YAML file
	tmpFile := filepath.Join(suite.tempDir, ".datarobot", "test_multiline_help_string.yaml")
	err := os.WriteFile(tmpFile, []byte(yamlContent), 0o600)
	suite.Require().NoError(err)

	// Parse the file
	prompts, err := filePrompts(tmpFile)
	suite.Require().NoError(err)
	suite.Require().Len(prompts, 2, "Expected 2 prompts")

	// Verify that our multiline string has a newline in it
	suite.Equal("A string type.\nWith a multiline help string.", prompts[0].Help)
	suite.Equal("A secret string type", prompts[1].Help)
}

func (suite *BuilderTestSuite) TestAlwaysAskYAMLParsing() {
	yamlContent := `
root:
  - env: PORT
    type: string
    default: "8080"
    help: Application port
    always_ask: true
  - env: DEBUG
    type: string
    default: "false"
    help: Enable debug mode
  - env: LOG_LEVEL
    type: string
    default: "info"
    help: Log level
    always_ask: false
`

	// Create a temporary YAML file
	tmpFile := filepath.Join(suite.tempDir, ".datarobot", "test_always_ask.yaml")
	err := os.WriteFile(tmpFile, []byte(yamlContent), 0o600)
	suite.Require().NoError(err)

	// Parse the file
	prompts, err := filePrompts(tmpFile)
	suite.Require().NoError(err)
	suite.Require().Len(prompts, 3, "Expected 3 prompts")

	// Verify always_ask is correctly parsed
	suite.Equal("PORT", prompts[0].Env)
	suite.True(prompts[0].AlwaysAsk, "PORT should have always_ask=true")
	suite.Equal("8080", prompts[0].Default)

	suite.Equal("DEBUG", prompts[1].Env)
	suite.False(prompts[1].AlwaysAsk, "DEBUG should have always_ask=false (default)")

	suite.Equal("LOG_LEVEL", prompts[2].Env)
	suite.False(prompts[2].AlwaysAsk, "LOG_LEVEL should have always_ask=false (explicit)")
}

func (suite *BuilderTestSuite) TestShouldAsk_ActiveAndNotHidden() {
	prompt := UserPrompt{Active: true, Hidden: false}
	suite.True(prompt.ShouldAsk(false))
}

func (suite *BuilderTestSuite) TestShouldAsk_NotActive() {
	prompt := UserPrompt{Active: false, Hidden: false}
	suite.False(prompt.ShouldAsk(false))
}

func (suite *BuilderTestSuite) TestShouldAsk_Hidden() {
	prompt := UserPrompt{Active: true, Hidden: true}
	suite.False(prompt.ShouldAsk(false))
}

func (suite *BuilderTestSuite) TestShouldAsk_SkipsPromptWithDefault() {
	prompt := UserPrompt{Active: true, Hidden: false, Default: "default_value", Value: "default_value"}
	suite.False(prompt.ShouldAsk(false), "Should skip prompt when value equals default")
}

func (suite *BuilderTestSuite) TestShouldAsk_ShowsPromptWithModifiedValue() {
	prompt := UserPrompt{Active: true, Hidden: false, Default: "default_value", Value: "user_modified"}
	suite.True(prompt.ShouldAsk(false), "Should show prompt when value differs from default")
}

func (suite *BuilderTestSuite) TestShouldAsk_ShowsPromptWithoutDefault() {
	prompt := UserPrompt{Active: true, Hidden: false, Default: "", Value: ""}
	suite.True(prompt.ShouldAsk(false), "Should show prompt when no default is set")
}

func (suite *BuilderTestSuite) TestShouldAsk_AlwaysAskOverridesDefault() {
	prompt := UserPrompt{Active: true, Hidden: false, Default: "default_value", Value: "default_value", AlwaysAsk: true}
	suite.True(prompt.ShouldAsk(false), "Should show prompt when always_ask is true even if value equals default")
}

func (suite *BuilderTestSuite) TestShouldAsk_ShowAllOverridesDefault() {
	prompt := UserPrompt{Active: true, Hidden: false, Default: "default_value", Value: "default_value"}
	suite.True(prompt.ShouldAsk(true), "Should show prompt when showAll is true even if value equals default")
}

func (suite *BuilderTestSuite) TestShouldAsk_ShowAllDoesNotOverrideHidden() {
	prompt := UserPrompt{Active: true, Hidden: true, Default: "default_value", Value: "default_value"}
	suite.False(prompt.ShouldAsk(true), "Should not show hidden prompts even when showAll is true")
}

func (suite *BuilderTestSuite) TestShouldAsk_RequiresOptionsAlwaysShown() {
	prompt := UserPrompt{
		Active:  true,
		Hidden:  false,
		Default: "option1",
		Value:   "option1",
		Options: []PromptOption{
			{Name: "Option 1", Value: "option1"},
			{Name: "Option 2", Value: "option2", Requires: "extra_config"},
		},
	}
	suite.True(prompt.ShouldAsk(false), "Should show prompt with requires options even if value equals default")
}

func (suite *BuilderTestSuite) TestShouldAsk_OptionsWithoutRequiresCanBeSkipped() {
	prompt := UserPrompt{
		Active:  true,
		Hidden:  false,
		Default: "option1",
		Value:   "option1",
		Options: []PromptOption{
			{Name: "Option 1", Value: "option1"},
			{Name: "Option 2", Value: "option2"},
		},
	}
	suite.False(prompt.ShouldAsk(false), "Should skip prompt with options but no requires if value equals default")
}

func (suite *BuilderTestSuite) TestHasRequiresOptions() {
	promptWithRequires := UserPrompt{
		Options: []PromptOption{
			{Name: "Option 1", Value: "option1"},
			{Name: "Option 2", Value: "option2", Requires: "extra_config"},
		},
	}
	suite.True(promptWithRequires.HasRequiresOptions())

	promptWithoutRequires := UserPrompt{
		Options: []PromptOption{
			{Name: "Option 1", Value: "option1"},
			{Name: "Option 2", Value: "option2"},
		},
	}
	suite.False(promptWithoutRequires.HasRequiresOptions())

	promptNoOptions := UserPrompt{}
	suite.False(promptNoOptions.HasRequiresOptions())
}
