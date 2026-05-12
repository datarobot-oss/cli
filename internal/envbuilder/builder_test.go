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

package envbuilder

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v3"
)

func TestBuilderTestSuite(t *testing.T) {
	suite.Run(t, new(BuilderTestSuite))
}

func TestPromptFileSchema(t *testing.T) {
	suite.Run(t, new(PromptFileSchemaTestSuite))
}

type PromptFileSchemaTestSuite struct {
	suite.Suite
	schema *PromptFileSchema
}

func (suite *PromptFileSchemaTestSuite) SetupTest() {
	suite.schema = &PromptFileSchema{}
}

func (suite *PromptFileSchemaTestSuite) TestValidPromptFile() {
	yamlContent := `
root:
  - env: PORT
    help: Application port
  - env: DEBUG
    help: Enable debug mode
`

	var root yaml.Node

	err := yaml.Unmarshal([]byte(yamlContent), &root)
	suite.Require().NoError(err)

	validationErr := suite.schema.Validate(root.Content[0])
	suite.NoError(validationErr, "Valid prompt file should pass validation")
}

func (suite *PromptFileSchemaTestSuite) TestValidMultipleSections() {
	yamlContent := `
root:
  - env: PORT
    help: Application port
config:
  - env: DB_URL
    help: Database URL
  - env: DB_USER
    help: Database user
`

	var root yaml.Node

	err := yaml.Unmarshal([]byte(yamlContent), &root)
	suite.Require().NoError(err)

	validationErr := suite.schema.Validate(root.Content[0])
	suite.NoError(validationErr, "Valid prompt file with multiple sections should pass")
}

func (suite *PromptFileSchemaTestSuite) TestValidWithKeyInsteadOfEnv() {
	yamlContent := `
root:
  - key: database-url
    help: Database connection string
`

	var root yaml.Node

	err := yaml.Unmarshal([]byte(yamlContent), &root)
	suite.Require().NoError(err)

	validationErr := suite.schema.Validate(root.Content[0])
	suite.NoError(validationErr, "Prompt with 'key' instead of 'env' should be valid")
}

func (suite *PromptFileSchemaTestSuite) TestInvalidRootIsScalar() {
	yamlContent := `"just a string"`

	var root yaml.Node

	err := yaml.Unmarshal([]byte(yamlContent), &root)
	suite.Require().NoError(err)

	validationErr := suite.schema.Validate(root.Content[0])
	suite.Require().Error(validationErr, "Scalar root should fail validation")
	suite.Require().Contains(validationErr.Error(), "root must be a mapping")
}

func (suite *PromptFileSchemaTestSuite) TestInvalidRootIsSequence() {
	yamlContent := `
- env: PORT
  help: Application port
`

	var root yaml.Node

	err := yaml.Unmarshal([]byte(yamlContent), &root)
	suite.Require().NoError(err)

	validationErr := suite.schema.Validate(root.Content[0])
	suite.Require().Error(validationErr, "Sequence root should fail validation")
	suite.Require().Contains(validationErr.Error(), "root must be a mapping")
}

func (suite *PromptFileSchemaTestSuite) TestInvalidEmptyRoot() {
	yamlContent := ``

	var root yaml.Node

	err := yaml.Unmarshal([]byte(yamlContent), &root)
	suite.Require().NoError(err)

	if len(root.Content) == 0 {
		// Empty YAML has no content, which is expected
		return
	}

	validationErr := suite.schema.Validate(root.Content[0])
	suite.Error(validationErr, "Empty root should fail validation")
}

func (suite *PromptFileSchemaTestSuite) TestInvalidSectionValueIsScalar() {
	yamlContent := `
root: "just a string"
`

	var root yaml.Node

	err := yaml.Unmarshal([]byte(yamlContent), &root)
	suite.Require().NoError(err)

	validationErr := suite.schema.Validate(root.Content[0])
	suite.Require().Error(validationErr, "Section with scalar value should fail validation")
	suite.Require().Contains(validationErr.Error(), "section value must be a sequence")
}

func (suite *PromptFileSchemaTestSuite) TestInvalidPromptIsNotMapping() {
	yamlContent := `
root:
  - "just a string"
`

	var root yaml.Node

	err := yaml.Unmarshal([]byte(yamlContent), &root)
	suite.Require().NoError(err)

	validationErr := suite.schema.Validate(root.Content[0])
	suite.Require().Error(validationErr, "Non-mapping prompt should fail validation")
	suite.Require().Contains(validationErr.Error(), "prompt at index 0 must be a mapping")
}

func (suite *PromptFileSchemaTestSuite) TestInvalidPromptMissingEnvAndKey() {
	yamlContent := `
root:
  - help: Missing env and key
`

	var root yaml.Node

	err := yaml.Unmarshal([]byte(yamlContent), &root)
	suite.Require().NoError(err)

	validationErr := suite.schema.Validate(root.Content[0])
	suite.Require().Error(validationErr, "Prompt missing env and key should fail validation")
	suite.Require().Contains(validationErr.Error(), "either 'env' or 'key'")
}

func (suite *PromptFileSchemaTestSuite) TestValidPromptMissingHelpIsAllowed() {
	yamlContent := `
root:
  - env: PORT
    type: string
`

	var root yaml.Node

	err := yaml.Unmarshal([]byte(yamlContent), &root)
	suite.Require().NoError(err)

	validationErr := suite.schema.Validate(root.Content[0])
	suite.NoError(validationErr, "Prompt missing help should be valid (warning logged, not error)")
}

func (suite *PromptFileSchemaTestSuite) TestLooksLikePromptFileValidFile() {
	yamlContent := []byte(`
root:
  - env: PORT
    help: Application port
  - env: DEBUG
    help: Enable debug mode
`)

	result := isPromptFile(yamlContent)
	suite.True(result, "Valid prompt file should be recognized")
}

func (suite *PromptFileSchemaTestSuite) TestLooksLikePromptFileInvalidStructure() {
	yamlContent := []byte(`
port: 8080
debug: true
`)

	result := isPromptFile(yamlContent)
	suite.False(result, "Non-prompt config file should not be recognized")
}

func (suite *PromptFileSchemaTestSuite) TestLooksLikePromptFileCopierAnswerFile() {
	yamlContent := []byte(`
project_name: my-project
project_slug: my_project
author_email: test@example.com
`)

	result := isPromptFile(yamlContent)
	suite.False(result, "Copier answer file should not be recognized as prompt file")
}

func (suite *PromptFileSchemaTestSuite) TestLooksLikePromptFileVersionManifest() {
	yamlContent := []byte(`
version: "1.0.0"
build_date: "2026-04-30"
components:
  - name: api
    version: "2.1.0"
  - name: worker
    version: "1.5.3"
`)

	result := isPromptFile(yamlContent)
	suite.False(result, "Version manifest should not be recognized as prompt file")
}

func (suite *PromptFileSchemaTestSuite) TestLooksLikePromptFileMalformedYAML() {
	yamlContent := []byte(`
root:
  - env: PORT
    help: "unclosed string
`)

	result := isPromptFile(yamlContent)
	suite.True(result, "Malformed YAML should return true to let real unmarshal surface error")
}

func (suite *PromptFileSchemaTestSuite) TestLooksLikePromptFileAgentYAMLStructure() {
	// Test with the exact structure from datarobot-agent-application
	yamlContent := []byte(`root:
  - env: AGENT_PORT
    type: string
    default: "8842"
    help: "Enter the local port to access the agent. Should be a unique value."
  - env: DATAROBOT_DEFAULT_EXECUTION_ENVIRONMENT
    type: string
    default: "[DataRobot] Python 3.11 GenAI Agents"
    optional: true
    help: "Enter the default execution environment for the agent's custom model."
  - env: DATAROBOT_DEFAULT_EXECUTION_ENVIRONMENT_VERSION_ID
    type: string
    default: "69e2134aa5df12076d70afe7"
    optional: true
    help: "Enter the default execution environment version ID for the agent's custom model."
`)

	result := isPromptFile(yamlContent)
	suite.True(result, "Agent YAML structure should be recognized as valid prompt file")
}

func (suite *BuilderTestSuite) TestMultipleYAMLFilesInSameFolder() {
	// Test that the builder correctly handles multiple valid YAML files in the same folder
	// while skipping non-prompt config files
	prompts, err := GatherUserPrompts(suite.tempDir, nil)
	suite.Require().NoError(err)

	// Should have gathered prompts from both parakeet.yaml and another_parakeet.yaml
	// plus the 2 core prompts (DATAROBOT_ENDPOINT and DATAROBOT_API_TOKEN)
	suite.Greater(len(prompts), 2, "Should gather prompts from multiple YAML files")

	// Verify that specific prompts from both files are present
	envVars := make(map[string]bool)
	for _, p := range prompts {
		envVars[p.Env] = true
	}

	// From testYamlFile1
	suite.True(envVars["PULUMI_CONFIG_PASSPHRASE"], "Should find PULUMI_CONFIG_PASSPHRASE from first file")
	suite.True(envVars["DATAROBOT_DEFAULT_USE_CASE"], "Should find DATAROBOT_DEFAULT_USE_CASE from first file")

	// From testYamlFile2
	suite.True(envVars["INFRA_ENABLE_LLM"], "Should find INFRA_ENABLE_LLM from second file")
	suite.True(envVars["TEXTGEN_DEPLOYMENT_ID"], "Should find TEXTGEN_DEPLOYMENT_ID from second file")
	suite.True(envVars["TEXTGEN_REGISTERED_MODEL_ID"], "Should find TEXTGEN_REGISTERED_MODEL_ID from second file")
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

func (suite *BuilderTestSuite) TestAlwaysPromptYAMLParsing() {
	yamlContent := `
root:
  - env: PORT
    type: string
    default: "8080"
    help: Application port
    always_prompt: true
  - env: DEBUG
    type: string
    default: "false"
    help: Enable debug mode
  - env: LOG_LEVEL
    type: string
    default: "info"
    help: Log level
    always_prompt: false
`

	// Create a temporary YAML file
	tmpFile := filepath.Join(suite.tempDir, ".datarobot", "test_always_prompt.yaml")
	err := os.WriteFile(tmpFile, []byte(yamlContent), 0o600)
	suite.Require().NoError(err)

	// Parse the file
	prompts, err := filePrompts(tmpFile)
	suite.Require().NoError(err)
	suite.Require().Len(prompts, 3, "Expected 3 prompts")

	// Verify always_prompt is correctly parsed
	suite.Equal("PORT", prompts[0].Env)
	suite.True(prompts[0].AlwaysPrompt, "PORT should have always_prompt=true")
	suite.Equal("8080", prompts[0].Default)

	suite.Equal("DEBUG", prompts[1].Env)
	suite.False(prompts[1].AlwaysPrompt, "DEBUG should have always_prompt=false (default)")

	suite.Equal("LOG_LEVEL", prompts[2].Env)
	suite.False(prompts[2].AlwaysPrompt, "LOG_LEVEL should have always_prompt=false (explicit)")
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

func (suite *BuilderTestSuite) TestShouldAsk_AlwaysPromptOverridesDefault() {
	prompt := UserPrompt{Active: true, Hidden: false, Default: "default_value", Value: "default_value", AlwaysPrompt: true}
	suite.True(prompt.ShouldAsk(false), "Should show prompt when always_prompt is true even if value equals default")
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

func (suite *BuilderTestSuite) TestShouldAsk_SkipsPromptWithGenerate() {
	prompt := UserPrompt{Active: true, Hidden: false, Generate: true}
	suite.False(prompt.ShouldAsk(false), "Should skip prompt when generate is true")
}

func (suite *BuilderTestSuite) TestShouldAsk_GenerateWithValueSkipped() {
	prompt := UserPrompt{Active: true, Hidden: false, Generate: true, Value: "some_generated_value"}
	suite.False(prompt.ShouldAsk(false), "Should skip prompt when generate is true even if value is set")
}

func (suite *BuilderTestSuite) TestShouldAsk_GenerateAlwaysPromptShowAll() {
	prompt := UserPrompt{Active: true, Hidden: false, Generate: true}
	suite.True(prompt.ShouldAsk(true), "Should show prompt when generate is true but showAll is true")
}

func (suite *BuilderTestSuite) TestShouldAsk_GenerateWithSecretType() {
	prompt := UserPrompt{
		Active:   true,
		Hidden:   false,
		Generate: true,
		Type:     PromptTypeSecret,
		Help:     "Auto-generated secret",
	}
	suite.False(prompt.ShouldAsk(false), "Should skip secret_string prompt when generate is true")
}

func (suite *BuilderTestSuite) TestShouldAsk_GenerateWithDefault() {
	prompt := UserPrompt{
		Active:   true,
		Hidden:   false,
		Generate: true,
		Default:  "default_secret",
		Value:    "default_secret",
	}
	suite.False(prompt.ShouldAsk(false), "Should skip prompt when generate is true, even if default is set")
}

func (suite *BuilderTestSuite) TestGenerateYAMLParsing() {
	yamlContent := `
root:
  - env: SESSION_SECRET
    type: secret_string
    generate: true
    help: Session encryption key (auto-generated)
    optional: false
  - env: API_KEY
    type: secret_string
    generate: false
    help: Enter your API key
    optional: false
  - env: JWT_SECRET
    type: secret_string
    generate: true
    help: JWT signing key
    optional: false
    always_prompt: true
`

	// Create a temporary YAML file
	tmpFile := filepath.Join(suite.tempDir, ".datarobot", "test_generate.yaml")
	err := os.WriteFile(tmpFile, []byte(yamlContent), 0o600)
	suite.Require().NoError(err)

	// Parse the file
	prompts, err := filePrompts(tmpFile)
	suite.Require().NoError(err)
	suite.Require().Len(prompts, 3, "Expected 3 prompts")

	// Verify generate is correctly parsed
	suite.Equal("SESSION_SECRET", prompts[0].Env)
	suite.True(prompts[0].Generate, "SESSION_SECRET should have generate=true")
	suite.Equal(PromptTypeSecret, prompts[0].Type, "SESSION_SECRET should be secret_string type")
	suite.False(prompts[0].AlwaysPrompt, "SESSION_SECRET should have always_prompt=false (default)")

	suite.Equal("API_KEY", prompts[1].Env)
	suite.False(prompts[1].Generate, "API_KEY should have generate=false (explicit)")
	suite.Equal(PromptTypeSecret, prompts[1].Type, "API_KEY should be secret_string type")

	suite.Equal("JWT_SECRET", prompts[2].Env)
	suite.True(prompts[2].Generate, "JWT_SECRET should have generate=true")
	suite.True(prompts[2].AlwaysPrompt, "JWT_SECRET should have always_prompt=true")
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

func (suite *BuilderTestSuite) TestApplyGeneratedValues_GeneratesValue() {
	prompts := []UserPrompt{
		{
			Env:      "SESSION_SECRET",
			Type:     PromptTypeSecret,
			Generate: true,
			Value:    "",
		},
	}

	result, err := ApplyGeneratedValues(prompts)
	suite.Require().NoError(err)
	suite.NotEmpty(result[0].Value, "Should generate a value for secret_string with generate: true")
	suite.Len(result[0].Value, GeneratedSecretLength, "Generated value should match expected length")
}

func (suite *BuilderTestSuite) TestApplyGeneratedValues_DoesNotOverwriteExistingValue() {
	prompts := []UserPrompt{
		{
			Env:      "SESSION_SECRET",
			Type:     PromptTypeSecret,
			Generate: true,
			Value:    "existing_value",
		},
	}

	result, err := ApplyGeneratedValues(prompts)
	suite.Require().NoError(err)
	suite.Equal("existing_value", result[0].Value, "Should not overwrite existing value")
}

func (suite *BuilderTestSuite) TestApplyGeneratedValues_IgnoresNonGeneratePrompts() {
	prompts := []UserPrompt{
		{
			Env:      "API_KEY",
			Type:     PromptTypeSecret,
			Generate: false,
			Value:    "",
		},
	}

	result, err := ApplyGeneratedValues(prompts)
	suite.Require().NoError(err)
	suite.Empty(result[0].Value, "Should not generate value for prompts with generate: false")
}

func (suite *BuilderTestSuite) TestApplyGeneratedValues_IgnoresNonSecretTypes() {
	prompts := []UserPrompt{
		{
			Env:      "SOME_VAR",
			Type:     PromptTypeString,
			Generate: true,
			Value:    "",
		},
	}

	result, err := ApplyGeneratedValues(prompts)
	suite.Require().NoError(err)
	suite.Empty(result[0].Value, "Should not generate value for non-secret_string types")
}

func (suite *BuilderTestSuite) TestApplyGeneratedValues_MultiplePrompts() {
	prompts := []UserPrompt{
		{
			Env:      "SESSION_SECRET",
			Type:     PromptTypeSecret,
			Generate: true,
			Value:    "",
		},
		{
			Env:      "EXISTING_SECRET",
			Type:     PromptTypeSecret,
			Generate: true,
			Value:    "already_set",
		},
		{
			Env:      "API_KEY",
			Type:     PromptTypeSecret,
			Generate: false,
			Value:    "",
		},
	}

	result, err := ApplyGeneratedValues(prompts)
	suite.Require().NoError(err)

	suite.NotEmpty(result[0].Value, "First secret should be generated")
	suite.Equal("already_set", result[1].Value, "Second secret should not be overwritten")
	suite.Empty(result[2].Value, "Third secret should not be generated (generate: false)")
}
